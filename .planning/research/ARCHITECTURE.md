# Architecture Patterns

**Domain:** Kubernetes deployment toolkit with gRPC sidecar for AI assistant instances
**Researched:** 2026-03-13

## Recommended Architecture

```
                    Developer Workstation
                    +-------------------+
                    |   ember-claw CLI   |  (Go binary)
                    |   (eclaw)          |
                    +--------+----------+
                             |
                     kubectl port-forward
                     or ClusterIP Service
                             |
          Emberchat Kubernetes Cluster (namespace: picoclaw)
          +--------------------------------------------------+
          |                                                  |
          |  +-- Pod: picoclaw-{name} -----------------+     |
          |  |                                         |     |
          |  |  +-- Container: sidecar --------------+ |     |
          |  |  | gRPC server (:50051)               | |     |
          |  |  | - Chat RPC (bidi stream)           | |     |
          |  |  | - Query RPC (unary)                | |     |
          |  |  | - Status RPC (unary)               | |     |
          |  |  | - Health RPC (standard grpc.health)| |     |
          |  |  |                                    | |     |
          |  |  | Bridges gRPC <-> PicoClaw library   | |     |
          |  |  | (imports pkg/agent directly)       | |     |
          |  |  +------------------------------------+ |     |
          |  |                                         |     |
          |  |  +-- Volume: picoclaw-{name}-data ----+ |     |
          |  |  | /data/.picoclaw/                   | |     |
          |  |  | - config.json                      | |     |
          |  |  | - workspace/                       | |     |
          |  |  | - sessions/                        | |     |
          |  |  +------------------------------------+ |     |
          |  +-----------------------------------------+     |
          |                                                  |
          |  +-- Pod: picoclaw-{name2} ----------------+     |
          |  |  (same structure, different config)     |     |
          |  +-----------------------------------------+     |
          |                                                  |
          +--------------------------------------------------+
```

### Key Architectural Decision: Library Import vs Process Wrapper

After analyzing PicoClaw's codebase, the sidecar should **import PicoClaw as a Go library** rather than wrapping its CLI process via stdin/stdout. Rationale:

1. **PicoClaw's `pkg/agent.AgentLoop`** exposes `ProcessDirect(ctx, message, sessionKey) (string, error)` -- a clean, context-aware Go API perfect for gRPC bridging.
2. **Process wrapping is fragile**: PicoClaw's interactive mode uses `readline` with terminal escape codes, history, and prompt rendering. Parsing that output reliably is error-prone.
3. **PicoClaw already runs headless**: The `gateway` command shows PicoClaw is designed for non-interactive, long-running operation. The agent loop processes messages programmatically.
4. **Single process is simpler**: No subprocess management, no pipe buffering issues, no signal forwarding complexity. One process, one binary.
5. **The project constraint "do not fork or modify" is respected**: Importing a library is using it, not modifying it. The `go.mod` dependency is standard Go practice.

The sidecar binary imports `github.com/sipeed/picoclaw/pkg/agent`, `github.com/sipeed/picoclaw/pkg/config`, etc., creates an `AgentLoop`, and exposes it via gRPC.

**Confidence: HIGH** -- this is based on direct source code analysis of PicoClaw's public API.

### Component Boundaries

| Component | Responsibility | Communicates With |
|-----------|---------------|-------------------|
| **CLI (`eclaw`)** | Instance lifecycle (deploy, delete, list, status, logs, chat) | K8s API (client-go), gRPC sidecar |
| **Sidecar binary** | Hosts PicoClaw agent, exposes gRPC API, serves health checks | PicoClaw library (in-process), K8s health probes |
| **Protobuf definitions** | Shared contract between CLI and sidecar | Compiled into both CLI and sidecar |
| **K8s manifest templates** | Define pod/pvc/service resources per instance | Applied by CLI or Makefile via kubectl |
| **Makefile** | Interactive deployment, build orchestration | Docker buildx, kubectl, container registry |
| **Dockerfile** | Multi-stage build of sidecar binary | PicoClaw source (Go module dependency) |

### Data Flow

#### Chat Flow (Interactive Streaming)

```
User terminal
    |
    v
eclaw chat {name}
    |
    | 1. kubectl port-forward picoclaw-{name} 50051:50051
    |    (or connect to ClusterIP if service exists)
    |
    v
gRPC BidiStream: ChatService.Chat
    |
    | 2. Client sends ChatMessage{text: "hello"}
    v
Sidecar gRPC server
    |
    | 3. agentLoop.ProcessDirect(ctx, "hello", "grpc:client")
    v
PicoClaw AgentLoop (in-process)
    |
    | 4. LLM API call (Anthropic/OpenAI/etc)
    v
Response string
    |
    | 5. Stream back ChatResponse{text: "Hi there!", done: true}
    v
eclaw renders response in terminal
```

#### Single-Shot Query Flow

```
eclaw query {name} "What is 2+2?"
    |
    | 1. Port-forward + gRPC Unary: ChatService.Query
    v
Sidecar -> agentLoop.ProcessDirect(ctx, "What is 2+2?", "grpc:query")
    |
    | 2. Returns response string
    v
eclaw prints response and exits
```

#### Deployment Flow

```
make deploy-picoclaw (interactive)
    |
    | 1. Prompts: name, AI provider, API key, model, resources
    v
Generates K8s manifests from templates
    |
    | 2. kubectl apply -f (generated manifests)
    v
K8s creates: Deployment + PVC + Service + Secret
    |
    | 3. Pod starts, sidecar initializes PicoClaw AgentLoop
    v
Instance ready (health check passes)
```

Alternatively via CLI:
```
eclaw deploy --name alice --provider anthropic --model claude-sonnet-4-20250514
    |
    | 1. Builds K8s objects via client-go
    | 2. Creates Secret (API key)
    | 3. Creates PVC
    | 4. Creates Deployment
    | 5. Optionally creates Service
    v
Instance ready
```

## Component Details

### 1. Protobuf Service Definition

```protobuf
syntax = "proto3";
package emberclaw.v1;

service PicoClawService {
  // Bidirectional streaming for interactive chat
  rpc Chat(stream ChatRequest) returns (stream ChatResponse);

  // Single-shot query (unary)
  rpc Query(QueryRequest) returns (QueryResponse);

  // Instance status
  rpc Status(StatusRequest) returns (StatusResponse);
}

message ChatRequest {
  string message = 1;
  string session_key = 2;  // optional, defaults to "grpc:default"
}

message ChatResponse {
  string text = 1;
  bool done = 2;           // true when response is complete
  string error = 3;        // non-empty on error
}

message QueryRequest {
  string message = 1;
  string session_key = 2;
}

message QueryResponse {
  string text = 1;
  string error = 2;
}

message StatusRequest {}

message StatusResponse {
  string instance_name = 1;
  string model = 2;
  string provider = 3;
  int64 uptime_seconds = 4;
  bool ready = 5;
}
```

Health checking uses the standard `grpc.health.v1.Health` service (no custom proto needed).

### 2. Sidecar Binary (`ember-claw-sidecar`)

**Responsibility:** Single Go binary that:
- Reads PicoClaw config from `/data/.picoclaw/config.json`
- Creates an `agent.AgentLoop` with the configured provider
- Starts a gRPC server on port 50051
- Implements `PicoClawService` by delegating to `AgentLoop.ProcessDirect()`
- Serves gRPC health checks for Kubernetes probes
- Runs HTTP health endpoint on port 8080 for simpler liveness probes

**Key design:**
- The sidecar IS the PicoClaw instance. There is no separate PicoClaw process.
- Config is mounted via ConfigMap + Secret (API key) into the PVC volume.
- Session data persists across pod restarts via PVC.

### 3. CLI Binary (`eclaw`)

**Responsibility:** Developer-facing tool for managing PicoClaw instances on K8s.

**Subcommands:**
```
eclaw deploy    --name NAME [--provider X] [--model Y] [--cpu Z] [--memory Z]
eclaw delete    NAME
eclaw list
eclaw status    NAME
eclaw logs      NAME [--follow]
eclaw chat      NAME [--session KEY]
eclaw query     NAME "message"
eclaw config    NAME [--set key=value]
```

**K8s interaction pattern:**
- Uses `client-go` for K8s API operations (not shelling out to kubectl)
- Uses kubeconfig from `~/.kube/config` or `KUBECONFIG` env var
- All resources labeled with `app.kubernetes.io/managed-by: eclaw` and `eclaw.dev/instance: {name}`
- Namespace is configurable, defaults to `picoclaw`

**gRPC interaction pattern:**
- For `chat` and `query`: establishes port-forward via client-go's portforward package, then connects gRPC
- No LoadBalancer or Ingress needed -- port-forward is sufficient for dev/test fleet

### 4. Kubernetes Resources Per Instance

Each `eclaw deploy` creates:

| Resource | Name Pattern | Purpose |
|----------|-------------|---------|
| Deployment | `picoclaw-{name}` | Runs the sidecar container (replicas: 1) |
| PVC | `picoclaw-{name}-data` | Persistent storage for config, workspace, sessions |
| Secret | `picoclaw-{name}-config` | API keys (provider key, etc.) |
| ConfigMap | `picoclaw-{name}-config` | PicoClaw config.json (non-sensitive settings) |
| Service (optional) | `picoclaw-{name}` | ClusterIP for intra-cluster access |

**Labels on all resources:**
```yaml
labels:
  app.kubernetes.io/name: picoclaw
  app.kubernetes.io/instance: {name}
  app.kubernetes.io/managed-by: eclaw
  app.kubernetes.io/component: ai-assistant
```

### 5. Docker Image

Single multi-stage Dockerfile:

```dockerfile
# Stage 1: Build sidecar binary
FROM golang:1.25-alpine AS builder
COPY . /src
WORKDIR /src
RUN go build -o /sidecar ./cmd/sidecar

# Stage 2: Minimal runtime
FROM alpine:3.23
COPY --from=builder /sidecar /usr/local/bin/sidecar
USER 1000:1000
ENTRYPOINT ["sidecar"]
```

The sidecar binary statically links PicoClaw's packages. The resulting image is small (the Go binary + Alpine base).

**Registry:** `reg.r.lastbot.com/ember-claw-sidecar:{version}`

### 6. Makefile Targets

Following the parent umbrella repo pattern:

```makefile
build-sidecar          # docker buildx build
push-sidecar           # docker push to registry
build-push-sidecar     # build + push
deploy-picoclaw        # interactive deployment (prompts for name, config)
delete-picoclaw        # interactive deletion
list-picoclaw          # list running instances
```

## Patterns to Follow

### Pattern 1: Labeling Convention for Multi-Instance Management
**What:** All K8s resources for a PicoClaw instance share consistent labels
**When:** Always -- this is how `eclaw list` and `eclaw delete` discover resources
**Why:** Without consistent labels, cleanup of all resources for an instance requires hardcoded name patterns

```yaml
metadata:
  labels:
    app.kubernetes.io/name: picoclaw
    app.kubernetes.io/instance: "alice"  # user-chosen name
    app.kubernetes.io/managed-by: eclaw
```

### Pattern 2: Port-Forward for gRPC Access
**What:** CLI uses `client-go` portforward to reach pod gRPC port, not Services/Ingress
**When:** For all CLI-to-pod communication (chat, query, status)
**Why:** No need for LoadBalancer, TLS termination, or ingress config for a dev fleet. Port-forward is zero-config, secure (uses kubeconfig auth), and works through NAT/firewalls.

```go
// Pseudocode
pf := portforward.New(restClient, pod, []string{"50051:50051"})
pf.Start()
defer pf.Stop()
conn := grpc.Dial("localhost:50051")
```

### Pattern 3: Config via ConfigMap + Secret
**What:** PicoClaw config.json is a ConfigMap, API keys are Secrets, both mounted into PVC init path
**When:** At deploy time
**Why:** Separates sensitive (API keys) from non-sensitive config. ConfigMap changes can trigger rolling updates.

### Pattern 4: Go Module Repository Structure
**What:** Monorepo with shared proto package, separate cmd entries for sidecar and CLI

```
ember-claw/
  cmd/
    sidecar/        # gRPC server wrapping PicoClaw
      main.go
    eclaw/          # CLI tool
      main.go
  internal/
    k8s/            # Kubernetes client operations
    grpc/           # gRPC client (CLI side)
    server/         # gRPC server (sidecar side)
    manifest/       # K8s manifest generation
    config/         # CLI configuration
  proto/
    emberclaw/v1/   # Protobuf definitions
      service.proto
  deploy/
    templates/      # K8s manifest templates
  Makefile
  Dockerfile        # Sidecar image
  go.mod
  go.sum
```

**Why this structure:**
- `cmd/` separates two binaries with distinct deployment targets
- `internal/` prevents external imports (implementation detail)
- `proto/` is the contract shared between both binaries
- `deploy/templates/` keeps K8s manifests version-controlled

## Anti-Patterns to Avoid

### Anti-Pattern 1: Subprocess Wrapping via stdin/stdout
**What:** Running `picoclaw agent` as a child process and piping messages through stdin/stdout
**Why bad:** PicoClaw's interactive mode uses `readline` with escape codes, prompt strings, and terminal manipulation. Parsing that output is brittle. The `ProcessDirect` Go API is clean and returns a plain string.
**Instead:** Import PicoClaw as a Go library and call `AgentLoop.ProcessDirect()` directly.

### Anti-Pattern 2: Helm Charts for Simple Dev Fleet
**What:** Creating a Helm chart with values.yaml for deployment
**Why bad:** Overengineered for a dev/test fleet of named instances. Helm's release model doesn't map well to "named instances managed by a CLI." The parent repo uses Make, not Helm.
**Instead:** Go templates or `kubectl apply` with generated manifests. The CLI generates YAML and applies it.

### Anti-Pattern 3: Shared PVC Across Instances
**What:** Using one PVC for all PicoClaw instances
**Why bad:** PicoClaw stores sessions, memory, and workspace files. Sharing causes data isolation issues and makes deletion messy.
**Instead:** One PVC per instance. `eclaw delete` cleans up the PVC.

### Anti-Pattern 4: Ingress/LoadBalancer for gRPC
**What:** Exposing gRPC via Ingress or LoadBalancer for CLI access
**Why bad:** Requires TLS setup, gRPC-aware ingress controller (nginx gRPC annotations or Envoy), DNS records. All unnecessary for a dev tool.
**Instead:** kubectl port-forward via client-go. Zero config, secure by default.

### Anti-Pattern 5: Using kubectl CLI Instead of client-go
**What:** Shelling out to `kubectl` from the Go CLI
**Why bad:** Requires kubectl installed, version compatibility concerns, output parsing is fragile, error handling is poor.
**Instead:** Use `client-go` directly. The Go CLI is a first-class K8s client.

## Suggested Build Order (Dependencies)

The components have clear dependency ordering:

```
Phase 1: Proto + Sidecar (foundation)
  proto/emberclaw/v1/service.proto
       |
       v
  cmd/sidecar/ (gRPC server + PicoClaw library integration)
       |
       v
  Dockerfile (sidecar image)

Phase 2: CLI + K8s Integration
  internal/k8s/ (client-go operations)
  internal/manifest/ (K8s YAML generation)
       |
       v
  cmd/eclaw/ (CLI binary, uses internal/k8s + gRPC client)

Phase 3: Deployment Infra
  Makefile (build/push/deploy targets)
  deploy/templates/ (K8s manifest templates)
```

**Dependency rationale:**
1. Proto must come first -- both sidecar and CLI depend on generated code
2. Sidecar is the core value -- without it running, CLI has nothing to talk to
3. CLI depends on sidecar (for gRPC client) and K8s API (for instance management)
4. Makefile and deploy templates are the final layer, wiring everything together

## Scalability Considerations

| Concern | At 5 instances | At 50 instances | At 500 instances |
|---------|----------------|-----------------|------------------|
| Resource usage | ~50MB RAM each, negligible | ~2.5GB RAM total, watch CPU | Need resource quotas, node affinity |
| CLI list/status | Instant via label selector | Still fast, paginate | Use server-side filtering |
| PVC storage | 1Gi each, fine | 50Gi total, watch storage class | Need storage class with provisioning limits |
| Port-forward | One at a time, fine | Manageable | Consider a gateway/proxy service |
| Secret management | Manual in CLI | Cumbersome | Consider external secrets operator |

For the stated use case (dev/test fleet), 5-20 instances is the realistic range. The architecture is appropriate for that scale. If approaching 100+, the port-forward pattern should be replaced with a central gateway service.

## Sources

- PicoClaw source code at `/Users/tuomas/Projects/picoclaw/` (direct analysis)
  - `pkg/agent/loop.go`: AgentLoop API (ProcessDirect, ProcessDirectWithChannel)
  - `pkg/agent/instance.go`: AgentInstance structure
  - `pkg/config/config.go`: Config structure
  - `pkg/health/server.go`: Health check patterns
  - `cmd/picoclaw/internal/agent/helpers.go`: CLI agent mode (stdin/stdout interaction)
  - `cmd/picoclaw/internal/gateway/helpers.go`: Gateway mode (long-running service)
  - `docker/Dockerfile`: Container image pattern
  - `docker/docker-compose.yml`: Container runtime configuration
- Coralie umbrella repo patterns at `/Users/tuomas/Projects/` (direct analysis)
  - `Makefile`: Build/push/deploy target conventions
  - `coralie-gemini-worker/deploy/deployment.yaml`: K8s manifest patterns, PVC, health checks
- Kubernetes well-known labels: `app.kubernetes.io/*` standard
- gRPC health checking protocol: standard `grpc.health.v1.Health` service
- client-go portforward package: standard K8s API for pod port forwarding

**Confidence:** HIGH for all architecture decisions. Based entirely on direct source code analysis and established Kubernetes/gRPC patterns. No web search was needed -- the codebase provides all necessary context.
