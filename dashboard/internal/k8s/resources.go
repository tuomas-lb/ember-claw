package k8s

import (
	"os"
	"encoding/json"
	"fmt"

	"github.com/tuomas-lb/ember-claw/dashboard/internal/config"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	fallbackImage      = "ember-claw-sidecar:latest"
	defaultCPULimit    = "1000m"
	defaultMemoryLimit = "1Gi"
	defaultStorageSize = "5Gi"
	FleetConfigMap     = "picoclaw-fleet"
	FleetMDKey         = "Fleet.md"
	RoutingConfigMap   = "picoclaw-call-routing"
)

// sidecarImage returns the PicoClaw sidecar image the dashboard deploys for new
// instances. It is configurable via the SIDECAR_IMAGE env var (set on the
// dashboard Deployment) so the dashboard is not tied to any one registry.
func sidecarImage() string {
	if v := os.Getenv("SIDECAR_IMAGE"); v != "" {
		return v
	}
	return fallbackImage
}

// picoclawConfig mirrors the config.json format expected by ember-claw-sidecar.
type picoclawConfig struct {
	Agents    agentsConfig    `json:"agents"`
	Tools     toolsConfig     `json:"tools"`
	ModelList []modelEntry    `json:"model_list"`
	Gateway   gatewayConfig   `json:"gateway"`
}

type agentsConfig struct {
	Defaults agentDefaults `json:"defaults"`
}

type agentDefaults struct {
	ModelName               string `json:"model_name"`
	Workspace               string `json:"workspace"`
	RestrictToWorkspace     bool   `json:"restrict_to_workspace"`
	AllowReadOutsideWorkspace bool  `json:"allow_read_outside_workspace"`
	MaxToolIterations       int    `json:"max_tool_iterations"`
}

type toolsConfig struct {
	Exec execConfig `json:"exec"`
	MCP  mcpConfig  `json:"mcp"`
}

type execConfig struct {
	EnableDenyPatterns bool `json:"enable_deny_patterns"`
	AllowRemote        bool `json:"allow_remote"`
}

type mcpConfig struct {
	Servers   map[string]mcpServer `json:"servers"`
	Enabled   bool                 `json:"enabled"`
	Discovery mcpDiscovery         `json:"discovery"`
}

type mcpServer struct {
	Type    string            `json:"type"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	Enabled bool              `json:"enabled"`
}

type mcpDiscovery struct {
	Enabled bool `json:"enabled"`
}

type modelEntry struct {
	ModelName string `json:"model_name"`
	Model     string `json:"model"`
	APIKey    string `json:"api_key"`
	APIBase   string `json:"api_base"`
}

type gatewayConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

func buildConfigJSON(req config.DeployRequest) ([]byte, error) {
	modelFull := config.ProviderModelPrefix(req.Provider) + req.Model
	apiBase := config.ProviderAPIBase(req.Provider)

	cfg := picoclawConfig{
		Agents: agentsConfig{
			Defaults: agentDefaults{
				ModelName:               req.Model,
				Workspace:               "/home/picoclaw/.picoclaw/workspace",
				RestrictToWorkspace:     false,
				AllowReadOutsideWorkspace: true,
				MaxToolIterations:       200,
			},
		},
		Tools: toolsConfig{
			Exec: execConfig{
				EnableDenyPatterns: false,
				AllowRemote:        true,
			},
			MCP: mcpConfig{
				Servers: map[string]mcpServer{
					"backlog": {
						Type:    "stdio",
						Command: "backlog",
						Args:    []string{"mcp", "start"},
						Env:     map[string]string{"BACKLOG_CWD": "/workspace"},
						Enabled: true,
					},
				},
				Enabled: true,
				Discovery: mcpDiscovery{
					Enabled: false,
				},
			},
		},
		ModelList: []modelEntry{
			{
				ModelName: req.Model,
				Model:     modelFull,
				APIKey:    req.APIKey,
				APIBase:   apiBase,
			},
		},
		Gateway: gatewayConfig{
			Host: "0.0.0.0",
			Port: 8080,
		},
	}

	return json.MarshalIndent(cfg, "", "  ")
}

func commonLabels(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/managed-by": "eclaw",
		"app.kubernetes.io/name":       "picoclaw",
		"app.kubernetes.io/instance":   name,
	}
}

// resourceName returns the standard resource name for the given component.
func resourceName(instanceName, component string) string {
	if component == "" {
		return fmt.Sprintf("picoclaw-%s", instanceName)
	}
	return fmt.Sprintf("picoclaw-%s-%s", instanceName, component)
}

// BuildSecret creates the Secret holding config.json and optional integration keys.
func BuildSecret(namespace string, req config.DeployRequest) (*corev1.Secret, error) {
	configJSON, err := buildConfigJSON(req)
	if err != nil {
		return nil, fmt.Errorf("build config.json: %w", err)
	}

	data := map[string][]byte{
		"config.json": configJSON,
	}

	if req.LinearAPIKey != "" {
		data["LINEAR_API_KEY"] = []byte(req.LinearAPIKey)
	}
	if req.SlackBotToken != "" {
		data["SLACK_BOT_TOKEN"] = []byte(req.SlackBotToken)
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName(req.Name, "config"),
			Namespace: namespace,
			Labels:    commonLabels(req.Name),
		},
		Data: data,
	}, nil
}

// BuildBootstrapConfigMap creates a ConfigMap with the default identity files
// (AGENTS.md, SOUL.md, IDENTITY.md, USER.md) that the init container copies
// into the instance's PVC on first boot.
func BuildBootstrapConfigMap(namespace string, req config.DeployRequest) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName(req.Name, "bootstrap"),
			Namespace: namespace,
			Labels:    commonLabels(req.Name),
		},
		Data: map[string]string{
			"IDENTITY.md": fmt.Sprintf(`# %s

Instance: picoclaw-%s
Model: %s
Provider: %s
`, req.Name, req.Name, req.Model, req.Provider),

			"AGENTS.md": `# Agent Operating Rules

## Boot Sequence (MANDATORY — every session start)
1. Read SOUL.md — who am I
2. Read AGENTS.md — operating rules (this file)
3. Read Fleet.md — fleet registry, agent roles, handoff routes
4. Read state.md — current phase, last action, blockers
5. Read decisions.md — prior decisions (append-only)
6. Read /workspace/Backlog.md — current task board
7. Begin work from state.md Next Action field

## Priorities (in order)
1. Safety — no irreversible action without APPROVED entry in state.md
2. Accuracy — mark [UNVERIFIED] rather than assert
3. Persistence — write findings to files before reasoning about them

## Always
- Read state.md and decisions.md at the start of every invocation
- Read Fleet.md to know your role, handoff routes, and peer agents
- Write to state.md after every completed step
- Append decisions to decisions.md before terminating
- Update task board on every status transition
- Write a memory entry to memory/ before session end

## Never
- Act on scope not defined in your role (see Fleet.md)
- Terminate with findings only in context — write to files first
- Proceed past a blocker without writing BLOCKED to state.md
- Make decisions without recording them in decisions.md

## Checkpoint Protocol
After every completed phase or step:
1. Write findings to output file
2. Append decisions to decisions.md
3. Overwrite state.md with phase snapshot
4. Update task board status

## On Scope Violation
Write BLOCKED to state.md with reason and halt immediately.

## Telephony (Voice Calls)
You can make and receive phone calls through the voice bridge.

### Incoming calls
When someone calls your extension, you'll receive a message like:
  [INCOMING CALL] Extension: incoming, CallerID: unknown.
Reply with: ANSWER: <greeting>, IGNORE, DECLINE, or TRANSFER: <number>

### Outbound calls
To place a call, use exec to run:
  curl -s -X POST http://voice-bridge.picoclaw.svc.cluster.local:8080/call \
    -H "Content-Type: application/json" \
    -d '{"destination":"<number or extension>","caller_id":"PicoClaw <1001>","instructions":"<what to discuss>","instance":"<your instance name>"}'

### Active calls
To see active calls:
  curl -s http://voice-bridge.picoclaw.svc.cluster.local:8080/calls

### During a call
A voice AI (Gemini) handles the actual speaking. It will relay what the caller says to you and speak your responses naturally. You give instructions, it does the talking.
`,

			"SOUL.md": fmt.Sprintf(`# %s — Soul

## Identity
Autonomous AI agent in the PicoClaw fleet. Persistent workspace, file-grounded operation.

## Voice
Concise, precise, action-oriented. Prefer structured output over prose.

## Values
- File persistence over context accumulation
- Phase checkpoints over continuous threads
- Observable behavior over desired qualities
- State written is state that survives
`, req.Name),

			"USER.md": `# User

## Preferences
- Timezone: Europe/Helsinki
- Communication: Direct, no filler
- Output: Structured markdown, code blocks for commands
`,
		},
	}
}

// BuildConfigMap creates the ConfigMap holding environment variables.
func BuildConfigMap(namespace string, req config.DeployRequest) *corev1.ConfigMap {
	data := map[string]string{
		"PICOCLAW_PROVIDER": req.Provider,
		"PICOCLAW_MODEL":    req.Model,
	}
	for k, v := range req.CustomEnv {
		data[k] = v
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName(req.Name, "env"),
			Namespace: namespace,
			Labels:    commonLabels(req.Name),
		},
		Data: data,
	}
}

// BuildPVC creates the PersistentVolumeClaim for the instance.
func BuildPVC(namespace string, req config.DeployRequest) *corev1.PersistentVolumeClaim {
	storageSize := req.StorageSize
	if storageSize == "" {
		storageSize = defaultStorageSize
	}

	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName(req.Name, "data"),
			Namespace: namespace,
			Labels:    commonLabels(req.Name),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(storageSize),
				},
			},
		},
	}
}

// BuildDeployment creates the Deployment for a PicoClaw instance.
// It includes an init container that bootstraps identity/personality files,
// and a main container that runs the gRPC gateway with health/readiness probes.
func BuildDeployment(namespace string, req config.DeployRequest) *appsv1.Deployment {
	cpuLimit := req.CPULimit
	if cpuLimit == "" {
		cpuLimit = defaultCPULimit
	}
	memLimit := req.MemoryLimit
	if memLimit == "" {
		memLimit = defaultMemoryLimit
	}

	labels := commonLabels(req.Name)
	replicas := int32(1)

	// Environment from the secret/configmap
	envFromSources := []corev1.EnvFromSource{
		{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: resourceName(req.Name, "config"),
				},
				Optional: boolPtr(true),
			},
		},
		{
			ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: resourceName(req.Name, "env"),
				},
				Optional: boolPtr(true),
			},
		},
	}

	// Volume mounts for main container
	mainVolumeMounts := []corev1.VolumeMount{
		{
			Name:      "data",
			MountPath: "/home/picoclaw/.picoclaw",
		},
		{
			Name:      "config",
			MountPath: "/config",
			ReadOnly:  true,
		},
		{
			Name:      "workspace",
			MountPath: "/workspace",
		},
		{
			Name:      "fleet",
			MountPath: "/workspace/Fleet.md",
			SubPath:   FleetMDKey,
			ReadOnly:  true,
		},
	}

	// Init container mounts — writes bootstrap identity files to data volume
	initVolumeMounts := []corev1.VolumeMount{
		{
			Name:      "data",
			MountPath: "/home/picoclaw/.picoclaw",
		},
		{
			Name:      "bootstrap-files",
			MountPath: "/bootstrap",
			ReadOnly:  true,
		},
	}

	// Init container copies bootstrap files into the data PVC on first run
	initContainer := corev1.Container{
		Name:  "bootstrap",
		Image: sidecarImage(),
		Command: []string{
			"/bin/sh", "-c",
			`set -e
DEST=/home/picoclaw/.picoclaw
WS="$DEST/workspace"

# Create workspace directory structure
mkdir -p "$DEST" "$WS/memory" "$WS/findings" "$WS/briefings" "$WS/skills" \
  "$WS/protocols" "$WS/reports" "$WS/changelogs" "$WS/qa" "$WS/diagnosis" \
  "$WS/state" "$WS/sessions" "$WS/backlog"

# Copy bootstrap identity files (skip if already exist)
for f in IDENTITY.md AGENTS.md SOUL.md USER.md; do
  if [ ! -f "$DEST/$f" ]; then
    cp "/bootstrap/$f" "$DEST/$f" 2>/dev/null || true
  fi
done

# Create MEMORY.md if missing
if [ ! -f "$WS/memory/MEMORY.md" ]; then
  echo "# Memory" > "$WS/memory/MEMORY.md"
fi

# Create state.md if missing
if [ ! -f "$WS/state.md" ]; then
  cat > "$WS/state.md" << 'STATE'
# State — Initial

## Current Phase
Boot — awaiting first task

## Last Completed Action
None — fresh instance

## In Progress
None

## Blocked
None

## Next Action
Awaiting instructions
STATE
fi

# Create decisions.md if missing
if [ ! -f "$WS/decisions.md" ]; then
  echo "# Decisions" > "$WS/decisions.md"
fi

echo "Bootstrap complete"`,
		},
		VolumeMounts: initVolumeMounts,
	}

	// Main container
	mainContainer := corev1.Container{
		Name:  "picoclaw",
		Image: sidecarImage(),
		Ports: []corev1.ContainerPort{
			{Name: "grpc", ContainerPort: 50051, Protocol: corev1.ProtocolTCP},
			{Name: "health", ContainerPort: 8080, Protocol: corev1.ProtocolTCP},
		},
		Env: []corev1.EnvVar{
			{Name: "PICOCLAW_CONFIG", Value: "/config/config.json"},
			{Name: "PICOCLAW_HOME", Value: "/home/picoclaw/.picoclaw"},
		},
		EnvFrom: envFromSources,
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(cpuLimit),
				corev1.ResourceMemory: resource.MustParse(memLimit),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
		},
		VolumeMounts: mainVolumeMounts,
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/health",
					Port: intstr.FromInt(8080),
				},
			},
			InitialDelaySeconds: 15,
			PeriodSeconds:       20,
			FailureThreshold:    3,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/ready",
					Port: intstr.FromInt(8080),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       10,
			FailureThreshold:    3,
		},
	}

	// Volumes
	volumes := []corev1.Volume{
		{
			Name: "data",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: resourceName(req.Name, "data"),
				},
			},
		},
		{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: resourceName(req.Name, "config"),
					Items: []corev1.KeyToPath{
						{Key: "config.json", Path: "config.json"},
					},
				},
			},
		},
		{
			Name: "workspace",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/data/workspace",
				},
			},
		},
		{
			// Shared Fleet.md mounted into all instances
			Name: "fleet",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: FleetConfigMap,
					},
					Items: []corev1.KeyToPath{
						{Key: FleetMDKey, Path: FleetMDKey},
					},
					Optional: boolPtr(true),
				},
			},
		},
		{
			// Bootstrap files from a ConfigMap — contains AGENTS.md, SOUL.md, IDENTITY.md, USER.md
			Name: "bootstrap-files",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: resourceName(req.Name, "bootstrap"),
					},
					Optional: boolPtr(true),
				},
			},
		},
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName(req.Name, ""),
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/instance": req.Name,
					"app.kubernetes.io/name":     "picoclaw",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					InitContainers:               []corev1.Container{initContainer},
					Containers:                    []corev1.Container{mainContainer},
					Volumes:                       volumes,
					ImagePullSecrets:              []corev1.LocalObjectReference{{Name: "eclaw-registry"}},
					RestartPolicy:                 corev1.RestartPolicyAlways,
					TerminationGracePeriodSeconds: int64Ptr(30),
				},
			},
		},
	}
}

// BuildService creates the ClusterIP Service for gRPC and health endpoints.
func BuildService(namespace string, req config.DeployRequest) *corev1.Service {
	labels := commonLabels(req.Name)

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName(req.Name, ""),
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app.kubernetes.io/instance": req.Name,
				"app.kubernetes.io/name":     "picoclaw",
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "grpc",
					Port:       50051,
					TargetPort: intstr.FromString("grpc"),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "health",
					Port:       8080,
					TargetPort: intstr.FromString("health"),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}
}

func boolPtr(b bool) *bool { return &b }

func int64Ptr(i int64) *int64 { return &i }
