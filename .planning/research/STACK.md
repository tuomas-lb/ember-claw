# Technology Stack

**Project:** Ember-Claw (Kubernetes deployment toolkit + gRPC sidecar + Go CLI)
**Researched:** 2026-03-13
**Overall confidence:** HIGH (versions verified against Go module proxy; patterns verified against existing codebase)

## Recommended Stack

### Language & Runtime

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| Go | 1.25 | All components (sidecar, CLI, proto codegen) | Matches PicoClaw (go 1.25.7), conference-service (go 1.25). Single binary output, excellent gRPC support, native K8s client. System has go1.25.1 installed. | HIGH |

**Rationale:** Go is a hard constraint from PROJECT.md, but also the right choice regardless -- the entire ecosystem (PicoClaw, coralie-conference-service, sip-worker, livekit-agent) runs Go 1.25.x. Using the same version avoids any compatibility issues.

### gRPC & Protocol Buffers

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| google.golang.org/grpc | v1.79.2 | gRPC server (sidecar) and client (CLI) | Latest stable release. Coralie uses v1.69.2 -- we should use latest since this is greenfield. Bidirectional streaming is the core requirement for interactive chat. | HIGH |
| google.golang.org/protobuf | v1.36.11 | Protobuf runtime | Latest stable. PicoClaw already uses this exact version. | HIGH |
| protoc | 33.2 | Proto file compilation | Already installed on system. Works with protoc-gen-go v1.36.11. | HIGH |
| protoc-gen-go | v1.36.11 | Go code generation from .proto | Already installed. Matches protobuf runtime version. | HIGH |
| protoc-gen-go-grpc | v1.6.0 | gRPC service code generation | Already installed. Latest stable. Generates server/client interfaces. | HIGH |

**Why gRPC over alternatives:**
- **Not Connect-RPC:** Connect-RPC (connectrpc.com/connect v1.19.1 is latest) would add HTTP/2+JSON flexibility we do not need. This is internal cluster traffic only, no browser clients, no REST requirements. Standard gRPC is simpler and the Coralie ecosystem already uses it.
- **Not WebSocket:** gRPC bidirectional streaming is purpose-built for this. WebSocket would require manual framing, no codegen, no type safety.
- **Not stdin/stdout pipe directly:** The sidecar wraps PicoClaw's stdin/stdout specifically to expose it over the network via gRPC. That is the entire point of this project.

### Kubernetes Client

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| k8s.io/client-go | v0.35.2 | Programmatic K8s API access from CLI | Latest stable release (v0.35.2). Provides typed clients for Deployments, Services, PVCs, Pods, and port-forwarding. The CLI needs to create/delete/list/describe resources. | HIGH |
| k8s.io/apimachinery | v0.35.2 | K8s API types and utilities | Always matches client-go version. Provides metav1, labels, runtime types. | HIGH |
| k8s.io/api | v0.35.2 | K8s API object definitions | Always matches client-go version. Provides appsv1, corev1, etc. | HIGH |

**Why client-go, not kubectl exec/shell:**
The CLI needs to programmatically create deployments, PVCs, services, and set up port-forwarding for chat. Shelling out to kubectl is brittle (version mismatches, parsing text output, error handling). client-go gives typed, tested, version-matched access to the K8s API. It also enables port-forwarding in-process, which is critical for the `ember-claw chat` command to connect to the gRPC sidecar.

**Kubeconfig handling:** client-go's `tools/clientcmd` package handles kubeconfig loading. Point it at `/Users/tuomas/Projects/ember.kubeconfig.yaml` (or configurable via `--kubeconfig` flag / `KUBECONFIG` env var).

### CLI Framework

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| github.com/spf13/cobra | v1.10.2 | CLI command structure | The de facto Go CLI framework. PicoClaw uses this exact version. Every major Go CLI tool uses Cobra (kubectl, docker, gh, hugo). Provides subcommands, flags, help generation, shell completion. | HIGH |
| github.com/spf13/pflag | v1.0.10 | POSIX-compatible flags | Cobra's flag library. Already a transitive dependency. | HIGH |

**Why Cobra over alternatives:**
- **Not urfave/cli:** Cobra has won the Go CLI ecosystem. kubectl uses it. PicoClaw uses it. The learning curve is zero for this team.
- **Not kong:** Smaller community, fewer examples, no ecosystem alignment benefit.
- **Not bare flag package:** No subcommand support. `ember-claw deploy`, `ember-claw chat`, `ember-claw list` etc. need a proper subcommand tree.

### Terminal Output & Interaction

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| github.com/fatih/color | v1.18.0 | Colored terminal output | Latest stable. Lightweight, zero-dependency colorized output. For status messages, errors, success indicators. Simpler than lipgloss for non-TUI output. | HIGH |
| github.com/olekukonko/tablewriter | v0.0.6 or v1.1.4 | Table formatting for list/status | Formats `ember-claw list` output as ASCII tables. Use v0.0.6 (the widely-used v0 API) unless v1.1.4's API is cleaner -- verify at implementation time. | MEDIUM |
| github.com/chzyer/readline | v1.5.1 | Interactive chat input | PicoClaw uses this exact version. Provides line editing, history for interactive chat mode. | HIGH |

**Why NOT bubbletea/lipgloss/glamour for the CLI:**
The CLI is a management tool, not a TUI application. `ember-claw list` prints a table. `ember-claw chat` is a readline-based REPL. Full TUI frameworks (bubbletea v1.3.10, lipgloss v1.1.0, glamour v1.0.0) add substantial dependency weight for features we will not use. PicoClaw already has a TUI launcher (`picoclaw-launcher-tui`) -- ember-claw should be a lean CLI tool. If interactive chat wants markdown rendering later, glamour can be added then.

### Logging

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| github.com/rs/zerolog | v1.34.0 | Structured logging for the sidecar | PicoClaw uses this exact version. Zero-allocation JSON logging. The sidecar is a long-running process that needs structured logs for debugging. | HIGH |

**Why zerolog:**
- PicoClaw uses it, so patterns are consistent.
- Zero-allocation design is ideal for the sidecar which processes streaming data.
- **Not log/slog:** slog (stdlib since Go 1.21) is fine, but zerolog is already in the ecosystem and more performant for structured output.
- **Not zap:** Conference-service uses zap indirectly via zapdriver, but zerolog is what PicoClaw uses and it is lighter.

### Configuration

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| github.com/caarlos0/env/v11 | v11.4.0 | Environment variable parsing | Latest stable. PicoClaw uses v11.3.1. Clean struct-tag-based env parsing for sidecar configuration (port, PicoClaw binary path, etc.). | HIGH |
| gopkg.in/yaml.v3 | v3.0.1 | YAML config file parsing (optional) | For instance configuration files if needed. Part of existing ecosystem. | MEDIUM |

### Testing

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| github.com/stretchr/testify | v1.11.1 | Test assertions and mocking | Latest stable. PicoClaw uses v1.11.1. Standard Go testing companion. | HIGH |
| testing (stdlib) | - | Test runner | Built-in. Always use with testify assertions. | HIGH |

### Containerization

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| Docker (multi-stage) | - | Container image builds | Multi-stage build: Go builder stage compiles sidecar + PicoClaw, final stage is minimal (distroless or alpine). Follow existing Coralie pattern of `docker buildx --platform linux/amd64`. | HIGH |
| gcr.io/distroless/static-debian12 | latest | Runtime base image | Minimal attack surface, no shell, <2MB. Suitable because both the sidecar and PicoClaw are static Go binaries. | MEDIUM |
| alpine:3.21 | - | Alternative runtime base | If PicoClaw needs libc or shell access for any reason, use alpine instead of distroless. Verify at implementation time. | MEDIUM |

### Build & Deployment Orchestration

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| GNU Make | - | Build orchestration, interactive deployment | Matches parent umbrella repo pattern exactly. Interactive targets with shell prompts for instance name, config. | HIGH |
| docker buildx | - | Cross-platform image building | Already used across all Coralie services. Target: `linux/amd64`. | HIGH |
| kubectl (via client-go) | - | K8s resource management | The CLI tool embeds client-go; Make targets use kubectl directly for simple operations. | HIGH |

### Identifiers

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| github.com/google/uuid | v1.6.0 | Unique instance/session IDs | Latest stable. PicoClaw uses this exact version. For generating unique participant/session IDs in gRPC. | HIGH |

## Alternatives Considered

| Category | Recommended | Alternative | Why Not |
|----------|-------------|-------------|---------|
| gRPC framework | google.golang.org/grpc | connectrpc.com/connect | No browser/REST clients needed. Standard gRPC matches ecosystem. Connect adds unnecessary abstraction. |
| CLI framework | cobra | urfave/cli/v2 | Cobra dominates Go CLI. PicoClaw + kubectl use it. Zero learning curve. |
| CLI framework | cobra | kong | Smaller community, no alignment with existing codebase. |
| K8s interaction | client-go | shelling to kubectl | Brittle, text parsing, no in-process port-forwarding, poor error handling. |
| K8s interaction | client-go | controller-runtime | Over-engineered. We are not building an operator. We need simple CRUD + port-forward. |
| K8s packaging | Make + manifests | Helm | PROJECT.md explicitly says "Make for deployment, not Helm." Matches umbrella pattern. |
| K8s packaging | Make + manifests | Kustomize | Adds tool dependency. Template rendering in Make/shell is sufficient for this scale. |
| Logging | zerolog | log/slog | zerolog already in ecosystem. More performant. Consistent with PicoClaw. |
| Logging | zerolog | zap | Heavier. zerolog is already the PicoClaw standard. |
| TUI | readline (minimal) | bubbletea | ember-claw is a CLI, not a TUI. Chat mode is a REPL, not a full-screen app. |
| Terminal tables | tablewriter | lipgloss/table | Lipgloss tables require the full charm stack. tablewriter is purpose-built and lightweight. |
| Config | env/v11 + flags | viper | Viper is heavy (pulls in many deps). env/v11 + Cobra flags cover all needs. |

## Project Structure

```
ember-claw/
  cmd/
    ember-claw/           # CLI binary entry point
      main.go
    sidecar/              # gRPC sidecar binary entry point
      main.go
  internal/
    cli/                  # CLI command implementations
      deploy.go
      list.go
      delete.go
      status.go
      logs.go
      chat.go
    sidecar/              # Sidecar server implementation
      server.go           # gRPC server, stdin/stdout bridge
      process.go          # PicoClaw process management
    k8s/                  # Kubernetes client wrapper
      client.go           # client-go setup, kubeconfig
      deploy.go           # Create/delete deployments, services, PVCs
      portforward.go      # In-process port forwarding for chat
    config/               # Configuration types
      instance.go         # Instance configuration struct
  api/
    proto/                # Proto definitions
      chat.proto          # Chat service definition
    gen/                  # Generated Go code (from protoc)
      chat.pb.go
      chat_grpc.pb.go
  deploy/
    templates/            # K8s manifest templates
      deployment.yaml
      service.yaml
      pvc.yaml
  Makefile                # Build, push, deploy targets
  Dockerfile              # Multi-stage build
  go.mod
  go.sum
```

## Version Alignment Summary

Key principle: **Align with PicoClaw where dependencies overlap.** This minimizes surprises when both binaries run in the same container.

| Dependency | PicoClaw Version | Ember-Claw Version | Notes |
|------------|-----------------|--------------------:|-------|
| Go | 1.25.7 | 1.25 (go.mod) | go.mod says `go 1.25`, runtime is 1.25.x |
| cobra | v1.10.2 | v1.10.2 | Exact match |
| protobuf | v1.36.11 | v1.36.11 | Exact match |
| zerolog | v1.34.0 | v1.34.0 | Exact match |
| uuid | v1.6.0 | v1.6.0 | Exact match |
| readline | v1.5.1 | v1.5.1 | Exact match |
| testify | v1.11.1 | v1.11.1 | Exact match |
| grpc | (not used) | v1.79.2 | New dependency, use latest |
| client-go | (not used) | v0.35.2 | New dependency, use latest |
| env/v11 | v11.3.1 | v11.4.0 | Minor bump OK, latest patch |

## Installation

```bash
# Initialize module
go mod init github.com/LastBotInc/ember-claw

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

# Proto compilation tools (already installed)
# go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11
# go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.0
```

## Proto Compilation

```bash
# From project root
protoc --go_out=api/gen --go_opt=paths=source_relative \
       --go-grpc_out=api/gen --go-grpc_opt=paths=source_relative \
       api/proto/chat.proto
```

## Key Technical Decisions

### 1. Two Separate Binaries (sidecar + CLI)

The sidecar and CLI are separate `cmd/` entry points compiled into separate binaries. The sidecar runs in the container alongside PicoClaw. The CLI runs on the developer's machine. They share the proto-generated types and potentially some internal packages, but have different concerns (server vs. client, container vs. local).

### 2. client-go Port Forwarding for Chat

The `ember-claw chat <instance>` command uses client-go's port-forward API to tunnel a local port to the sidecar's gRPC port inside the pod. This avoids exposing gRPC services via LoadBalancer or Ingress (which would be inappropriate for a dev/test fleet). The port-forward is established, then a gRPC client connects to localhost:forwarded_port.

### 3. Manifest Templates, Not Helm

Kubernetes manifests live as Go templates or simple YAML files with `sed`/`envsubst` substitution in Make targets. This matches the existing umbrella repo pattern and avoids requiring Helm installation. For the ~3 resource types needed (Deployment, Service, PVC), Helm's complexity is not justified.

### 4. Sidecar Process Management

The sidecar starts PicoClaw as a subprocess (os/exec), pipes gRPC streaming messages to PicoClaw's stdin, and reads responses from stdout. When the gRPC stream ends, stdin is closed. When PicoClaw exits, the sidecar detects it and can restart or report an error. This is straightforward os/exec usage -- no need for a process supervisor library.

## Sources

- Go module proxy (proxy.golang.org) -- all version numbers verified via `go list -m -versions`
- PicoClaw go.mod at `/Users/tuomas/Projects/picoclaw/go.mod` -- dependency alignment
- coralie-conference-service go.mod and proto files -- ecosystem patterns
- Parent Makefile at `/Users/tuomas/Projects/Makefile` -- deployment conventions
- protoc, protoc-gen-go, protoc-gen-go-grpc -- versions verified from installed binaries
- System Go version verified: go1.25.1 darwin/arm64
