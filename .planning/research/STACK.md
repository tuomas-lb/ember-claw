# Technology Stack

**Project:** Ember-Claw (Kubernetes deployment toolkit + gRPC sidecar + Go CLI)
**Researched:** 2026-03-13
**Overall confidence:** HIGH (versions verified against Go module proxy via `go list -m -versions`; patterns verified against existing codebase; architecture informed by PicoClaw source analysis)

## Recommended Stack

### Language & Runtime

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| Go | 1.25 | All components (sidecar, CLI, proto codegen) | Matches PicoClaw (go 1.25.7), conference-service (go 1.25). Single binary output, excellent gRPC support, native K8s client. System has go1.25.1 installed. | HIGH |

**Rationale:** Go is a hard constraint from PROJECT.md, but also the right choice regardless -- the entire ecosystem (PicoClaw, coralie-conference-service, sip-worker, livekit-agent) runs Go 1.25.x. Using the same version avoids any compatibility issues.

### gRPC & Protocol Buffers

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| google.golang.org/grpc | v1.79.2 | gRPC server (sidecar) and client (CLI) | Latest stable release (verified via module proxy). Coralie uses v1.69.2 -- use latest since this is greenfield. Bidirectional streaming is the core requirement for interactive chat. | HIGH |
| google.golang.org/protobuf | v1.36.11 | Protobuf runtime | Latest stable. PicoClaw already uses this exact version. | HIGH |
| protoc | 33.2 | Proto file compilation | Already installed on system. Works with protoc-gen-go v1.36.11. | HIGH |
| protoc-gen-go | v1.36.11 | Go code generation from .proto | Already installed on system. Matches protobuf runtime version. | HIGH |
| protoc-gen-go-grpc | v1.6.0 | gRPC service code generation | Already installed on system. Generates server/client interfaces from proto. | HIGH |

**Why gRPC over alternatives:**
- **Not Connect-RPC:** Connect-RPC (connectrpc.com/connect v1.19.1 is latest) would add HTTP/2+JSON flexibility we do not need. Internal cluster traffic only, no browser clients, no REST requirements. Standard gRPC is simpler and the Coralie ecosystem already uses it.
- **Not WebSocket:** gRPC bidirectional streaming is purpose-built for this. WebSocket would require manual framing, no codegen, no type safety.

### PicoClaw (Library Dependency -- Critical Architecture Decision)

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| github.com/sipeed/picoclaw | pin to commit | AI agent loop, config, session management | Imported as a Go library. The sidecar calls `AgentLoop.ProcessDirect(ctx, message, sessionKey)` directly -- no subprocess wrapping needed. | HIGH |

**Critical architectural decision:** PicoClaw is imported as a Go library, not spawned as a subprocess. The `pkg/agent.AgentLoop` exposes `ProcessDirect(ctx, message, sessionKey) (string, error)` which is a clean programmatic API. The sidecar binary embeds PicoClaw directly. See ARCHITECTURE.md for full rationale and PITFALLS.md Pitfall 1 for why subprocess wrapping is wrong.

**Dependency management for PicoClaw:**
- If public module: `go get github.com/sipeed/picoclaw@<commit>` in go.mod
- For local development: use `go.work` workspace file (not `go mod replace`) so local changes are picked up without modifying go.mod
- Add `go.work` to `.dockerignore` so workspace config does not leak into Docker builds

### Kubernetes Client

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| k8s.io/client-go | v0.35.2 | Programmatic K8s API access from CLI | Latest stable release (verified via module proxy). Provides typed clients for Deployments, Services, PVCs, Pods, and port-forwarding. | HIGH |
| k8s.io/apimachinery | v0.35.2 | K8s API types and utilities | Always matches client-go version. Provides metav1, labels, runtime types. | HIGH |
| k8s.io/api | v0.35.2 | K8s API object definitions | Always matches client-go version. Provides appsv1, corev1, etc. | HIGH |

**Why client-go, not kubectl exec/shell:**
The CLI needs to programmatically create deployments, PVCs, services, and set up port-forwarding for chat. Shelling out to kubectl is brittle (version mismatches, parsing text output, error handling). client-go gives typed, tested, version-matched access to the K8s API. It also enables port-forwarding in-process, which is critical for the `eclaw chat` command to connect to the gRPC sidecar.

**Kubeconfig handling:** client-go's `tools/clientcmd` package handles kubeconfig loading. Point it at `/Users/tuomas/Projects/ember.kubeconfig.yaml` (or configurable via `--kubeconfig` flag / `KUBECONFIG` env var).

### CLI Framework

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| github.com/spf13/cobra | v1.10.2 | CLI command structure | The de facto Go CLI framework. PicoClaw uses this exact version. kubectl, docker, gh, hugo all use Cobra. Provides subcommands, flags, help generation, shell completion. | HIGH |
| github.com/spf13/pflag | v1.0.10 | POSIX-compatible flags | Cobra's flag library. Already a transitive dependency. | HIGH |

**Why Cobra over alternatives:**
- **Not urfave/cli:** Cobra has won the Go CLI ecosystem. kubectl uses it. PicoClaw uses it. Zero learning curve for this team.
- **Not kong:** Smaller community, fewer examples, no ecosystem alignment benefit.
- **Not bare flag package:** No subcommand support. `eclaw deploy`, `eclaw chat`, `eclaw list` etc. need a proper subcommand tree.

### Terminal Output & Interaction

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| github.com/fatih/color | v1.18.0 | Colored terminal output | Latest stable (verified). Lightweight, zero-dependency colorized output. For status messages, errors, success indicators. | HIGH |
| github.com/olekukonko/tablewriter | v0.0.6 | Table formatting for list/status | Formats `eclaw list` output as ASCII tables. The v0 API is widely used and battle-tested. v1.1.4 exists but v0 has broader community examples. | MEDIUM |
| github.com/chzyer/readline | v1.5.1 | Interactive chat input | PicoClaw uses this exact version. Provides line editing, history for interactive chat mode. | HIGH |

**Why NOT bubbletea/lipgloss/glamour for the CLI:**
The CLI is a management tool, not a TUI application. `eclaw list` prints a table. `eclaw chat` is a readline-based REPL. Full TUI frameworks (bubbletea v1.3.10, lipgloss v1.1.0, glamour v1.0.0) add substantial dependency weight for features we will not use. PicoClaw already has a TUI launcher (`picoclaw-launcher-tui`) -- ember-claw should be a lean CLI tool.

### Logging

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| github.com/rs/zerolog | v1.34.0 | Structured logging for the sidecar | PicoClaw uses this exact version. Zero-allocation JSON logging. The sidecar is a long-running process that needs structured logs for debugging. | HIGH |

**Why zerolog:**
- PicoClaw uses it, so patterns are consistent across the embedded library and the sidecar wrapper.
- Zero-allocation design is ideal for the sidecar.
- **Not log/slog:** slog (stdlib since Go 1.21) is fine, but zerolog is already in the ecosystem and more performant for structured output.
- **Not zap:** Conference-service uses zap indirectly via zapdriver, but zerolog is what PicoClaw uses and it is lighter.

### Configuration

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| github.com/caarlos0/env/v11 | v11.4.0 | Environment variable parsing | Latest stable (verified). PicoClaw uses v11.3.1. Clean struct-tag-based env parsing for sidecar configuration. | HIGH |
| gopkg.in/yaml.v3 | v3.0.1 | YAML config file parsing | For PicoClaw config.json/yaml mounting. Part of existing ecosystem. | MEDIUM |

### Testing

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| github.com/stretchr/testify | v1.11.1 | Test assertions and mocking | Latest stable (verified). PicoClaw uses this exact version. Standard Go testing companion. | HIGH |
| testing (stdlib) | - | Test runner | Built-in. Always use with testify assertions. | HIGH |

### Containerization

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| Docker (multi-stage) | - | Container image builds | Multi-stage: Go builder compiles sidecar (which embeds PicoClaw), minimal Alpine runtime. Platform: linux/amd64 via docker buildx. | HIGH |
| alpine:3.23 | - | Runtime base image | Matches PicoClaw's own Dockerfile. Contains ca-certificates for HTTPS (needed for LLM API calls). Provides shell for debugging during development. | HIGH |

**Note on distroless:** `gcr.io/distroless/static-debian12` is smaller (<2MB), but PicoClaw needs ca-certificates for HTTPS calls to AI provider APIs, and having a shell is valuable during development. Alpine with ca-certificates is the pragmatic choice. Harden to distroless later if desired.

### Build & Deployment Orchestration

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| GNU Make | - | Build orchestration, interactive deployment | Matches parent umbrella repo pattern exactly. Interactive targets with shell prompts for instance name, config. | HIGH |
| docker buildx | - | Cross-platform image building | Already used across all Coralie services. Target: linux/amd64. | HIGH |

### Identifiers & Health

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| github.com/google/uuid | v1.6.0 | Unique session IDs | Latest stable (verified). PicoClaw uses this exact version. | HIGH |
| google.golang.org/grpc/health | (bundled with grpc) | Standard gRPC health protocol | Kubernetes can probe gRPC health directly (since K8s 1.24). Delegates to PicoClaw's internal ready state. | HIGH |

## Alternatives Considered

| Category | Recommended | Alternative | Why Not |
|----------|-------------|-------------|---------|
| gRPC framework | google.golang.org/grpc | connectrpc.com/connect | No browser/REST clients needed. Standard gRPC matches ecosystem. |
| CLI framework | cobra | urfave/cli/v2 | Cobra dominates Go CLI. PicoClaw + kubectl use it. |
| K8s interaction | client-go | shelling to kubectl | Brittle, no in-process port-forwarding, poor error handling. |
| K8s interaction | client-go | controller-runtime | Over-engineered. Not building an operator. Need simple CRUD + port-forward. |
| K8s packaging | Make + manifests | Helm | PROJECT.md says "Make, not Helm." Matches umbrella repo pattern. |
| K8s packaging | Make + manifests | Kustomize | Adds tool dependency. Template rendering in Make/shell is sufficient for this scale. |
| Logging | zerolog | log/slog | zerolog already in ecosystem. Consistent with PicoClaw. |
| Logging | zerolog | zap | Heavier. zerolog is already the PicoClaw standard. |
| TUI | readline (minimal) | bubbletea | CLI, not TUI. Chat mode is a REPL. |
| Terminal tables | tablewriter | lipgloss/table | Lipgloss requires full charm stack. tablewriter is purpose-built and lightweight. |
| Config | env/v11 + flags | viper | Viper is heavy (many transitive deps). env/v11 + Cobra flags cover all needs. |
| PicoClaw integration | Library import (pkg/agent) | Subprocess wrapping (stdin/stdout) | ProcessDirect() API is clean. stdin/stdout parsing is fragile with readline escape codes. |
| PicoClaw integration | Library import (pkg/agent) | HTTP bridge to gateway mode | Adds unnecessary HTTP hop. Library import is direct function call with proper Go context propagation. |
| Base image | Alpine | distroless | PicoClaw needs ca-certificates. Alpine provides shell for debugging. |

## Project Structure

```
ember-claw/
  cmd/
    eclaw/                # CLI binary entry point
      main.go
    sidecar/              # gRPC sidecar binary entry point
      main.go
  internal/
    cli/                  # CLI command implementations (Cobra commands)
      root.go             # Root command, kubeconfig setup
      deploy.go
      list.go
      delete.go
      status.go
      logs.go
      chat.go
      query.go
    server/               # Sidecar gRPC server implementation
      server.go           # gRPC server wrapping AgentLoop
      health.go           # Health check integration
    k8s/                  # Kubernetes client wrapper
      client.go           # client-go setup, kubeconfig
      resources.go        # Create/delete Deployments, Services, PVCs, Secrets
      portforward.go      # In-process port forwarding for chat
      labels.go           # Label constants and selectors
    manifest/             # K8s manifest generation
      templates.go        # Go template rendering for YAML
    config/               # Configuration types
      instance.go         # Instance configuration struct
      defaults.go         # Default resource limits, ports, etc.
  proto/
    emberclaw/v1/         # Protobuf definitions
      service.proto
  gen/
    emberclaw/v1/         # Generated Go code (from protoc)
      service.pb.go
      service_grpc.pb.go
  deploy/
    templates/            # K8s manifest templates (for Make targets)
      deployment.yaml.tmpl
      service.yaml.tmpl
      pvc.yaml.tmpl
      secret.yaml.tmpl
  Makefile
  Dockerfile
  go.mod
  go.sum
```

## Version Alignment Summary

Key principle: **Align with PicoClaw where dependencies overlap.** Since the sidecar imports PicoClaw as a Go library, version alignment is required to avoid dependency conflicts in the shared module graph.

| Dependency | PicoClaw Version | Ember-Claw Version | Notes |
|------------|-----------------|--------------------:|-------|
| Go | 1.25.7 | 1.25 (go.mod) | go.mod says `go 1.25`, runtime is 1.25.x |
| cobra | v1.10.2 | v1.10.2 | Exact match (shared transitive dep) |
| protobuf | v1.36.11 | v1.36.11 | Exact match (shared transitive dep) |
| zerolog | v1.34.0 | v1.34.0 | Exact match |
| uuid | v1.6.0 | v1.6.0 | Exact match |
| readline | v1.5.1 | v1.5.1 | Exact match |
| testify | v1.11.1 | v1.11.1 | Exact match |
| env/v11 | v11.3.1 | v11.4.0 | Minor bump OK. Go modules resolve to highest compatible. |
| grpc | (not in PicoClaw) | v1.79.2 | New dependency, latest stable |
| client-go | (not in PicoClaw) | v0.35.2 | New dependency, latest stable |
| picoclaw | - | pinned commit | Import as library. Pin for reproducibility. |

## Installation

```bash
# Initialize module
go mod init github.com/LastBotInc/ember-claw

# PicoClaw as library dependency (pin to specific commit)
go get github.com/sipeed/picoclaw@<latest-commit>

# Core dependencies
go get google.golang.org/grpc@v1.79.2
go get google.golang.org/protobuf@v1.36.11
go get k8s.io/client-go@v0.35.2
go get k8s.io/apimachinery@v0.35.2
go get k8s.io/api@v0.35.2
go get github.com/spf13/cobra@v1.10.2

# Supporting libraries
go get github.com/rs/zerolog@v1.34.0
go get github.com/caarlos0/env/v11@v11.4.0
go get github.com/fatih/color@v1.18.0
go get github.com/olekukonko/tablewriter@v0.0.6
go get github.com/chzyer/readline@v1.5.1
go get github.com/google/uuid@v1.6.0

# Dev/test dependencies
go get github.com/stretchr/testify@v1.11.1

# Proto compilation tools (already installed on system)
# protoc 33.2 (libprotoc 33.2) -- installed at $(which protoc)
# protoc-gen-go v1.36.11 -- installed at /Users/tuomas/go/bin/protoc-gen-go
# protoc-gen-go-grpc v1.6.0 -- installed at /Users/tuomas/go/bin/protoc-gen-go-grpc
```

## Proto Compilation

```bash
# From project root
protoc --go_out=gen --go_opt=paths=source_relative \
       --go-grpc_out=gen --go-grpc_opt=paths=source_relative \
       proto/emberclaw/v1/service.proto
```

## Key Technical Decisions

### 1. PicoClaw as Go Library, Not Subprocess

The sidecar imports PicoClaw's `pkg/agent` package and calls `AgentLoop.ProcessDirect(ctx, message, sessionKey)` directly. This is a clean Go function call, not a process wrapper. Benefits: no stdin/stdout parsing, no TTY allocation, proper context cancellation, native error handling, session management via session keys. This makes the sidecar a thin gRPC-to-AgentLoop adapter.

### 2. Single Container Per Pod (Not True Sidecar Pattern)

Because the sidecar imports PicoClaw as a library, there is only one process and one container per pod. The term "sidecar" describes the conceptual role (gRPC adapter for PicoClaw), not the Kubernetes deployment topology. This eliminates: startup ordering, inter-container communication, shared volume coordination, and doubled resource requests.

### 3. Two Separate Binaries (sidecar + CLI)

The sidecar and CLI are separate `cmd/` entry points compiled into separate binaries. The sidecar runs in the K8s pod with PicoClaw embedded. The CLI runs on the developer's machine. They share the proto-generated types but have different concerns (server vs. client, container vs. local). Only the sidecar binary depends on PicoClaw's packages. The CLI depends on client-go and the gRPC client stubs.

### 4. client-go Port Forwarding for Chat

The `eclaw chat <instance>` command uses client-go's SPDY-based port-forward API to tunnel a local port to the sidecar's gRPC port inside the pod. This avoids exposing gRPC services via LoadBalancer or Ingress. Programmatic port-forwarding (not shelling to kubectl) enables proper lifecycle management and reconnection logic.

### 5. K8s Manifests as Go Structs (CLI) + YAML Templates (Make)

The CLI creates K8s resources programmatically using client-go typed APIs (Go structs from k8s.io/api). Make targets use YAML templates with Go text/template rendering for interactive deployment. Both paths produce the same resources.

### 6. Secrets for API Keys, ConfigMaps for Non-Sensitive Config

AI provider API keys go into Kubernetes Secrets (never in Deployment env vars directly). PicoClaw configuration (model name, provider, system prompt) goes into ConfigMaps. Both are mounted into the pod and read by PicoClaw's config loader.

## Sources

- Go module proxy (proxy.golang.org) -- all version numbers verified via `go list -m -versions` on 2026-03-13
- PicoClaw go.mod at `/Users/tuomas/Projects/picoclaw/go.mod` -- dependency alignment
- PicoClaw source code -- `pkg/agent/loop.go` (ProcessDirect API), `cmd/picoclaw/internal/gateway/` (gateway mode)
- PicoClaw Dockerfile at `/Users/tuomas/Projects/picoclaw/docker/Dockerfile` -- Alpine 3.23 base image
- coralie-conference-service go.mod and proto files at `/Users/tuomas/Projects/coralie-conference-service/` -- ecosystem patterns
- coralie-sip-worker go.mod at `/Users/tuomas/Projects/coralie-sip-worker/` -- gRPC version patterns
- livekit-agent-go go.mod at `/Users/tuomas/Projects/livekit-agent-go/` -- Go workspace patterns (go.work, replace directives)
- Parent Makefile at `/Users/tuomas/Projects/Makefile` -- deployment conventions
- protoc, protoc-gen-go, protoc-gen-go-grpc -- versions verified from installed binaries on system
- System Go version verified: go1.25.1 darwin/arm64
