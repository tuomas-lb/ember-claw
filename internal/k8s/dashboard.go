package k8s

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// DashboardName is the fixed name of the fleet dashboard resources.
	DashboardName = "dashboard"
	// DashboardPort is the dashboard HTTP port.
	DashboardPort = 8090
)

// DashboardOptions configures a fleet dashboard deployment.
type DashboardOptions struct {
	Host          string // Ingress hostname (required)
	Image         string // Dashboard container image (required)
	SidecarImage  string // Image the dashboard deploys for NEW instances (SIDECAR_IMAGE)
	Issuer        string // cert-manager ClusterIssuer for the public TLS cert
	Class         string // Ingress class (default nginx)
	MTLSCAPEM     []byte // PEM of the CA cert for client-cert auth (optional but recommended)
	WithPostgres  bool   // Deploy Postgres + wire DATABASE_URL for chat persistence
	PostgresImage string // Postgres image (default postgres:16-alpine)
	StorageClass  string // Storage class for the Postgres PVC (cluster default if empty)
}

func dashboardLabels() map[string]string {
	return map[string]string{
		"app":                          DashboardName,
		"app.kubernetes.io/managed-by": ManagedByValue,
		"app.kubernetes.io/component":  "fleet-dashboard",
	}
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// DeployDashboard creates or updates the fleet dashboard (ServiceAccount, Role,
// RoleBinding, Deployment, Service, Ingress) plus, optionally, an mTLS CA secret
// and a Postgres instance for chat persistence. All resources are namespace-scoped
// to the client's namespace (least privilege). Upsert semantics throughout.
func (c *Client) DeployDashboard(ctx context.Context, opts DashboardOptions) error {
	if opts.Host == "" {
		return fmt.Errorf("host is required")
	}
	if opts.Image == "" {
		return fmt.Errorf("dashboard image is required")
	}
	if opts.Class == "" {
		opts.Class = "nginx"
	}
	if err := c.EnsureNamespace(ctx); err != nil {
		return fmt.Errorf("ensure namespace: %w", err)
	}
	labels := dashboardLabels()

	// 1. ServiceAccount + namespace-scoped Role + RoleBinding.
	sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: DashboardName, Namespace: c.namespace, Labels: labels}}
	if _, err := c.cs.CoreV1().ServiceAccounts(c.namespace).Create(ctx, sa, metav1.CreateOptions{}); err != nil && !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("create serviceaccount: %w", err)
	}
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: DashboardName, Namespace: c.namespace, Labels: labels},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get", "list", "watch", "create", "update", "patch", "delete"}},
			{APIGroups: []string{""}, Resources: []string{"services", "persistentvolumeclaims"}, Verbs: []string{"get", "list", "create", "delete"}},
			{APIGroups: []string{""}, Resources: []string{"secrets", "configmaps"}, Verbs: []string{"get", "list", "create", "update", "delete"}},
			{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list"}},
			{APIGroups: []string{""}, Resources: []string{"pods/log"}, Verbs: []string{"get", "list"}},
		},
	}
	if err := upsertRole(ctx, c, role); err != nil {
		return err
	}
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: DashboardName, Namespace: c.namespace, Labels: labels},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: DashboardName},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: DashboardName, Namespace: c.namespace}},
	}
	if _, err := c.cs.RbacV1().RoleBindings(c.namespace).Create(ctx, rb, metav1.CreateOptions{}); err != nil && !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("create rolebinding: %w", err)
	}

	// 2. Optional Postgres (created before the dashboard so DATABASE_URL resolves).
	env := []corev1.EnvVar{
		{Name: "NAMESPACE", Value: c.namespace},
		{Name: "ADDR", Value: fmt.Sprintf(":%d", DashboardPort)},
	}
	if opts.SidecarImage != "" {
		env = append(env, corev1.EnvVar{Name: "SIDECAR_IMAGE", Value: opts.SidecarImage})
	}
	if opts.WithPostgres {
		if err := c.ensurePostgres(ctx, opts, labels); err != nil {
			return err
		}
		env = append(env, corev1.EnvVar{
			Name: "DATABASE_URL",
			ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "dashboard-postgres"},
				Key:                  "database_url",
			}},
		})
	}

	// 3. Deployment.
	var pullSecrets []corev1.LocalObjectReference
	if c.hasRegistrySecret(ctx) {
		pullSecrets = []corev1.LocalObjectReference{{Name: RegistrySecretName}}
	}
	replicas := int32(1)
	probe := &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/api/providers", Port: intstr.FromInt32(DashboardPort)}}, InitialDelaySeconds: 5, PeriodSeconds: 20}
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: DashboardName, Namespace: c.namespace, Labels: labels},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": DashboardName}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					ServiceAccountName: DashboardName,
					ImagePullSecrets:   pullSecrets,
					Containers: []corev1.Container{{
						Name:            DashboardName,
						Image:           opts.Image,
						ImagePullPolicy: corev1.PullIfNotPresent,
						Ports:           []corev1.ContainerPort{{Name: "http", ContainerPort: DashboardPort}},
						Env:             env,
						LivenessProbe:   probe,
						ReadinessProbe:  probe,
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("128Mi")},
							Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("256Mi")},
						},
					}},
				},
			},
		},
	}
	if err := upsertDeployment(ctx, c, deploy); err != nil {
		return err
	}

	// 4. Service.
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: DashboardName, Namespace: c.namespace, Labels: labels},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: map[string]string{"app": DashboardName},
			Ports:    []corev1.ServicePort{{Name: "http", Port: DashboardPort, TargetPort: intstr.FromInt32(DashboardPort)}},
		},
	}
	if _, err := c.cs.CoreV1().Services(c.namespace).Create(ctx, svc, metav1.CreateOptions{}); err != nil && !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("create service: %w", err)
	}

	// 5. mTLS CA secret + Ingress.
	annotations := map[string]string{
		"nginx.ingress.kubernetes.io/proxy-read-timeout": "86400",
		"nginx.ingress.kubernetes.io/proxy-send-timeout": "86400",
	}
	if opts.Issuer != "" {
		annotations["cert-manager.io/cluster-issuer"] = opts.Issuer
	}
	if len(opts.MTLSCAPEM) > 0 {
		caSecret := DashboardName + "-mtls-ca"
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: caSecret, Namespace: c.namespace, Labels: labels},
			Data:       map[string][]byte{"ca.crt": opts.MTLSCAPEM},
		}
		if _, err := c.cs.CoreV1().Secrets(c.namespace).Create(ctx, s, metav1.CreateOptions{}); k8serrors.IsAlreadyExists(err) {
			if _, err := c.cs.CoreV1().Secrets(c.namespace).Update(ctx, s, metav1.UpdateOptions{}); err != nil {
				return fmt.Errorf("update mtls ca secret: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("create mtls ca secret: %w", err)
		}
		annotations["nginx.ingress.kubernetes.io/auth-tls-secret"] = c.namespace + "/" + caSecret
		annotations["nginx.ingress.kubernetes.io/auth-tls-verify-client"] = "on"
		annotations["nginx.ingress.kubernetes.io/auth-tls-verify-depth"] = "1"
	}
	pathType := networkingv1.PathTypePrefix
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: DashboardName, Namespace: c.namespace, Labels: labels, Annotations: annotations},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &opts.Class,
			Rules: []networkingv1.IngressRule{{
				Host: opts.Host,
				IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{Paths: []networkingv1.HTTPIngressPath{{
					Path: "/", PathType: &pathType,
					Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{Name: DashboardName, Port: networkingv1.ServiceBackendPort{Number: DashboardPort}}},
				}}}},
			}},
			TLS: []networkingv1.IngressTLS{{Hosts: []string{opts.Host}, SecretName: DashboardName + "-tls"}},
		},
	}
	if _, err := c.cs.NetworkingV1().Ingresses(c.namespace).Create(ctx, ing, metav1.CreateOptions{}); k8serrors.IsAlreadyExists(err) {
		existing, getErr := c.cs.NetworkingV1().Ingresses(c.namespace).Get(ctx, DashboardName, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("get ingress: %w", getErr)
		}
		ing.ResourceVersion = existing.ResourceVersion
		if _, err := c.cs.NetworkingV1().Ingresses(c.namespace).Update(ctx, ing, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update ingress: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("create ingress: %w", err)
	}

	return nil
}

// ensurePostgres deploys a single-replica Postgres for chat persistence and a
// secret holding the password + full connection string. The password is only
// generated when the secret does not already exist (idempotent, preserves data).
func (c *Client) ensurePostgres(ctx context.Context, opts DashboardOptions, labels map[string]string) error {
	image := opts.PostgresImage
	if image == "" {
		image = "postgres:16-alpine"
	}
	const svcName = "postgres"
	const secretName = "dashboard-postgres"

	if _, err := c.cs.CoreV1().Secrets(c.namespace).Get(ctx, secretName, metav1.GetOptions{}); k8serrors.IsNotFound(err) {
		pw, err := randomHex(20)
		if err != nil {
			return err
		}
		dbURL := fmt.Sprintf("postgres://clivia:%s@%s.%s.svc.cluster.local:5432/clivia?sslmode=disable", pw, svcName, c.namespace)
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: c.namespace, Labels: labels},
			StringData: map[string]string{"password": pw, "database_url": dbURL},
		}
		if _, err := c.cs.CoreV1().Secrets(c.namespace).Create(ctx, s, metav1.CreateOptions{}); err != nil && !k8serrors.IsAlreadyExists(err) {
			return fmt.Errorf("create postgres secret: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("check postgres secret: %w", err)
	}

	// PVC (immutable; skip if exists).
	pvcSpec := corev1.PersistentVolumeClaimSpec{
		AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		Resources:   corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("5Gi")}},
	}
	if opts.StorageClass != "" {
		pvcSpec.StorageClassName = &opts.StorageClass
	}
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "dashboard-postgres-data", Namespace: c.namespace, Labels: labels}, Spec: pvcSpec}
	if _, err := c.cs.CoreV1().PersistentVolumeClaims(c.namespace).Create(ctx, pvc, metav1.CreateOptions{}); err != nil && !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("create postgres pvc: %w", err)
	}

	pgLabels := map[string]string{"app": svcName}
	replicas := int32(1)
	fsGroup := int64(999)
	probe := &corev1.Probe{ProbeHandler: corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: []string{"sh", "-c", "pg_isready -U clivia -d clivia"}}}, InitialDelaySeconds: 5, PeriodSeconds: 10}
	pgDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: svcName, Namespace: c.namespace, Labels: pgLabels},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			Selector: &metav1.LabelSelector{MatchLabels: pgLabels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: pgLabels},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{FSGroup: &fsGroup},
					Containers: []corev1.Container{{
						Name:  svcName,
						Image: image,
						Ports: []corev1.ContainerPort{{Name: "postgres", ContainerPort: 5432}},
						Env: []corev1.EnvVar{
							{Name: "POSTGRES_USER", Value: "clivia"},
							{Name: "POSTGRES_DB", Value: "clivia"},
							{Name: "POSTGRES_PASSWORD", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: secretName}, Key: "password"}}},
							{Name: "PGDATA", Value: "/var/lib/postgresql/data/pgdata"},
						},
						VolumeMounts:   []corev1.VolumeMount{{Name: "data", MountPath: "/var/lib/postgresql/data"}},
						ReadinessProbe: probe,
						LivenessProbe:  probe,
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("128Mi")},
							Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("512Mi")},
						},
					}},
					Volumes: []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "dashboard-postgres-data"}}}},
				},
			},
		},
	}
	if err := upsertDeployment(ctx, c, pgDeploy); err != nil {
		return fmt.Errorf("postgres deployment: %w", err)
	}
	pgSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: svcName, Namespace: c.namespace, Labels: pgLabels},
		Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, Selector: pgLabels, Ports: []corev1.ServicePort{{Name: "postgres", Port: 5432, TargetPort: intstr.FromInt32(5432)}}},
	}
	if _, err := c.cs.CoreV1().Services(c.namespace).Create(ctx, pgSvc, metav1.CreateOptions{}); err != nil && !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("create postgres service: %w", err)
	}
	return nil
}

// DeleteDashboard removes the dashboard resources (and, if present, Postgres).
// The Postgres PVC is left in place to avoid accidental chat-history loss.
func (c *Client) DeleteDashboard(ctx context.Context, includePostgres bool) error {
	opts := metav1.DeleteOptions{}
	_ = c.cs.NetworkingV1().Ingresses(c.namespace).Delete(ctx, DashboardName, opts)
	_ = c.cs.CoreV1().Services(c.namespace).Delete(ctx, DashboardName, opts)
	_ = c.cs.AppsV1().Deployments(c.namespace).Delete(ctx, DashboardName, opts)
	_ = c.cs.RbacV1().RoleBindings(c.namespace).Delete(ctx, DashboardName, opts)
	_ = c.cs.RbacV1().Roles(c.namespace).Delete(ctx, DashboardName, opts)
	_ = c.cs.CoreV1().ServiceAccounts(c.namespace).Delete(ctx, DashboardName, opts)
	_ = c.cs.CoreV1().Secrets(c.namespace).Delete(ctx, DashboardName+"-mtls-ca", opts)
	if includePostgres {
		_ = c.cs.AppsV1().Deployments(c.namespace).Delete(ctx, "postgres", opts)
		_ = c.cs.CoreV1().Services(c.namespace).Delete(ctx, "postgres", opts)
		// Secret + PVC intentionally retained (chat history / credentials).
	}
	return nil
}

func upsertRole(ctx context.Context, c *Client, role *rbacv1.Role) error {
	if _, err := c.cs.RbacV1().Roles(c.namespace).Create(ctx, role, metav1.CreateOptions{}); k8serrors.IsAlreadyExists(err) {
		if _, err := c.cs.RbacV1().Roles(c.namespace).Update(ctx, role, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update role: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("create role: %w", err)
	}
	return nil
}

func upsertDeployment(ctx context.Context, c *Client, d *appsv1.Deployment) error {
	if _, err := c.cs.AppsV1().Deployments(c.namespace).Create(ctx, d, metav1.CreateOptions{}); k8serrors.IsAlreadyExists(err) {
		if _, err := c.cs.AppsV1().Deployments(c.namespace).Update(ctx, d, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update deployment %s: %w", d.Name, err)
		}
	} else if err != nil {
		return fmt.Errorf("create deployment %s: %w", d.Name, err)
	}
	return nil
}

// DashboardImageDefault resolves the dashboard image the same way deploy resolves
// the sidecar image: explicit value, else ECLAW_DASHBOARD_IMAGE, else registry+name.
func DashboardImageDefault(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if v := os.Getenv("ECLAW_DASHBOARD_IMAGE"); v != "" {
		return v
	}
	if reg := os.Getenv("IMAGE_REGISTRY"); reg != "" {
		return reg + "/picoclaw-dashboard:latest"
	}
	return ""
}
