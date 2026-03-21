package k8s

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// DefaultStorageSize is the default PVC size.
	DefaultStorageSize = "1Gi"
	// MountPath is the path where the PVC is mounted in the container.
	MountPath = "/home/picoclaw/.picoclaw"
	// DefaultServiceName is the sidecar image name (without registry prefix).
	DefaultServiceName = "ember-claw-sidecar"
	// DefaultImageTag is the default image tag.
	DefaultImageTag = "latest"
	// RegistrySecretName is the well-known name of the image pull secret.
	RegistrySecretName = "eclaw-registry"
)

// DeployOptions contains all configuration for deploying a PicoClaw instance.
type DeployOptions struct {
	Name          string            // Instance name (resources are prefixed with picoclaw-{name})
	Provider      string            // AI provider (anthropic, openai, etc.)
	APIKey        string            // Provider API key
	Model         string            // Model name
	Image         string            // Container image (resolved from IMAGE_REGISTRY or ECLAW_IMAGE)
	CPURequest    string            // e.g., "100m"
	CPULimit      string            // e.g., "500m"
	MemoryRequest string            // e.g., "128Mi"
	MemoryLimit   string            // e.g., "512Mi"
	StorageSize   string            // PVC size (default: "1Gi")
	StorageClass  string            // Optional storage class name
	CustomEnv     map[string]string // Additional env vars
	LinearAPIKey  string            // Linear API key (optional)
	LinearTeamID  string            // Linear team UUID (optional)
	SlackBotToken string            // Slack bot token (optional)
	MCPServers    map[string]mcpServerConfig // Additional MCP servers to include
	Identity      string            // Custom IDENTITY.md content (optional, uses default if empty)
}

// picoClawConfig is the subset of PicoClaw's config.json we generate for deployment.
// Field paths match PicoClaw's official config structure (tools.mcp, channels, gateway).
type picoClawConfig struct {
	Agents struct {
		Defaults struct {
			ModelName           string `json:"model_name"`
			Workspace           string `json:"workspace"`
			RestrictToWorkspace bool   `json:"restrict_to_workspace"`
			AllowReadOutsideWS bool   `json:"allow_read_outside_workspace"`
			MaxToolIterations   int    `json:"max_tool_iterations"`
		} `json:"defaults"`
	} `json:"agents"`
	Tools struct {
		Exec struct {
			EnableDenyPatterns bool `json:"enable_deny_patterns"`
			AllowRemote        bool `json:"allow_remote"`
		} `json:"exec"`
		MCP *mcpConfig `json:"mcp,omitempty"`
	} `json:"tools"`
	ModelList []picoClawModelEntry `json:"model_list"`
	Channels  *channelsConfig      `json:"channels,omitempty"`
	Gateway   *gatewayConfig       `json:"gateway,omitempty"`
}

// channelsConfig holds channel configuration for PicoClaw gateway mode.
type channelsConfig struct {
	Telegram *telegramChannelConfig `json:"telegram,omitempty"`
	// Other channels can be added here or configured via config push.
}

// telegramChannelConfig holds Telegram bot configuration.
type telegramChannelConfig struct {
	Enabled            bool     `json:"enabled"`
	Token              string   `json:"token"`
	AllowFrom          []string `json:"allow_from,omitempty"`
	BaseURL            string   `json:"base_url,omitempty"`
	Proxy              string   `json:"proxy,omitempty"`
	ReasoningChannelID string   `json:"reasoning_channel_id,omitempty"`
}

// gatewayConfig holds PicoClaw gateway settings.
type gatewayConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// mcpConfig holds the MCP server configuration for PicoClaw.
type mcpConfig struct {
	Enabled   bool                       `json:"enabled"`
	Discovery *mcpDiscoveryConfig        `json:"discovery,omitempty"`
	Servers   map[string]mcpServerConfig `json:"servers"`
}

type mcpDiscoveryConfig struct {
	Enabled bool `json:"enabled"`
}

// mcpServerConfig defines a single MCP server entry.
type mcpServerConfig struct {
	Enabled bool              `json:"enabled"`
	Type    string            `json:"type"`              // "stdio", "sse", "http"
	Command string            `json:"command,omitempty"` // for stdio
	Args    []string          `json:"args,omitempty"`    // for stdio
	URL     string            `json:"url,omitempty"`     // for sse/http
	Headers map[string]string `json:"headers,omitempty"` // for sse/http
	Env     map[string]string `json:"env,omitempty"`     // for stdio
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
	cfg.Agents.Defaults.RestrictToWorkspace = false
	cfg.Agents.Defaults.AllowReadOutsideWS = true
	cfg.Agents.Defaults.MaxToolIterations = 50
	// Disable command deny patterns — safe in isolated container.
	cfg.Tools.Exec.EnableDenyPatterns = false
	cfg.Tools.Exec.AllowRemote = true

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

	// Configure MCP servers — Backlog.md included by default (installed in container).
	servers := map[string]mcpServerConfig{
		"backlog": {
			Enabled: true,
			Type:    "stdio",
			Command: "backlog",
			Args:    []string{"mcp", "start"},
			Env: map[string]string{
				"BACKLOG_CWD": MountPath + "/workspace",
			},
		},
	}
	// Merge any additional MCP servers from deploy options.
	for name, srv := range opts.MCPServers {
		servers[name] = srv
	}
	cfg.Tools.MCP = &mcpConfig{
		Enabled:   true,
		Discovery: &mcpDiscoveryConfig{Enabled: false},
		Servers:   servers,
	}

	// Default gateway config for container mode (binds to all interfaces on health port).
	cfg.Gateway = &gatewayConfig{
		Host: "0.0.0.0",
		Port: 8080,
	}

	return cfg
}

// defaultIdentity returns the default IDENTITY.md content for a PicoClaw instance.
// Loaded by PicoClaw at startup — defines name, capabilities, and environment.
func defaultIdentity(instanceName string) string {
	return fmt.Sprintf(`# Identity

## Name
%s

## Description
AI assistant deployed via EmberClaw on Kubernetes.

## Environment
- Alpine Linux container with persistent storage (PVC)
- Workspace: /home/picoclaw/.picoclaw/workspace
- Available runtimes: python3, nodejs, go, shell (ash/sh)
- System tools: curl, jq, git, wget, openssh
- pip packages persist across restarts (PVC-backed via PYTHONUSERBASE)
- Pre-installed Python: requests, beautifulsoup4, pyyaml

## Capabilities
- Shell command execution (unrestricted in container)
- File read/write/edit operations
- Web search and URL fetching
- Task management via Backlog.md (MCP tools)
- Subagent spawning for parallel work
- Multi-channel messaging (gRPC, Telegram if configured)

## Limitations
- Cannot access resources outside this container/cluster without explicit network tools
- No GUI — all interaction is text-based
- Memory resets between sessions unless saved to memory/MEMORY.md or backlog
`, instanceName)
}

// defaultAgentsInstructions returns the default AGENTS.md content.
// Loaded by PicoClaw — defines procedural rules and operational directives.
func defaultAgentsInstructions() string {
	return `# Agent Instructions

## Critical Rules

1. ONLY use tools from your actual tool list — never invent tool names
2. If a tool fails, state the error in one line and suggest an alternative
3. Never claim to have done something you haven't verified
4. Never hallucinate external services, APIs, or commands that aren't installed

## Execution

- Use exec for ALL shell operations — you have full unrestricted access
- Run commands and show actual output, don't paraphrase
- Chain commands with && when they depend on each other
- Check exit codes — don't assume success
- For long-running tasks, use spawn to run in background

## Task Management

- Use mcp_backlog_task_* tools for tracking work items
- Check existing tasks before creating duplicates
- Update task status as work progresses
- Save important findings to memory/MEMORY.md for persistence

## Communication

- The "message" tool sends to the CURRENT channel (gRPC or Telegram)
- Do not attempt to use messaging tools/services that aren't in your tool list
- When on Telegram, replies go through PicoClaw's channel system automatically

## Response Style

- Be concise — answer the question, skip the preamble
- Show don't tell — run the command, show the output
- One-sentence error reports, not paragraphs of apology
- Skip "I'd be happy to" and "Sure!" — just do the thing
`
}

// defaultSoul returns the default SOUL.md content.
// Loaded by PicoClaw — defines personality traits and values.
func defaultSoul() string {
	return `# Soul

## Personality
- Direct and efficient — respects the user's time
- Curious — investigates thoroughly before answering
- Honest — admits uncertainty rather than guessing
- Proactive — anticipates follow-up needs

## Values
1. Accuracy over speed — verify before claiming
2. Action over discussion — do it, don't explain how you would do it
3. Transparency — show your work (commands, outputs, reasoning)
4. User autonomy — present options when choices exist

## Communication Style
- Technical but accessible
- Terse for simple tasks, detailed for complex ones
- Code and output speak louder than explanations
- No filler words, no excessive politeness
`
}

// defaultUser returns the default USER.md content.
// Loaded by PicoClaw — user profile template for personalization.
func defaultUser() string {
	return `# User

## Preferences
- Communication style: technical, concise
- Show command output: always

## Notes
Update this file with user-specific information as you learn it.
Record timezone, language, name, and work context here.
`
}

// InstanceSummary holds a brief summary of a PicoClaw instance for list output.
type InstanceSummary struct {
	Name            string
	DeploymentName  string
	DesiredReplicas int32
	ReadyReplicas   int32
	PodPhase        corev1.PodPhase // Actual pod phase (more reliable than deployment status on some clusters)
	ContainerState  string          // e.g. "Running", "CrashLoopBackOff", "ImagePullBackOff", "Waiting"
	Restarts        int32           // Total container restart count
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
		return fmt.Errorf("image is required: set ECLAW_IMAGE or IMAGE_REGISTRY in .env, or use --image flag")
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

	secretData := map[string]string{
		"config.json": string(configJSON),
	}
	// Add optional integration credentials as env vars (picked up by sidecar container).
	if opts.LinearAPIKey != "" {
		secretData["LINEAR_API_KEY"] = opts.LinearAPIKey
	}
	if opts.LinearTeamID != "" {
		secretData["LINEAR_TEAM_ID"] = opts.LinearTeamID
	}
	if opts.SlackBotToken != "" {
		secretData["SLACK_BOT_TOKEN"] = opts.SlackBotToken
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configSecretName,
			Namespace: c.namespace,
			Labels:    instanceLabels,
		},
		StringData: secretData,
	}
	if _, err := c.cs.CoreV1().Secrets(c.namespace).Create(ctx, secret, metav1.CreateOptions{}); k8serrors.IsAlreadyExists(err) {
		if _, err := c.cs.CoreV1().Secrets(c.namespace).Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update secret: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("create secret: %w", err)
	}

	// 2a. Create or update ConfigMap for bootstrap files (IDENTITY.md, AGENTS.md).
	// These are PicoClaw's "system prompt" — loaded at startup from workspace directory.
	bootstrapCMName := baseName + "-bootstrap"
	identityContent := opts.Identity
	if identityContent == "" {
		identityContent = defaultIdentity(opts.Name)
	}
	bootstrapCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bootstrapCMName,
			Namespace: c.namespace,
			Labels:    instanceLabels,
		},
		Data: map[string]string{
			"IDENTITY.md": identityContent,
			"AGENTS.md":   defaultAgentsInstructions(),
			"SOUL.md":     defaultSoul(),
			"USER.md":     defaultUser(),
		},
	}
	if _, err := c.cs.CoreV1().ConfigMaps(c.namespace).Create(ctx, bootstrapCM, metav1.CreateOptions{}); k8serrors.IsAlreadyExists(err) {
		if _, err := c.cs.CoreV1().ConfigMaps(c.namespace).Update(ctx, bootstrapCM, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update bootstrap configmap: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("create bootstrap configmap: %w", err)
	}

	// 2b. Create or update ConfigMap for custom environment variables (CONF-04).
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
	// Add imagePullSecrets if registry credentials exist in the namespace.
	var imagePullSecrets []corev1.LocalObjectReference
	if c.hasRegistrySecret(ctx) {
		imagePullSecrets = []corev1.LocalObjectReference{
			{Name: RegistrySecretName},
		}
	}

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
					ImagePullSecrets: imagePullSecrets,
					// Init container: copy bootstrap files to workspace (only if not already present).
					InitContainers: []corev1.Container{
						{
							Name:  "bootstrap",
							Image: "alpine:3.21",
							Command: []string{"sh", "-c", `
								mkdir -p /workspace/workspace /workspace/workspace/memory &&
								for f in IDENTITY.md AGENTS.md SOUL.md USER.md; do
									if [ ! -f "/workspace/workspace/$f" ]; then
										cp "/bootstrap/$f" "/workspace/workspace/$f"
										echo "Installed $f"
									else
										echo "$f already exists, skipping"
									fi
								done
								if [ ! -f "/workspace/workspace/memory/MEMORY.md" ]; then
									echo "# Long-term Memory" > /workspace/workspace/memory/MEMORY.md
									echo "" >> /workspace/workspace/memory/MEMORY.md
									echo "Persistent memory across sessions." >> /workspace/workspace/memory/MEMORY.md
									echo "Installed memory/MEMORY.md"
								fi
							`},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "data", MountPath: "/workspace"},
								{Name: "bootstrap", MountPath: "/bootstrap", ReadOnly: true},
							},
						},
					},
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
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: configSecretName,
										},
									},
								},
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
						{
							Name: "bootstrap",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: bootstrapCMName,
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
	type podInfo struct {
		Phase          corev1.PodPhase
		ContainerState string
		Restarts       int32
	}
	podInfoByInstance := make(map[string]podInfo)
	if pods != nil {
		for _, p := range pods.Items {
			name := p.Labels[LabelInstance]
			info := podInfo{Phase: p.Status.Phase}

			// Extract container-level status (the real source of truth).
			for _, cs := range p.Status.ContainerStatuses {
				info.Restarts += cs.RestartCount
				if cs.State.Waiting != nil {
					// Waiting reasons like CrashLoopBackOff, ImagePullBackOff, ErrImagePull
					info.ContainerState = cs.State.Waiting.Reason
				} else if cs.State.Terminated != nil {
					info.ContainerState = "Terminated:" + cs.State.Terminated.Reason
				} else if cs.State.Running != nil && info.ContainerState == "" {
					info.ContainerState = "Running"
				}
			}
			// Also check init containers.
			for _, cs := range p.Status.InitContainerStatuses {
				if cs.State.Waiting != nil {
					info.ContainerState = "Init:" + cs.State.Waiting.Reason
				}
			}

			// Prefer the most informative pod (Running > others, or newest).
			if existing, ok := podInfoByInstance[name]; !ok || p.Status.Phase == corev1.PodRunning {
				_ = existing
				podInfoByInstance[name] = info
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
		info := podInfoByInstance[instanceName]
		summaries = append(summaries, InstanceSummary{
			Name:            instanceName,
			DeploymentName:  d.Name,
			DesiredReplicas: desired,
			ReadyReplicas:   d.Status.ReadyReplicas,
			PodPhase:        info.Phase,
			ContainerState:  info.ContainerState,
			Restarts:        info.Restarts,
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

// PullConfig retrieves the raw config.json for an instance from its K8s secret.
func (c *Client) PullConfig(ctx context.Context, instanceName string) ([]byte, error) {
	secretName := resourceName(instanceName) + "-config"
	secret, err := c.cs.CoreV1().Secrets(c.namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get secret %s: %w", secretName, err)
	}
	if v, ok := secret.Data["config.json"]; ok {
		return v, nil
	}
	return nil, fmt.Errorf("secret %s has no config.json key", secretName)
}

// PushConfig writes raw config.json bytes to an instance's K8s secret and restarts the pod.
func (c *Client) PushConfig(ctx context.Context, instanceName string, configJSON []byte) error {
	// Validate it's valid JSON.
	var check map[string]interface{}
	if err := json.Unmarshal(configJSON, &check); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	secretName := resourceName(instanceName) + "-config"
	secret, err := c.cs.CoreV1().Secrets(c.namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get secret %s: %w", secretName, err)
	}
	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}
	secret.Data["config.json"] = configJSON

	if _, err := c.cs.CoreV1().Secrets(c.namespace).Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update secret %s: %w", secretName, err)
	}
	return c.RestartInstance(ctx, instanceName)
}

// SetTelegram configures Telegram channel in the instance's config.json.
// It reads the existing config, patches in the telegram channel + gateway settings, and pushes back.
func (c *Client) SetTelegram(ctx context.Context, instanceName, token string, allowFrom []string) error {
	raw, err := c.PullConfig(ctx, instanceName)
	if err != nil {
		return err
	}

	// Unmarshal into generic map to preserve all existing fields.
	var cfg map[string]interface{}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("parse existing config: %w", err)
	}

	// Patch channels.telegram
	channels, ok := cfg["channels"].(map[string]interface{})
	if !ok {
		channels = map[string]interface{}{}
	}
	tgConfig := map[string]interface{}{
		"enabled":    true,
		"token":      token,
		"allow_from": allowFrom,
	}
	channels["telegram"] = tgConfig
	cfg["channels"] = channels

	// Ensure gateway config exists with sane defaults for container.
	if _, ok := cfg["gateway"]; !ok {
		cfg["gateway"] = map[string]interface{}{
			"host": "0.0.0.0",
			"port": 8080,
		}
	}

	updated, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal updated config: %w", err)
	}
	return c.PushConfig(ctx, instanceName, updated)
}

// SetSecret adds or updates a key-value pair in the instance's Secret.
// The pod must be restarted to pick up changes (rollout restart on the Deployment).
func (c *Client) SetSecret(ctx context.Context, instanceName, key, value string) error {
	secretName := resourceName(instanceName) + "-config"

	secret, err := c.cs.CoreV1().Secrets(c.namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get secret %s: %w", secretName, err)
	}

	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}
	secret.Data[key] = []byte(value)

	if _, err := c.cs.CoreV1().Secrets(c.namespace).Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update secret %s: %w", secretName, err)
	}

	return c.RestartInstance(ctx, instanceName)
}

// RestartInstance triggers a restart of an instance's deployment.
// It first checks if the pod is in an error state (ImagePullBackOff, CrashLoopBackOff, etc.)
// and performs a scale-down/up cycle to force a fresh pod. Otherwise uses annotation-based rollout.
func (c *Client) RestartInstance(ctx context.Context, name string) error {
	baseName := resourceName(name)
	deploy, err := c.cs.AppsV1().Deployments(c.namespace).Get(ctx, baseName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get deployment: %w", err)
	}

	// Check if the pod is stuck in an error state that annotation restart won't fix.
	needsForceRestart := false
	pods, _ := c.cs.CoreV1().Pods(c.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: InstanceSelector(name),
	})
	if pods != nil {
		for _, p := range pods.Items {
			for _, cs := range p.Status.ContainerStatuses {
				if cs.State.Waiting != nil {
					reason := cs.State.Waiting.Reason
					if reason == "ImagePullBackOff" || reason == "ErrImagePull" ||
						reason == "CrashLoopBackOff" || reason == "CreateContainerError" {
						needsForceRestart = true
					}
				}
			}
		}
	}

	if needsForceRestart {
		// Delete all pods to force fresh creation.
		if pods != nil {
			for _, p := range pods.Items {
				_ = c.cs.CoreV1().Pods(c.namespace).Delete(ctx, p.Name, metav1.DeleteOptions{})
			}
		}
	}

	// Always bump the restart annotation so the deployment controller creates new pods.
	if deploy.Spec.Template.Annotations == nil {
		deploy.Spec.Template.Annotations = make(map[string]string)
	}
	deploy.Spec.Template.Annotations["eclaw/restartedAt"] = time.Now().Format(time.RFC3339)
	if _, err := c.cs.AppsV1().Deployments(c.namespace).Update(ctx, deploy, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("restart deployment: %w", err)
	}
	return nil
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

// SetRegistryCredentials creates or updates a docker-registry Secret in the namespace
// that can be used as imagePullSecrets for pulling container images from a private registry.
func (c *Client) SetRegistryCredentials(ctx context.Context, server, username, password string) error {
	if err := c.EnsureNamespace(ctx); err != nil {
		return fmt.Errorf("ensure namespace: %w", err)
	}

	// Build the dockerconfigjson payload.
	auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	dockerConfig := fmt.Sprintf(`{"auths":{%q:{"username":%q,"password":%q,"auth":%q}}}`,
		server, username, password, auth)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      RegistrySecretName,
			Namespace: c.namespace,
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: []byte(dockerConfig),
		},
	}

	if _, err := c.cs.CoreV1().Secrets(c.namespace).Create(ctx, secret, metav1.CreateOptions{}); k8serrors.IsAlreadyExists(err) {
		if _, err := c.cs.CoreV1().Secrets(c.namespace).Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update registry secret: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("create registry secret: %w", err)
	}

	return nil
}

// hasRegistrySecret checks if the eclaw-registry pull secret exists in the namespace.
func (c *Client) hasRegistrySecret(ctx context.Context) bool {
	_, err := c.cs.CoreV1().Secrets(c.namespace).Get(ctx, RegistrySecretName, metav1.GetOptions{})
	return err == nil
}

// ExposeOptions contains configuration for exposing an instance externally.
type ExposeOptions struct {
	Name     string // Instance name
	Type     string // "nodeport", "loadbalancer", or "ingress"
	NodePort int32  // Optional specific NodePort number (only for nodeport type)
	Host     string // Hostname for ingress (required for ingress type)
	TLS      bool   // Enable TLS via cert-manager (only for ingress)
	Issuer   string // cert-manager ClusterIssuer name (default: letsencrypt-prod)
	Class    string // Ingress class (default: nginx)
	Path     string // URL path prefix (default: /)
}

// ExposeResult holds the result of exposing an instance.
type ExposeResult struct {
	Type     string
	URL      string
	NodePort int32 // Populated for nodeport type
}

// ExposeInstance creates external access for an instance's HTTP port (8080).
// It creates a separate -ext Service (NodePort, LoadBalancer, or ClusterIP for ingress backing)
// and optionally an Ingress resource. Uses upsert semantics.
func (c *Client) ExposeInstance(ctx context.Context, opts ExposeOptions) (*ExposeResult, error) {
	baseName := resourceName(opts.Name)
	extSvcName := baseName + "-ext"
	instanceLabels := InstanceLabels(opts.Name)

	// Determine the Service type.
	var svcType corev1.ServiceType
	switch opts.Type {
	case "nodeport":
		svcType = corev1.ServiceTypeNodePort
	case "loadbalancer":
		svcType = corev1.ServiceTypeLoadBalancer
	case "ingress":
		svcType = corev1.ServiceTypeClusterIP
	default:
		return nil, fmt.Errorf("unsupported expose type %q: must be nodeport, loadbalancer, or ingress", opts.Type)
	}

	// Build the external Service.
	svcPort := corev1.ServicePort{
		Name:       "http",
		Port:       8080,
		TargetPort: intstr.FromInt32(8080),
		Protocol:   corev1.ProtocolTCP,
	}
	if opts.Type == "nodeport" && opts.NodePort > 0 {
		svcPort.NodePort = opts.NodePort
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      extSvcName,
			Namespace: c.namespace,
			Labels:    instanceLabels,
		},
		Spec: corev1.ServiceSpec{
			Type:     svcType,
			Selector: instanceLabels,
			Ports:    []corev1.ServicePort{svcPort},
		},
	}

	// Upsert the Service.
	createdSvc, err := c.cs.CoreV1().Services(c.namespace).Create(ctx, svc, metav1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		// Fetch existing to preserve ClusterIP (immutable field).
		existing, getErr := c.cs.CoreV1().Services(c.namespace).Get(ctx, extSvcName, metav1.GetOptions{})
		if getErr != nil {
			return nil, fmt.Errorf("get existing service: %w", getErr)
		}
		svc.Spec.ClusterIP = existing.Spec.ClusterIP
		svc.ResourceVersion = existing.ResourceVersion
		createdSvc, err = c.cs.CoreV1().Services(c.namespace).Update(ctx, svc, metav1.UpdateOptions{})
		if err != nil {
			return nil, fmt.Errorf("update external service: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("create external service: %w", err)
	}

	result := &ExposeResult{Type: opts.Type}

	// For ingress type, also create the Ingress resource.
	if opts.Type == "ingress" {
		if opts.Host == "" {
			return nil, fmt.Errorf("--host is required for ingress type")
		}

		pathType := networkingv1.PathTypePrefix
		ingress := &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:        baseName,
				Namespace:   c.namespace,
				Labels:      instanceLabels,
				Annotations: map[string]string{},
			},
			Spec: networkingv1.IngressSpec{
				IngressClassName: &opts.Class,
				Rules: []networkingv1.IngressRule{
					{
						Host: opts.Host,
						IngressRuleValue: networkingv1.IngressRuleValue{
							HTTP: &networkingv1.HTTPIngressRuleValue{
								Paths: []networkingv1.HTTPIngressPath{
									{
										Path:     opts.Path,
										PathType: &pathType,
										Backend: networkingv1.IngressBackend{
											Service: &networkingv1.IngressServiceBackend{
												Name: extSvcName,
												Port: networkingv1.ServiceBackendPort{
													Number: 8080,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		if opts.TLS {
			ingress.Annotations["cert-manager.io/cluster-issuer"] = opts.Issuer
			ingress.Spec.TLS = []networkingv1.IngressTLS{
				{
					Hosts:      []string{opts.Host},
					SecretName: baseName + "-tls",
				},
			}
		}

		// Upsert the Ingress.
		if _, err := c.cs.NetworkingV1().Ingresses(c.namespace).Create(ctx, ingress, metav1.CreateOptions{}); k8serrors.IsAlreadyExists(err) {
			existing, getErr := c.cs.NetworkingV1().Ingresses(c.namespace).Get(ctx, baseName, metav1.GetOptions{})
			if getErr != nil {
				return nil, fmt.Errorf("get existing ingress: %w", getErr)
			}
			ingress.ResourceVersion = existing.ResourceVersion
			if _, err := c.cs.NetworkingV1().Ingresses(c.namespace).Update(ctx, ingress, metav1.UpdateOptions{}); err != nil {
				return nil, fmt.Errorf("update ingress: %w", err)
			}
		} else if err != nil {
			return nil, fmt.Errorf("create ingress: %w", err)
		}

		scheme := "http"
		if opts.TLS {
			scheme = "https"
		}
		result.URL = fmt.Sprintf("%s://%s%s", scheme, opts.Host, opts.Path)
	}

	// Build result URL for non-ingress types.
	switch opts.Type {
	case "nodeport":
		nodePort := int32(0)
		if createdSvc != nil && len(createdSvc.Spec.Ports) > 0 {
			nodePort = createdSvc.Spec.Ports[0].NodePort
		}
		result.NodePort = nodePort
		result.URL = fmt.Sprintf("<node-ip>:%d", nodePort)
	case "loadbalancer":
		result.URL = fmt.Sprintf("%s (port 8080, awaiting external IP)", extSvcName)
	}

	return result, nil
}

// UnexposeInstance removes external access for an instance by deleting
// the -ext Service and any Ingress resource.
func (c *Client) UnexposeInstance(ctx context.Context, name string) error {
	baseName := resourceName(name)
	extSvcName := baseName + "-ext"

	// Delete the external Service if it exists.
	err := c.cs.CoreV1().Services(c.namespace).Delete(ctx, extSvcName, metav1.DeleteOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("delete external service %s: %w", extSvcName, err)
	}

	// Delete the Ingress if it exists.
	err = c.cs.NetworkingV1().Ingresses(c.namespace).Delete(ctx, baseName, metav1.DeleteOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("delete ingress %s: %w", baseName, err)
	}

	return nil
}

