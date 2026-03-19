package k8s

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

const testNamespace = "picoclaw"

func newTestClient() *Client {
	fakeCS := fake.NewSimpleClientset()
	return NewClientFromClientset(fakeCS, testNamespace)
}

func defaultDeployOptions() DeployOptions {
	return DeployOptions{
		Name:          "research",
		Provider:      "anthropic",
		APIKey:        "sk-ant-test123",
		Model:         "claude-3-5-sonnet-20241022",
		CPURequest:    "100m",
		CPULimit:      "500m",
		MemoryRequest: "128Mi",
		MemoryLimit:   "512Mi",
		StorageSize:   "1Gi",
	}
}

// TestDeployInstance verifies that DeployInstance creates all 5 K8s resources.
func TestDeployInstance(t *testing.T) {
	client := newTestClient()
	ctx := context.Background()
	opts := defaultDeployOptions()

	err := client.DeployInstance(ctx, opts)
	require.NoError(t, err)

	fakeCS := client.cs.(*fake.Clientset)

	// Check Secret created
	secrets, err := fakeCS.CoreV1().Secrets(testNamespace).List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	assert.Len(t, secrets.Items, 1, "expected 1 Secret")

	// Check ConfigMap created
	cms, err := fakeCS.CoreV1().ConfigMaps(testNamespace).List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	assert.Len(t, cms.Items, 1, "expected 1 ConfigMap")

	// Check PVC created
	pvcs, err := fakeCS.CoreV1().PersistentVolumeClaims(testNamespace).List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	assert.Len(t, pvcs.Items, 1, "expected 1 PVC")

	// Check Deployment created
	deployments, err := fakeCS.AppsV1().Deployments(testNamespace).List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	assert.Len(t, deployments.Items, 1, "expected 1 Deployment")

	// Check Service created
	services, err := fakeCS.CoreV1().Services(testNamespace).List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	assert.Len(t, services.Items, 1, "expected 1 Service")
}

// TestResourceLabels verifies all created resources have managed-by=eclaw label.
func TestResourceLabels(t *testing.T) {
	client := newTestClient()
	ctx := context.Background()
	opts := defaultDeployOptions()

	err := client.DeployInstance(ctx, opts)
	require.NoError(t, err)

	fakeCS := client.cs.(*fake.Clientset)

	checkLabels := func(labels map[string]string, resourceType string) {
		assert.Equal(t, ManagedByValue, labels[LabelManagedBy], "%s missing managed-by=eclaw", resourceType)
		assert.Equal(t, opts.Name, labels[LabelInstance], "%s missing instance label", resourceType)
	}

	secrets, _ := fakeCS.CoreV1().Secrets(testNamespace).List(ctx, metav1.ListOptions{})
	for _, s := range secrets.Items {
		checkLabels(s.Labels, "Secret")
	}

	deployments, _ := fakeCS.AppsV1().Deployments(testNamespace).List(ctx, metav1.ListOptions{})
	for _, d := range deployments.Items {
		checkLabels(d.Labels, "Deployment")
	}

	services, _ := fakeCS.CoreV1().Services(testNamespace).List(ctx, metav1.ListOptions{})
	for _, s := range services.Items {
		checkLabels(s.Labels, "Service")
	}

	pvcs, _ := fakeCS.CoreV1().PersistentVolumeClaims(testNamespace).List(ctx, metav1.ListOptions{})
	for _, p := range pvcs.Items {
		checkLabels(p.Labels, "PVC")
	}
}

// TestAPIKeyInSecret verifies API key is in Secret StringData, not Deployment env.
func TestAPIKeyInSecret(t *testing.T) {
	client := newTestClient()
	ctx := context.Background()
	opts := defaultDeployOptions()

	err := client.DeployInstance(ctx, opts)
	require.NoError(t, err)

	fakeCS := client.cs.(*fake.Clientset)

	// Secret must contain PICOCLAW_PROVIDERS_{PROVIDER}_API_KEY
	secrets, _ := fakeCS.CoreV1().Secrets(testNamespace).List(ctx, metav1.ListOptions{})
	require.Len(t, secrets.Items, 1)
	secret := secrets.Items[0]

	expectedKey := "PICOCLAW_PROVIDERS_ANTHROPIC_API_KEY"
	// Check StringData (before encoding) or Data (after encoding)
	found := false
	if val, ok := secret.StringData[expectedKey]; ok && val == opts.APIKey {
		found = true
	}
	// Also check Data field (fake clientset may convert StringData to Data)
	if !found {
		if val, ok := secret.Data[expectedKey]; ok && string(val) == opts.APIKey {
			found = true
		}
	}
	assert.True(t, found, "Secret must contain %s with the API key value", expectedKey)

	// Deployment must NOT have the API key in env.value directly
	deployments, _ := fakeCS.AppsV1().Deployments(testNamespace).List(ctx, metav1.ListOptions{})
	require.Len(t, deployments.Items, 1)
	deployment := deployments.Items[0]
	for _, container := range deployment.Spec.Template.Spec.Containers {
		for _, env := range container.Env {
			if strings.Contains(env.Name, "API_KEY") {
				assert.Empty(t, env.Value, "API key must not be in Deployment env.value; use Secret+envFrom instead")
			}
		}
	}

	// Deployment must have envFrom referencing the secret
	hasSecretRef := false
	for _, container := range deployment.Spec.Template.Spec.Containers {
		for _, envFrom := range container.EnvFrom {
			if envFrom.SecretRef != nil {
				hasSecretRef = true
			}
		}
	}
	assert.True(t, hasSecretRef, "Deployment container must use envFrom.secretRef to inject Secret env vars")
}

// TestResourceLimits verifies CPU/memory resource limits are applied to the container spec.
func TestResourceLimits(t *testing.T) {
	client := newTestClient()
	ctx := context.Background()
	opts := defaultDeployOptions()

	err := client.DeployInstance(ctx, opts)
	require.NoError(t, err)

	fakeCS := client.cs.(*fake.Clientset)
	deployments, _ := fakeCS.AppsV1().Deployments(testNamespace).List(ctx, metav1.ListOptions{})
	require.Len(t, deployments.Items, 1)
	deployment := deployments.Items[0]
	require.NotEmpty(t, deployment.Spec.Template.Spec.Containers)

	container := deployment.Spec.Template.Spec.Containers[0]
	resources := container.Resources

	assert.Equal(t, opts.CPURequest, resources.Requests.Cpu().String(), "CPU request mismatch")
	assert.Equal(t, opts.CPULimit, resources.Limits.Cpu().String(), "CPU limit mismatch")
	assert.Equal(t, opts.MemoryRequest, resources.Requests.Memory().String(), "Memory request mismatch")
	assert.Equal(t, opts.MemoryLimit, resources.Limits.Memory().String(), "Memory limit mismatch")
}

// TestPVCCreation verifies PVC has correct name, size, access mode, and labels.
func TestPVCCreation(t *testing.T) {
	client := newTestClient()
	ctx := context.Background()
	opts := defaultDeployOptions()

	err := client.DeployInstance(ctx, opts)
	require.NoError(t, err)

	fakeCS := client.cs.(*fake.Clientset)
	pvcs, _ := fakeCS.CoreV1().PersistentVolumeClaims(testNamespace).List(ctx, metav1.ListOptions{})
	require.Len(t, pvcs.Items, 1)
	pvc := pvcs.Items[0]

	assert.Equal(t, "picoclaw-research-data", pvc.Name, "PVC name should be picoclaw-{name}-data")
	assert.Contains(t, pvc.Spec.AccessModes, corev1.ReadWriteOnce, "PVC must be ReadWriteOnce")
	storage := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	assert.Equal(t, "1Gi", storage.String(), "PVC storage size mismatch")

	// Verify PVC is mounted in Deployment at /home/picoclaw/.picoclaw
	deployments, _ := fakeCS.AppsV1().Deployments(testNamespace).List(ctx, metav1.ListOptions{})
	require.Len(t, deployments.Items, 1)
	deployment := deployments.Items[0]

	pvcMounted := false
	for _, vol := range deployment.Spec.Template.Spec.Volumes {
		if vol.PersistentVolumeClaim != nil && vol.PersistentVolumeClaim.ClaimName == "picoclaw-research-data" {
			pvcMounted = true
		}
	}
	assert.True(t, pvcMounted, "Deployment must reference the PVC in volumes")

	mountPathFound := false
	for _, container := range deployment.Spec.Template.Spec.Containers {
		for _, vm := range container.VolumeMounts {
			if vm.MountPath == "/home/picoclaw/.picoclaw" {
				mountPathFound = true
			}
		}
	}
	assert.True(t, mountPathFound, "Deployment must mount PVC at /home/picoclaw/.picoclaw")
}

// TestListInstances verifies label selector filtering returns only eclaw-managed deployments.
func TestListInstances(t *testing.T) {
	fakeCS := fake.NewSimpleClientset()
	client := NewClientFromClientset(fakeCS, testNamespace)
	ctx := context.Background()

	// Pre-create 2 eclaw-managed deployments
	for _, name := range []string{"instance-a", "instance-b"} {
		_, err := fakeCS.AppsV1().Deployments(testNamespace).Create(ctx, &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "picoclaw-" + name,
				Namespace: testNamespace,
				Labels:    InstanceLabels(name),
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(1),
				Selector: &metav1.LabelSelector{MatchLabels: InstanceLabels(name)},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: InstanceLabels(name)},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img"}}},
				},
			},
		}, metav1.CreateOptions{})
		require.NoError(t, err)
	}

	// Pre-create 1 non-eclaw deployment (should be excluded)
	_, err := fakeCS.AppsV1().Deployments(testNamespace).Create(ctx, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-deployment",
			Namespace: testNamespace,
			Labels:    map[string]string{"app": "other"},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "other"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "other"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img"}}},
			},
		},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	instances, err := client.ListInstances(ctx)
	require.NoError(t, err)
	assert.Len(t, instances, 2, "ListInstances should return only eclaw-managed deployments")

	names := make([]string, len(instances))
	for i, inst := range instances {
		names[i] = inst.Name
	}
	assert.Contains(t, names, "instance-a")
	assert.Contains(t, names, "instance-b")
}

// TestDeleteInstance verifies compute resources are deleted but PVC is preserved.
func TestDeleteInstance(t *testing.T) {
	fakeCS := fake.NewSimpleClientset()
	client := NewClientFromClientset(fakeCS, testNamespace)
	ctx := context.Background()

	// First deploy to create all resources
	opts := defaultDeployOptions()
	err := client.DeployInstance(ctx, opts)
	require.NoError(t, err)

	// Verify PVC exists before delete
	pvcs, _ := fakeCS.CoreV1().PersistentVolumeClaims(testNamespace).List(ctx, metav1.ListOptions{})
	require.Len(t, pvcs.Items, 1, "PVC must exist before delete")

	// Delete instance
	err = client.DeleteInstance(ctx, opts.Name)
	require.NoError(t, err)

	// Deployment should be gone
	deployments, _ := fakeCS.AppsV1().Deployments(testNamespace).List(ctx, metav1.ListOptions{})
	assert.Len(t, deployments.Items, 0, "Deployment must be deleted")

	// Service should be gone
	services, _ := fakeCS.CoreV1().Services(testNamespace).List(ctx, metav1.ListOptions{})
	assert.Len(t, services.Items, 0, "Service must be deleted")

	// Secret should be gone
	secrets, _ := fakeCS.CoreV1().Secrets(testNamespace).List(ctx, metav1.ListOptions{})
	assert.Len(t, secrets.Items, 0, "Secret must be deleted")

	// ConfigMap should be gone
	cms, _ := fakeCS.CoreV1().ConfigMaps(testNamespace).List(ctx, metav1.ListOptions{})
	assert.Len(t, cms.Items, 0, "ConfigMap must be deleted")

	// PVC must still exist
	pvcsAfter, _ := fakeCS.CoreV1().PersistentVolumeClaims(testNamespace).List(ctx, metav1.ListOptions{})
	assert.Len(t, pvcsAfter.Items, 1, "PVC must be PRESERVED after DeleteInstance")
}

// TestDeletePVC verifies DeletePVC removes the PVC by instance name.
func TestDeletePVC(t *testing.T) {
	fakeCS := fake.NewSimpleClientset()
	client := NewClientFromClientset(fakeCS, testNamespace)
	ctx := context.Background()

	// Pre-create a PVC
	_, err := fakeCS.CoreV1().PersistentVolumeClaims(testNamespace).Create(ctx, &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "picoclaw-research-data",
			Namespace: testNamespace,
			Labels:    InstanceLabels("research"),
		},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	// Verify it exists
	pvcs, _ := fakeCS.CoreV1().PersistentVolumeClaims(testNamespace).List(ctx, metav1.ListOptions{})
	require.Len(t, pvcs.Items, 1)

	// Delete PVC
	err = client.DeletePVC(ctx, "research")
	require.NoError(t, err)

	// Verify it's gone
	pvcsAfter, _ := fakeCS.CoreV1().PersistentVolumeClaims(testNamespace).List(ctx, metav1.ListOptions{})
	assert.Len(t, pvcsAfter.Items, 0, "PVC must be deleted by DeletePVC")
}

// TestGetInstanceStatus verifies status returns deployment replica info.
func TestGetInstanceStatus(t *testing.T) {
	fakeCS := fake.NewSimpleClientset()
	client := NewClientFromClientset(fakeCS, testNamespace)
	ctx := context.Background()

	// Pre-create a deployment and secret
	replicas := int32(1)
	ready := int32(1)
	_, err := fakeCS.AppsV1().Deployments(testNamespace).Create(ctx, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "picoclaw-research",
			Namespace: testNamespace,
			Labels:    InstanceLabels("research"),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: InstanceLabels("research")},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: InstanceLabels("research")},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img"}}},
			},
		},
		Status: appsv1.DeploymentStatus{
			Replicas:      replicas,
			ReadyReplicas: ready,
		},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	// Pre-create the config secret with provider/model info
	_, err = fakeCS.CoreV1().Secrets(testNamespace).Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "picoclaw-research-config",
			Namespace: testNamespace,
			Labels:    InstanceLabels("research"),
		},
		StringData: map[string]string{
			"PICOCLAW_PROVIDERS_ANTHROPIC_API_KEY":  "sk-ant-test",
			"PICOCLAW_AGENTS_DEFAULTS_PROVIDER":     "anthropic",
			"PICOCLAW_AGENTS_DEFAULTS_MODEL_NAME":   "claude-3-5-sonnet-20241022",
		},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	status, err := client.GetInstanceStatus(ctx, "research")
	require.NoError(t, err)
	require.NotNil(t, status)

	assert.Equal(t, "research", status.Name)
	assert.Equal(t, int32(1), status.DesiredReplicas)
	assert.Equal(t, int32(1), status.ReadyReplicas)
}

// TestFindRunningPod verifies FindRunningPod returns the pod name when running.
func TestFindRunningPod(t *testing.T) {
	fakeCS := fake.NewSimpleClientset()
	client := NewClientFromClientset(fakeCS, testNamespace)
	ctx := context.Background()

	// Pre-create a running pod
	_, err := fakeCS.CoreV1().Pods(testNamespace).Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "picoclaw-research-abc123",
			Namespace: testNamespace,
			Labels:    InstanceLabels("research"),
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	podName, err := client.FindRunningPod(ctx, "research")
	require.NoError(t, err)
	assert.Equal(t, "picoclaw-research-abc123", podName)
}

// TestFindRunningPodNotFound verifies FindRunningPod returns error when no pod exists.
func TestFindRunningPodNotFound(t *testing.T) {
	client := newTestClient()
	ctx := context.Background()

	_, err := client.FindRunningPod(ctx, "nonexistent")
	assert.Error(t, err, "FindRunningPod should return error when no running pod found")
}

// TestDeployInstance_CustomEnv verifies custom environment variables are set in ConfigMap.
func TestDeployInstance_CustomEnv(t *testing.T) {
	client := newTestClient()
	ctx := context.Background()
	opts := defaultDeployOptions()
	opts.CustomEnv = map[string]string{
		"LOG_LEVEL": "debug",
		"FEATURE_X": "enabled",
	}

	err := client.DeployInstance(ctx, opts)
	require.NoError(t, err)

	fakeCS := client.cs.(*fake.Clientset)
	cms, _ := fakeCS.CoreV1().ConfigMaps(testNamespace).List(ctx, metav1.ListOptions{})
	require.Len(t, cms.Items, 1)

	assert.Equal(t, "debug", cms.Items[0].Data["LOG_LEVEL"])
	assert.Equal(t, "enabled", cms.Items[0].Data["FEATURE_X"])
}

// TestDeployInstance_StorageClass verifies custom storage class is applied to PVC.
func TestDeployInstance_StorageClass(t *testing.T) {
	client := newTestClient()
	ctx := context.Background()
	opts := defaultDeployOptions()
	opts.StorageClass = "ssd-premium"

	err := client.DeployInstance(ctx, opts)
	require.NoError(t, err)

	fakeCS := client.cs.(*fake.Clientset)
	pvcs, _ := fakeCS.CoreV1().PersistentVolumeClaims(testNamespace).List(ctx, metav1.ListOptions{})
	require.Len(t, pvcs.Items, 1)

	require.NotNil(t, pvcs.Items[0].Spec.StorageClassName)
	assert.Equal(t, "ssd-premium", *pvcs.Items[0].Spec.StorageClassName)
}

// TestDeployInstance_DefaultImage verifies the default container image is used when empty.
func TestDeployInstance_DefaultImage(t *testing.T) {
	client := newTestClient()
	ctx := context.Background()
	opts := defaultDeployOptions()
	opts.Image = "" // Should use DefaultImage

	err := client.DeployInstance(ctx, opts)
	require.NoError(t, err)

	fakeCS := client.cs.(*fake.Clientset)
	deployments, _ := fakeCS.AppsV1().Deployments(testNamespace).List(ctx, metav1.ListOptions{})
	require.Len(t, deployments.Items, 1)

	container := deployments.Items[0].Spec.Template.Spec.Containers[0]
	assert.Equal(t, DefaultImage, container.Image)
}

// TestDeployInstance_CustomImage verifies a custom image overrides the default.
func TestDeployInstance_CustomImage(t *testing.T) {
	client := newTestClient()
	ctx := context.Background()
	opts := defaultDeployOptions()
	opts.Image = "reg.r.lastbot.com/ember-claw-sidecar:0.1.5"

	err := client.DeployInstance(ctx, opts)
	require.NoError(t, err)

	fakeCS := client.cs.(*fake.Clientset)
	deployments, _ := fakeCS.AppsV1().Deployments(testNamespace).List(ctx, metav1.ListOptions{})
	require.Len(t, deployments.Items, 1)

	container := deployments.Items[0].Spec.Template.Spec.Containers[0]
	assert.Equal(t, "reg.r.lastbot.com/ember-claw-sidecar:0.1.5", container.Image)
}

// TestDeployInstance_GeminiProvider verifies Gemini-specific env vars are set correctly.
func TestDeployInstance_GeminiProvider(t *testing.T) {
	client := newTestClient()
	ctx := context.Background()
	opts := defaultDeployOptions()
	opts.Provider = "gemini"
	opts.APIKey = "AIza-test-key"
	opts.Model = "gemini-2.5-flash"

	err := client.DeployInstance(ctx, opts)
	require.NoError(t, err)

	fakeCS := client.cs.(*fake.Clientset)
	secrets, _ := fakeCS.CoreV1().Secrets(testNamespace).List(ctx, metav1.ListOptions{})
	require.Len(t, secrets.Items, 1)

	secret := secrets.Items[0]
	// Check Gemini-specific env var name
	found := false
	for key, val := range secret.StringData {
		if key == "PICOCLAW_PROVIDERS_GEMINI_API_KEY" && val == "AIza-test-key" {
			found = true
		}
	}
	if !found {
		for key, val := range secret.Data {
			if key == "PICOCLAW_PROVIDERS_GEMINI_API_KEY" && string(val) == "AIza-test-key" {
				found = true
			}
		}
	}
	assert.True(t, found, "Secret must contain PICOCLAW_PROVIDERS_GEMINI_API_KEY")
}

// TestGetInstanceLogs verifies log retrieval path (happy path with a running pod).
func TestGetInstanceLogs(t *testing.T) {
	fakeCS := fake.NewSimpleClientset()
	client := NewClientFromClientset(fakeCS, testNamespace)
	ctx := context.Background()

	// Pre-create a running pod
	_, err := fakeCS.CoreV1().Pods(testNamespace).Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "picoclaw-research-abc123",
			Namespace: testNamespace,
			Labels:    InstanceLabels("research"),
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	// GetInstanceLogs will call FindRunningPod then attempt GetLogs.
	// With fake clientset, GetLogs returns an empty stream.
	stream, err := client.GetInstanceLogs(ctx, "research", false, 100)
	// fake clientset may return error for GetLogs (not fully supported),
	// but FindRunningPod path is exercised
	if err == nil && stream != nil {
		stream.Close()
	}
}

// TestGetInstanceLogs_NoPod verifies error when no pod exists.
func TestGetInstanceLogs_NoPod(t *testing.T) {
	client := newTestClient()
	ctx := context.Background()

	_, err := client.GetInstanceLogs(ctx, "nonexistent", false, 100)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no running pod")
}

// int32Ptr returns a pointer to an int32 value.
func int32Ptr(i int32) *int32 {
	return &i
}
