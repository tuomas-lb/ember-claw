package k8s

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/tuomas-lb/ember-claw/dashboard/internal/config"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	managedByLabel = "app.kubernetes.io/managed-by=eclaw"
	nameLabel      = "app.kubernetes.io/name=picoclaw"
)

// Client wraps a Kubernetes clientset and targets a specific namespace.
type Client struct {
	cs        kubernetes.Interface
	namespace string
}

// New creates a Kubernetes client. It tries in-cluster config first,
// falling back to the user's kubeconfig for local development.
func New(namespace string) (*Client, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		// Fall back to kubeconfig
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			home, _ := os.UserHomeDir()
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("build k8s config: %w", err)
		}
	}

	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create k8s clientset: %w", err)
	}

	return &Client{cs: cs, namespace: namespace}, nil
}

// ListInstances returns all PicoClaw instances managed by eclaw.
func (c *Client) ListInstances(ctx context.Context) ([]config.Instance, error) {
	labelSelector := fmt.Sprintf("%s,%s", managedByLabel, nameLabel)
	deployments, err := c.cs.AppsV1().Deployments(c.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}

	instances := make([]config.Instance, 0, len(deployments.Items))
	for _, d := range deployments.Items {
		inst := deploymentToInstance(&d)
		// Enrich model/provider from env ConfigMap if still empty
		if inst.Model == "" || inst.Provider == "" {
			cmName := resourceName(inst.Name, "env")
			if cm, err := c.cs.CoreV1().ConfigMaps(c.namespace).Get(ctx, cmName, metav1.GetOptions{}); err == nil {
				if inst.Model == "" {
					inst.Model = cm.Data["PICOCLAW_MODEL"]
				}
				if inst.Provider == "" {
					inst.Provider = cm.Data["PICOCLAW_PROVIDER"]
				}
			}
		}
		instances = append(instances, inst)
	}
	return instances, nil
}

// GetInstance returns a single instance by name, enriched with pod info.
func (c *Client) GetInstance(ctx context.Context, name string) (*config.InstanceStatus, error) {
	depName := resourceName(name, "")
	d, err := c.cs.AppsV1().Deployments(c.namespace).Get(ctx, depName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get deployment %s: %w", depName, err)
	}

	status := &config.InstanceStatus{
		Instance: deploymentToInstance(d),
	}

	pod, err := c.FindRunningPod(ctx, name)
	if err == nil && pod != nil {
		status.PodName = pod.Name
		status.PodIP = pod.Status.PodIP
	}

	return status, nil
}

// DeployInstance creates all resources for a new PicoClaw instance.
func (c *Client) DeployInstance(ctx context.Context, req config.DeployRequest) error {
	// Build resources
	secret, err := BuildSecret(c.namespace, req)
	if err != nil {
		return fmt.Errorf("build secret: %w", err)
	}
	cm := BuildConfigMap(c.namespace, req)
	bootstrapCM := BuildBootstrapConfigMap(c.namespace, req)
	pvc := BuildPVC(c.namespace, req)
	deployment := BuildDeployment(c.namespace, req)
	svc := BuildService(c.namespace, req)

	// Create PVC
	if _, err := c.cs.CoreV1().PersistentVolumeClaims(c.namespace).Create(ctx, pvc, metav1.CreateOptions{}); err != nil {
		if !k8serrors.IsAlreadyExists(err) {
			return fmt.Errorf("create pvc: %w", err)
		}
	}

	// Create env ConfigMap
	if _, err := c.cs.CoreV1().ConfigMaps(c.namespace).Create(ctx, cm, metav1.CreateOptions{}); err != nil {
		if !k8serrors.IsAlreadyExists(err) {
			return fmt.Errorf("create configmap: %w", err)
		}
	}

	// Create bootstrap ConfigMap (AGENTS.md, SOUL.md, etc.)
	if _, err := c.cs.CoreV1().ConfigMaps(c.namespace).Create(ctx, bootstrapCM, metav1.CreateOptions{}); err != nil {
		if !k8serrors.IsAlreadyExists(err) {
			return fmt.Errorf("create bootstrap configmap: %w", err)
		}
	}

	// Create Secret
	if _, err := c.cs.CoreV1().Secrets(c.namespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
		if !k8serrors.IsAlreadyExists(err) {
			return fmt.Errorf("create secret: %w", err)
		}
	}

	// Create Service
	if _, err := c.cs.CoreV1().Services(c.namespace).Create(ctx, svc, metav1.CreateOptions{}); err != nil {
		if !k8serrors.IsAlreadyExists(err) {
			return fmt.Errorf("create service: %w", err)
		}
	}

	// Create Deployment
	if _, err := c.cs.AppsV1().Deployments(c.namespace).Create(ctx, deployment, metav1.CreateOptions{}); err != nil {
		if !k8serrors.IsAlreadyExists(err) {
			return fmt.Errorf("create deployment: %w", err)
		}
	}

	return nil
}

// DeleteInstance removes all resources for a PicoClaw instance.
func (c *Client) DeleteInstance(ctx context.Context, name string) error {
	deletePolicy := metav1.DeletePropagationForeground
	opts := metav1.DeleteOptions{PropagationPolicy: &deletePolicy}

	depName := resourceName(name, "")
	if err := c.cs.AppsV1().Deployments(c.namespace).Delete(ctx, depName, opts); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("delete deployment: %w", err)
	}

	if err := c.cs.CoreV1().Services(c.namespace).Delete(ctx, depName, opts); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("delete service: %w", err)
	}

	secretName := resourceName(name, "config")
	if err := c.cs.CoreV1().Secrets(c.namespace).Delete(ctx, secretName, opts); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("delete secret: %w", err)
	}

	cmName := resourceName(name, "env")
	if err := c.cs.CoreV1().ConfigMaps(c.namespace).Delete(ctx, cmName, opts); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("delete env configmap: %w", err)
	}

	bootstrapName := resourceName(name, "bootstrap")
	if err := c.cs.CoreV1().ConfigMaps(c.namespace).Delete(ctx, bootstrapName, opts); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("delete bootstrap configmap: %w", err)
	}

	pvcName := resourceName(name, "data")
	if err := c.cs.CoreV1().PersistentVolumeClaims(c.namespace).Delete(ctx, pvcName, opts); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("delete pvc: %w", err)
	}

	return nil
}

// RestartInstance triggers a rolling restart by patching the deployment annotation.
func (c *Client) RestartInstance(ctx context.Context, name string) error {
	depName := resourceName(name, "")
	patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":%q}}}}}`,
		time.Now().UTC().Format(time.RFC3339))
	_, err := c.cs.AppsV1().Deployments(c.namespace).Patch(
		ctx, depName, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("patch deployment %s: %w", depName, err)
	}
	return nil
}

// RestartDeployment triggers a rolling restart of any deployment by name.
func (c *Client) RestartDeployment(ctx context.Context, name string) error {
	patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":%q}}}}}`,
		time.Now().UTC().Format(time.RFC3339))
	_, err := c.cs.AppsV1().Deployments(c.namespace).Patch(
		ctx, name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("restart deployment %s: %w", name, err)
	}
	return nil
}

// GetConfig reads the config.json from the instance's secret.
func (c *Client) GetConfig(ctx context.Context, name string) (map[string]interface{}, error) {
	secretName := resourceName(name, "config")
	secret, err := c.cs.CoreV1().Secrets(c.namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get secret %s: %w", secretName, err)
	}

	data, ok := secret.Data["config.json"]
	if !ok {
		return nil, fmt.Errorf("config.json not found in secret %s", secretName)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config.json: %w", err)
	}
	return cfg, nil
}

// PushConfig updates config.json in the instance's secret and triggers a restart.
func (c *Client) PushConfig(ctx context.Context, name string, cfg map[string]interface{}) error {
	secretName := resourceName(name, "config")
	secret, err := c.cs.CoreV1().Secrets(c.namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get secret %s: %w", secretName, err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}
	secret.Data["config.json"] = data

	if _, err := c.cs.CoreV1().Secrets(c.namespace).Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update secret: %w", err)
	}

	// Trigger rolling restart so the new config is picked up
	return c.RestartInstance(ctx, name)
}

// SetSecret updates (or adds) a single key in the instance's config secret.
func (c *Client) SetSecret(ctx context.Context, name, key, value string) error {
	secretName := resourceName(name, "config")
	secret, err := c.cs.CoreV1().Secrets(c.namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get secret %s: %w", secretName, err)
	}

	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}
	secret.Data[key] = []byte(value)

	if _, err := c.cs.CoreV1().Secrets(c.namespace).Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update secret: %w", err)
	}
	return nil
}

// StreamLogs returns an io.ReadCloser that streams logs from the instance's running pod.
func (c *Client) StreamLogs(ctx context.Context, name string, follow bool, tailLines int64) (io.ReadCloser, error) {
	pod, err := c.FindRunningPod(ctx, name)
	if err != nil {
		return nil, err
	}
	if pod == nil {
		return nil, fmt.Errorf("no running pod for instance %s", name)
	}

	// Select the bot container by name — a pod may carry helper sidecars (e.g.
	// backlog-web) so we cannot assume containers[0] is the agent. Prefer the
	// well-known main-container names, falling back to the first container.
	containerName := mainContainerName(pod)

	opts := &corev1.PodLogOptions{
		Follow:    follow,
		Container: containerName,
	}
	if tailLines > 0 {
		opts.TailLines = &tailLines
	}

	req := c.cs.CoreV1().Pods(c.namespace).GetLogs(pod.Name, opts)
	return req.Stream(ctx)
}

// mainContainerName returns the agent container in a (possibly multi-container)
// instance pod. eclaw names it "sidecar"; the dashboard's own deploy names it
// "picoclaw". Helper sidecars (backlog-web, etc.) are skipped. Falls back to the
// first container if neither known name is present.
func mainContainerName(pod *corev1.Pod) string {
	for _, want := range []string{"sidecar", "picoclaw"} {
		for _, ct := range pod.Spec.Containers {
			if ct.Name == want {
				return ct.Name
			}
		}
	}
	if len(pod.Spec.Containers) > 0 {
		return pod.Spec.Containers[0].Name
	}
	return ""
}

// StreamLogsScanner is a convenience helper that returns a line scanner over pod logs.
func (c *Client) StreamLogsScanner(ctx context.Context, name string, follow bool, tailLines int64) (*bufio.Scanner, io.Closer, error) {
	rc, err := c.StreamLogs(ctx, name, follow, tailLines)
	if err != nil {
		return nil, nil, err
	}
	return bufio.NewScanner(rc), rc, nil
}

// FindRunningPod returns the first Running pod for the instance, or nil if none found.
func (c *Client) FindRunningPod(ctx context.Context, name string) (*corev1.Pod, error) {
	selector := fmt.Sprintf("app.kubernetes.io/instance=%s,app.kubernetes.io/name=picoclaw", name)
	pods, err := c.cs.CoreV1().Pods(c.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return nil, fmt.Errorf("list pods for %s: %w", name, err)
	}

	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Status.Phase == corev1.PodRunning {
			return pod, nil
		}
	}

	// If no running pod, return the first pod (could be pending/starting)
	if len(pods.Items) > 0 {
		return &pods.Items[0], nil
	}

	return nil, nil
}

// deploymentToInstance converts an appsv1.Deployment to a config.Instance.
func deploymentToInstance(d *appsv1.Deployment) config.Instance {
	// Derive instance name by stripping the "picoclaw-" prefix
	name := d.Name
	const prefix = "picoclaw-"
	if len(name) > len(prefix) && name[:len(prefix)] == prefix {
		name = name[len(prefix):]
	}

	// Determine status from deployment conditions and replica counts
	status := "Pending"
	if d.Status.ReadyReplicas > 0 {
		status = "Running"
	} else if d.Status.UnavailableReplicas > 0 {
		// Check conditions for CrashLoopBackOff
		for _, c := range d.Status.Conditions {
			if c.Type == appsv1.DeploymentAvailable && c.Status == corev1.ConditionFalse {
				status = "Unavailable"
			}
		}
	}

	// Try labels first, fall back to env vars from container spec
	model := d.Labels["picoclaw-model"]
	provider := d.Labels["picoclaw-provider"]
	if model == "" || provider == "" {
		for _, c := range d.Spec.Template.Spec.Containers {
			for _, envFrom := range c.EnvFrom {
				if envFrom.ConfigMapRef != nil {
					// Will be enriched by ListInstances reading the configmap
				}
			}
			for _, env := range c.Env {
				if env.Name == "PICOCLAW_MODEL" && model == "" {
					model = env.Value
				}
				if env.Name == "PICOCLAW_PROVIDER" && provider == "" {
					provider = env.Value
				}
			}
		}
	}

	// Extract resource limits from first container spec
	cpu := ""
	mem := ""
	if len(d.Spec.Template.Spec.Containers) > 0 {
		limits := d.Spec.Template.Spec.Containers[0].Resources.Limits
		if v, ok := limits[corev1.ResourceCPU]; ok {
			cpu = v.String()
		}
		if v, ok := limits[corev1.ResourceMemory]; ok {
			mem = v.String()
		}
	}

	age := formatAge(d.CreationTimestamp.Time)

	return config.Instance{
		Name:     name,
		Status:   status,
		Model:    model,
		Provider: provider,
		Ready:    d.Status.ReadyReplicas > 0,
		Age:      age,
		CPU:      cpu,
		Memory:   mem,
	}
}

// formatAge returns a human-readable age string.
func formatAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// GetFleetMD returns the contents of Fleet.md from the shared ConfigMap.
func (c *Client) GetFleetMD(ctx context.Context) (string, error) {
	cm, err := c.cs.CoreV1().ConfigMaps(c.namespace).Get(ctx, FleetConfigMap, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	return cm.Data[FleetMDKey], nil
}

// PutFleetMD creates or updates the Fleet.md ConfigMap.
func (c *Client) PutFleetMD(ctx context.Context, content string) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      FleetConfigMap,
			Namespace: c.namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "eclaw",
				"app.kubernetes.io/component":  "fleet-registry",
			},
		},
		Data: map[string]string{
			FleetMDKey: content,
		},
	}

	_, err := c.cs.CoreV1().ConfigMaps(c.namespace).Get(ctx, FleetConfigMap, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		_, err = c.cs.CoreV1().ConfigMaps(c.namespace).Create(ctx, cm, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	_, err = c.cs.CoreV1().ConfigMaps(c.namespace).Update(ctx, cm, metav1.UpdateOptions{})
	return err
}

// GetCallRouting returns the call routing ConfigMap data.
func (c *Client) GetCallRouting(ctx context.Context) (map[string]string, error) {
	cm, err := c.cs.CoreV1().ConfigMaps(c.namespace).Get(ctx, RoutingConfigMap, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	return cm.Data, nil
}

// PutCallRouting creates or updates the call routing ConfigMap.
func (c *Client) PutCallRouting(ctx context.Context, data map[string]string) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      RoutingConfigMap,
			Namespace: c.namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "eclaw",
				"app.kubernetes.io/component":  "call-routing",
			},
		},
		Data: data,
	}

	_, err := c.cs.CoreV1().ConfigMaps(c.namespace).Get(ctx, RoutingConfigMap, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		_, err = c.cs.CoreV1().ConfigMaps(c.namespace).Create(ctx, cm, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	_, err = c.cs.CoreV1().ConfigMaps(c.namespace).Update(ctx, cm, metav1.UpdateOptions{})
	return err
}
