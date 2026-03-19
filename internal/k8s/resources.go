package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// DefaultImage is the default sidecar container image.
	DefaultImage = "reg.r.lastbot.com/ember-claw-sidecar:latest"
	// DefaultStorageSize is the default PVC size.
	DefaultStorageSize = "1Gi"
	// MountPath is the path where the PVC is mounted in the container.
	MountPath = "/home/picoclaw/.picoclaw"
)

// DeployOptions contains all configuration for deploying a PicoClaw instance.
type DeployOptions struct {
	Name          string            // Instance name (resources are prefixed with picoclaw-{name})
	Provider      string            // AI provider (anthropic, openai, etc.)
	APIKey        string            // Provider API key
	Model         string            // Model name
	Image         string            // Container image (default: DefaultImage)
	CPURequest    string            // e.g., "100m"
	CPULimit      string            // e.g., "500m"
	MemoryRequest string            // e.g., "128Mi"
	MemoryLimit   string            // e.g., "512Mi"
	StorageSize   string            // PVC size (default: "1Gi")
	StorageClass  string            // Optional storage class name
	CustomEnv     map[string]string // Additional env vars
}

// picoClawConfig is the subset of PicoClaw's config.json we generate for deployment.
type picoClawConfig struct {
	Agents struct {
		Defaults struct {
			ModelName string `json:"model_name"`
			Workspace string `json:"workspace"`
		} `json:"defaults"`
	} `json:"agents"`
	ModelList []picoClawModelEntry `json:"model_list"`
}

type picoClawModelEntry struct {
	ModelName string `json:"model_name"`
	Model     string `json:"model"`
	APIKey    string `json:"api_key,omitempty"`
	APIBase   string `json:"api_base,omitempty"`
}

// buildPicoClawConfig generates a config.json for a PicoClaw instance.
// The API key is embedded directly in the model_list entry.
// This config.json is stored in a K8s Secret (not ConfigMap) to protect the key.
func buildPicoClawConfig(opts DeployOptions) picoClawConfig {
	provider := strings.ToLower(opts.Provider)
	modelID := opts.Model

	// Build the protocol/model-id reference for model_list
	modelRef := provider + "/" + modelID

	cfg := picoClawConfig{}
	cfg.Agents.Defaults.ModelName = modelID
	cfg.Agents.Defaults.Workspace = MountPath + "/workspace"

	entry := picoClawModelEntry{
		ModelName: modelID,
		Model:     modelRef,
		APIKey:    opts.APIKey,
	}

	// Set provider-specific API base URLs
	switch provider {
	case "anthropic":
		entry.APIBase = "https://api.anthropic.com/v1"
	case "openai":
		entry.APIBase = "https://api.openai.com/v1"
	case "gemini", "google":
		entry.APIBase = "https://generativelanguage.googleapis.com/v1beta"
	case "groq":
		entry.APIBase = "https://api.groq.com/openai/v1"
	case "deepseek":
		entry.APIBase = "https://api.deepseek.com/v1"
	case "openrouter":
		entry.APIBase = "https://openrouter.ai/api/v1"
	}

	cfg.ModelList = []picoClawModelEntry{entry}
	return cfg
}

// InstanceSummary holds a brief summary of a PicoClaw instance for list output.
type InstanceSummary struct {
	Name            string
	DeploymentName  string
	DesiredReplicas int32
	ReadyReplicas   int32
	PodPhase        corev1.PodPhase // Actual pod phase (more reliable than deployment status on some clusters)
	Age             time.Duration
}

// InstanceStatus holds detailed status information for a single instance.
type InstanceStatus struct {
	Name            string
	DeploymentName  string
	DesiredReplicas int32
	ReadyReplicas   int32
	PodPhase        corev1.PodPhase
	Provider        string
	Model           string
	Age             time.Duration
}

// resourceName builds the base resource name for an instance.
func resourceName(name string) string {
	return "picoclaw-" + name
}

// DeployInstance creates all K8s resources for a new PicoClaw instance:
// Secret (API key + provider config), ConfigMap (custom env), PVC (persistent storage),
// Deployment (sidecar container), and Service (ClusterIP for gRPC).
//
// API key is stored in Secret StringData and injected via envFrom.secretRef.
// It is NOT placed directly in Deployment env vars (K8S-03).
func (c *Client) DeployInstance(ctx context.Context, opts DeployOptions) error {
	if opts.Image == "" {
		opts.Image = DefaultImage
	}
	if opts.StorageSize == "" {
		opts.StorageSize = DefaultStorageSize
	}

	// Ensure the namespace exists before creating resources.
	if err := c.EnsureNamespace(ctx); err != nil {
		return fmt.Errorf("ensure namespace: %w", err)
	}

	baseName := resourceName(opts.Name)
	configSecretName := baseName + "-config"
	customConfigMapName := baseName + "-env"
	pvcName := baseName + "-data"
	instanceLabels := InstanceLabels(opts.Name)

	// 1. Create or update Secret with config.json (contains API key in model_list) (K8S-03, CONF-02).
	picoConfig := buildPicoClawConfig(opts)
	configJSON, err := json.MarshalIndent(picoConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal picoclaw config: %w", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configSecretName,
			Namespace: c.namespace,
			Labels:    instanceLabels,
		},
		StringData: map[string]string{
			"config.json": string(configJSON),
		},
	}
	if _, err := c.cs.CoreV1().Secrets(c.namespace).Create(ctx, secret, metav1.CreateOptions{}); k8serrors.IsAlreadyExists(err) {
		if _, err := c.cs.CoreV1().Secrets(c.namespace).Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update secret: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("create secret: %w", err)
	}

	// 2. Create or update ConfigMap for custom environment variables (CONF-04).
	cmData := make(map[string]string)
	for k, v := range opts.CustomEnv {
		cmData[k] = v
	}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      customConfigMapName,
			Namespace: c.namespace,
			Labels:    instanceLabels,
		},
		Data: cmData,
	}
	if _, err := c.cs.CoreV1().ConfigMaps(c.namespace).Create(ctx, configMap, metav1.CreateOptions{}); k8serrors.IsAlreadyExists(err) {
		if _, err := c.cs.CoreV1().ConfigMaps(c.namespace).Update(ctx, configMap, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update configmap: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("create configmap: %w", err)
	}

	// 3. Create PVC for persistent storage if it doesn't exist (CONF-05).
	// PVCs are immutable once created, so skip if already exists.
	storageQty := resource.MustParse(opts.StorageSize)
	pvcSpec := corev1.PersistentVolumeClaimSpec{
		AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		Resources: corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: storageQty,
			},
		},
	}
	if opts.StorageClass != "" {
		pvcSpec.StorageClassName = &opts.StorageClass
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: c.namespace,
			Labels:    instanceLabels,
		},
		Spec: pvcSpec,
	}
	if _, err := c.cs.CoreV1().PersistentVolumeClaims(c.namespace).Create(ctx, pvc, metav1.CreateOptions{}); err != nil && !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("create pvc: %w", err)
	}

	// 4. Build resource requirements (CONF-03).
	resourceReqs := corev1.ResourceRequirements{}
	if opts.CPURequest != "" || opts.MemoryRequest != "" {
		resourceReqs.Requests = corev1.ResourceList{}
		if opts.CPURequest != "" {
			resourceReqs.Requests[corev1.ResourceCPU] = resource.MustParse(opts.CPURequest)
		}
		if opts.MemoryRequest != "" {
			resourceReqs.Requests[corev1.ResourceMemory] = resource.MustParse(opts.MemoryRequest)
		}
	}
	if opts.CPULimit != "" || opts.MemoryLimit != "" {
		resourceReqs.Limits = corev1.ResourceList{}
		if opts.CPULimit != "" {
			resourceReqs.Limits[corev1.ResourceCPU] = resource.MustParse(opts.CPULimit)
		}
		if opts.MemoryLimit != "" {
			resourceReqs.Limits[corev1.ResourceMemory] = resource.MustParse(opts.MemoryLimit)
		}
	}

	// 5. Create Deployment with envFrom (secretRef + configMapRef), PVC mount (K8S-03, CONF-05).
	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      baseName,
			Namespace: c.namespace,
			Labels:    instanceLabels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: instanceLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: instanceLabels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "sidecar",
							Image: opts.Image,
							Ports: []corev1.ContainerPort{
								{Name: "grpc", ContainerPort: 50051, Protocol: corev1.ProtocolTCP},
								{Name: "http", ContainerPort: 8080, Protocol: corev1.ProtocolTCP},
							},
							EnvFrom: []corev1.EnvFromSource{
								{
									ConfigMapRef: &corev1.ConfigMapEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: customConfigMapName,
										},
									},
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "PICOCLAW_CONFIG",
									Value: "/config/config.json",
								},
								{
									Name:  "PICOCLAW_HOME",
									Value: MountPath,
								},
							},
							Resources: resourceReqs,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "data",
									MountPath: MountPath,
								},
								{
									Name:      "config",
									MountPath: "/config",
									ReadOnly:  true,
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt32(8080),
									},
								},
								InitialDelaySeconds: 15,
								PeriodSeconds:       20,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/ready",
										Port: intstr.FromInt32(8080),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       10,
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: pvcName,
								},
							},
						},
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: configSecretName,
									Items: []corev1.KeyToPath{
										{Key: "config.json", Path: "config.json"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	if _, err := c.cs.AppsV1().Deployments(c.namespace).Create(ctx, deployment, metav1.CreateOptions{}); k8serrors.IsAlreadyExists(err) {
		if _, err := c.cs.AppsV1().Deployments(c.namespace).Update(ctx, deployment, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update deployment: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("create deployment: %w", err)
	}

	// 6. Create or update ClusterIP Service targeting the gRPC port (port 50051).
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      baseName,
			Namespace: c.namespace,
			Labels:    instanceLabels,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: instanceLabels,
			Ports: []corev1.ServicePort{
				{Name: "grpc", Port: 50051, Protocol: corev1.ProtocolTCP},
			},
		},
	}
	if _, err := c.cs.CoreV1().Services(c.namespace).Create(ctx, svc, metav1.CreateOptions{}); err != nil && !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("create service: %w", err)
	}

	return nil
}

// ListInstances returns a summary of all eclaw-managed PicoClaw instances.
// Discovery is by label selector app.kubernetes.io/managed-by=eclaw (CLI-02, K8S-02).
func (c *Client) ListInstances(ctx context.Context) ([]InstanceSummary, error) {
	deployments, err := c.cs.AppsV1().Deployments(c.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: ManagedSelector(),
	})
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}

	// Also fetch all managed pods to get actual pod phase (more reliable on some clusters).
	pods, _ := c.cs.CoreV1().Pods(c.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: ManagedSelector(),
	})
	podPhaseByInstance := make(map[string]corev1.PodPhase)
	if pods != nil {
		for _, p := range pods.Items {
			name := p.Labels[LabelInstance]
			// Prefer Running over other phases if multiple pods exist.
			if existing, ok := podPhaseByInstance[name]; !ok || p.Status.Phase == corev1.PodRunning {
				_ = existing
				podPhaseByInstance[name] = p.Status.Phase
			}
		}
	}

	summaries := make([]InstanceSummary, 0, len(deployments.Items))
	for _, d := range deployments.Items {
		instanceName := d.Labels[LabelInstance]
		desired := int32(0)
		if d.Spec.Replicas != nil {
			desired = *d.Spec.Replicas
		}
		age := time.Duration(0)
		if !d.CreationTimestamp.IsZero() {
			age = time.Since(d.CreationTimestamp.Time)
		}
		summaries = append(summaries, InstanceSummary{
			Name:            instanceName,
			DeploymentName:  d.Name,
			DesiredReplicas: desired,
			ReadyReplicas:   d.Status.ReadyReplicas,
			PodPhase:        podPhaseByInstance[instanceName],
			Age:             age,
		})
	}
	return summaries, nil
}

// DeleteInstance removes compute resources (Deployment, Service, Secret, ConfigMap)
// for the named instance. The PVC is intentionally NOT deleted to prevent data loss (CLI-03).
// Use DeletePVC to remove the PVC explicitly.
func (c *Client) DeleteInstance(ctx context.Context, name string) error {
	selector := InstanceSelector(name)
	deleteOpts := metav1.DeleteOptions{}
	listOpts := metav1.ListOptions{LabelSelector: selector}

	// Delete Deployments
	deployments, err := c.cs.AppsV1().Deployments(c.namespace).List(ctx, listOpts)
	if err != nil {
		return fmt.Errorf("list deployments for delete: %w", err)
	}
	for _, d := range deployments.Items {
		if err := c.cs.AppsV1().Deployments(c.namespace).Delete(ctx, d.Name, deleteOpts); err != nil {
			return fmt.Errorf("delete deployment %s: %w", d.Name, err)
		}
	}

	// Delete Services
	services, err := c.cs.CoreV1().Services(c.namespace).List(ctx, listOpts)
	if err != nil {
		return fmt.Errorf("list services for delete: %w", err)
	}
	for _, s := range services.Items {
		if err := c.cs.CoreV1().Services(c.namespace).Delete(ctx, s.Name, deleteOpts); err != nil {
			return fmt.Errorf("delete service %s: %w", s.Name, err)
		}
	}

	// Delete Secrets
	secrets, err := c.cs.CoreV1().Secrets(c.namespace).List(ctx, listOpts)
	if err != nil {
		return fmt.Errorf("list secrets for delete: %w", err)
	}
	for _, s := range secrets.Items {
		if err := c.cs.CoreV1().Secrets(c.namespace).Delete(ctx, s.Name, deleteOpts); err != nil {
			return fmt.Errorf("delete secret %s: %w", s.Name, err)
		}
	}

	// Delete ConfigMaps
	cms, err := c.cs.CoreV1().ConfigMaps(c.namespace).List(ctx, listOpts)
	if err != nil {
		return fmt.Errorf("list configmaps for delete: %w", err)
	}
	for _, cm := range cms.Items {
		if err := c.cs.CoreV1().ConfigMaps(c.namespace).Delete(ctx, cm.Name, deleteOpts); err != nil {
			return fmt.Errorf("delete configmap %s: %w", cm.Name, err)
		}
	}

	return nil
}

// DeletePVC removes the persistent volume claim for the named instance.
// This is intentionally separate from DeleteInstance to prevent accidental data loss.
func (c *Client) DeletePVC(ctx context.Context, name string) error {
	pvcName := resourceName(name) + "-data"
	if err := c.cs.CoreV1().PersistentVolumeClaims(c.namespace).Delete(ctx, pvcName, metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("delete pvc %s: %w", pvcName, err)
	}
	return nil
}

// GetInstanceStatus returns detailed status for a PicoClaw instance,
// including deployment replica counts, pod phase, and provider/model config.
func (c *Client) GetInstanceStatus(ctx context.Context, name string) (*InstanceStatus, error) {
	baseName := resourceName(name)

	deployment, err := c.cs.AppsV1().Deployments(c.namespace).Get(ctx, baseName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get deployment %s: %w", baseName, err)
	}

	desired := int32(0)
	if deployment.Spec.Replicas != nil {
		desired = *deployment.Spec.Replicas
	}

	status := &InstanceStatus{
		Name:            name,
		DeploymentName:  deployment.Name,
		DesiredReplicas: desired,
		ReadyReplicas:   deployment.Status.ReadyReplicas,
	}

	// Check pod phase
	pods, err := c.cs.CoreV1().Pods(c.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: InstanceSelector(name),
	})
	if err == nil && len(pods.Items) > 0 {
		status.PodPhase = pods.Items[0].Status.Phase
	}

	// Get provider/model from secret's config.json
	secret, err := c.cs.CoreV1().Secrets(c.namespace).Get(ctx, baseName+"-config", metav1.GetOptions{})
	if err == nil {
		configJSON := ""
		if v, ok := secret.StringData["config.json"]; ok {
			configJSON = v
		} else if v, ok := secret.Data["config.json"]; ok {
			configJSON = string(v)
		}
		if configJSON != "" {
			var cfg picoClawConfig
			if err := json.Unmarshal([]byte(configJSON), &cfg); err == nil {
				status.Model = cfg.Agents.Defaults.ModelName
				if len(cfg.ModelList) > 0 {
					// Extract provider from model ref (e.g. "gemini/gemini-2.5-flash" -> "gemini")
					parts := strings.SplitN(cfg.ModelList[0].Model, "/", 2)
					if len(parts) > 0 {
						status.Provider = parts[0]
					}
				}
			}
		}
	}

	if !deployment.CreationTimestamp.IsZero() {
		status.Age = time.Since(deployment.CreationTimestamp.Time)
	}

	return status, nil
}

// GetInstanceLogs returns a log stream from the running pod for the named instance.
// If follow is true, the stream remains open for new log lines.
// tailLines specifies the number of recent lines to retrieve (0 = all).
func (c *Client) GetInstanceLogs(ctx context.Context, name string, follow bool, tailLines int64) (io.ReadCloser, error) {
	podName, err := c.FindRunningPod(ctx, name)
	if err != nil {
		return nil, err
	}

	logOpts := &corev1.PodLogOptions{
		Follow:    follow,
		Container: "sidecar",
	}
	if tailLines > 0 {
		logOpts.TailLines = &tailLines
	}

	req := c.cs.CoreV1().Pods(c.namespace).GetLogs(podName, logOpts)
	stream, err := req.Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("get logs for pod %s: %w", podName, err)
	}
	return stream, nil
}

// FindRunningPod returns the name of the first running pod for the named instance.
// Uses label selector to find pods then filters by Running phase.
// Returns an error if no running pod is found.
func (c *Client) FindRunningPod(ctx context.Context, name string) (string, error) {
	pods, err := c.cs.CoreV1().Pods(c.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: InstanceSelector(name),
	})
	if err != nil {
		return "", fmt.Errorf("list pods for instance %q: %w", name, err)
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			return pod.Name, nil
		}
	}

	return "", fmt.Errorf("no running pod found for instance %q", name)
}

