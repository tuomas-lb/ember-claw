# Phase 1: Proto + Sidecar - Research

**Researched:** 2026-03-16
**Domain:** Go gRPC server wrapping PicoClaw as a library, protobuf service definition, K8s health probes
**Confidence:** HIGH

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| GRPC-01 | gRPC server binary imports PicoClaw as Go library using `ProcessDirect()` API | `AgentLoop.ProcessDirect(ctx, content, sessionKey)` confirmed in `pkg/agent/loop.go:627`. Full startup sequence documented in Gateway mode. |
| GRPC-02 | Bidirectional streaming RPC for interactive chat sessions | Standard `google.golang.org/grpc` bidi stream pattern. Session isolation achieved via distinct sessionKey per stream. |
| GRPC-03 | Unary RPC for single-shot queries | Trivially wraps `ProcessDirect()` in a unary handler. Same sessionKey logic applies. |
| GRPC-04 | Health check RPC for readiness/liveness probes | PicoClaw ships `pkg/health.Server` with `/health` and `/ready` HTTP endpoints. Sidecar reuses this plus standard `grpc.health.v1` for gRPC probes. |
| GRPC-05 | Session isolation per gRPC client connection | `sessionKey` parameter in `ProcessDirect` is the isolation mechanism. Each stream gets a unique key (e.g., UUID per connection). |
| K8S-04 | K8s liveness/readiness probes wired to health check endpoint | PicoClaw's `health.Server` on port 8080 serves `/health` (liveness) and `/ready` (readiness). Probe config researched and documented. |
</phase_requirements>

---

## Summary

Phase 1 produces a single Go binary — the ember-claw sidecar — that imports PicoClaw as a Go library and exposes it via gRPC on port 50051. The binary has no external process dependencies; PicoClaw runs embedded in the same process.

The PicoClaw API is clean and well-suited for this integration. `AgentLoop.ProcessDirect(ctx, content, sessionKey)` returns a complete response string. Session isolation between concurrent gRPC connections is achieved by passing a unique `sessionKey` per client connection. The `ProcessDirect` call is thread-safe (verified: each call routes through a message bus with per-session history).

PicoClaw's config loading is controlled via the `PICOCLAW_HOME` environment variable, which resolves the config file path to `$PICOCLAW_HOME/config.json`. In a container this is set to the PVC mount path (e.g., `/data/.picoclaw`), eliminating any home-directory ambiguity. PicoClaw also ships a `pkg/health` HTTP server with `/health` and `/ready` endpoints that the sidecar reuses for Kubernetes probes.

**Primary recommendation:** Build the sidecar in two logical parts: (1) the protobuf service definition + generated Go code, (2) the sidecar binary that wires AgentLoop to the gRPC server with health checks. These can be two separate implementation tasks within Phase 1.

---

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| google.golang.org/grpc | v1.79.2 | gRPC server on port 50051 | Latest stable; Coralie ecosystem uses gRPC throughout |
| google.golang.org/protobuf | v1.36.11 | Protobuf runtime | PicoClaw uses this exact version; exact match avoids conflicts |
| github.com/sipeed/picoclaw | pin to commit | PicoClaw agent library | The library being wrapped; commit-pinned for reproducibility |
| github.com/rs/zerolog | v1.34.0 | Structured logging | PicoClaw uses this exact version; zerolog in the embedded library means consistent log format |
| github.com/caarlos0/env/v11 | v11.4.0 | Env var parsing for sidecar config | PicoClaw uses v11.3.1; minor bump resolves to highest compatible |
| github.com/google/uuid | v1.6.0 | Session key generation per gRPC connection | PicoClaw uses this exact version |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| google.golang.org/grpc/health | bundled with grpc | Standard gRPC health protocol | K8s 1.24+ supports gRPC health probes natively |
| github.com/sipeed/picoclaw/pkg/health | (in-tree) | HTTP /health and /ready endpoints | Reuse PicoClaw's health server for HTTP probes |
| github.com/stretchr/testify | v1.11.1 | Test assertions | PicoClaw uses this exact version |

### Proto Compilation Tools (already installed on system)
| Tool | Version | Location |
|------|---------|----------|
| protoc | 33.2 | `$(which protoc)` |
| protoc-gen-go | v1.36.11 | `/Users/tuomas/go/bin/protoc-gen-go` |
| protoc-gen-go-grpc | v1.6.0 | `/Users/tuomas/go/bin/protoc-gen-go-grpc` |

**Installation (Phase 1 subset only):**
```bash
go mod init github.com/LastBotInc/ember-claw
go get github.com/sipeed/picoclaw@<commit-hash>
go get google.golang.org/grpc@v1.79.2
go get google.golang.org/protobuf@v1.36.11
go get github.com/rs/zerolog@v1.34.0
go get github.com/caarlos0/env/v11@v11.4.0
go get github.com/google/uuid@v1.6.0
go get github.com/stretchr/testify@v1.11.1
go mod tidy
```

---

## Architecture Patterns

### Recommended Project Structure (Phase 1 scope)
```
ember-claw/
  cmd/
    sidecar/
      main.go              # Entry point: load config, start AgentLoop, start gRPC server
  internal/
    server/
      server.go            # gRPC service implementation (PicoClawServiceServer)
      health.go            # Wires PicoClaw health.Server to gRPC + HTTP probes
      session.go           # Session key management per gRPC connection
  proto/
    emberclaw/v1/
      service.proto        # Protobuf service definition
  gen/
    emberclaw/v1/
      service.pb.go        # Generated (do not edit)
      service_grpc.pb.go   # Generated (do not edit)
  go.mod
  go.sum
```

### Pattern 1: PicoClaw Initialization Sequence

**What:** How to bootstrap an AgentLoop from config in a container
**When to use:** In `cmd/sidecar/main.go`
**Source:** Verified from `cmd/picoclaw/internal/gateway/helpers.go:62-117` and `cmd/picoclaw/internal/helpers.go:14-31`

```go
// Source: cmd/picoclaw/internal/helpers.go + pkg/config/config.go
// PICOCLAW_HOME env var controls config path resolution
// In container: set PICOCLAW_HOME=/data/.picoclaw
cfg, err := config.LoadConfig(configPath)  // configPath from PICOCLAW_CONFIG or $PICOCLAW_HOME/config.json

provider, modelID, err := providers.CreateProvider(cfg)
if modelID != "" {
    cfg.Agents.Defaults.ModelName = modelID
}

msgBus := bus.NewMessageBus()
agentLoop := agent.NewAgentLoop(cfg, msgBus, provider)

// Start the agent loop goroutine (required for background tasks)
ctx, cancel := context.WithCancel(context.Background())
defer cancel()
go agentLoop.Run(ctx)
```

### Pattern 2: ProcessDirect for gRPC Handler

**What:** The core call that bridges a gRPC request to PicoClaw
**When to use:** In every gRPC handler that needs an AI response
**Source:** Verified from `pkg/agent/loop.go:627-631`

```go
// Source: pkg/agent/loop.go
// ProcessDirect signature:
// func (al *AgentLoop) ProcessDirect(ctx context.Context, content, sessionKey string) (string, error)
//
// sessionKey = the session identifier for conversation history continuity
// Use a unique key per gRPC client stream for isolation (GRPC-05)

func (s *Server) Chat(stream emberclaw.PicoClawService_ChatServer) error {
    sessionKey := "grpc:" + uuid.New().String()  // unique per connection
    for {
        req, err := stream.Recv()
        if err == io.EOF {
            return nil
        }
        if err != nil {
            return err
        }
        response, err := s.agentLoop.ProcessDirect(stream.Context(), req.Message, sessionKey)
        if err != nil {
            return status.Errorf(codes.Internal, "agent error: %v", err)
        }
        if err := stream.Send(&emberclaw.ChatResponse{Text: response, Done: true}); err != nil {
            return err
        }
    }
}
```

### Pattern 3: Config Path Resolution for Container

**What:** PicoClaw resolves config via `PICOCLAW_HOME` env var
**When to use:** Container environment variable setup
**Source:** Verified from `cmd/picoclaw/internal/helpers.go:14-19` and `pkg/config/defaults.go:16-19`

```
Priority chain:
1. PICOCLAW_CONFIG env var → full path to config.json
2. PICOCLAW_HOME env var → $PICOCLAW_HOME/config.json
3. Default → ~/.picoclaw/config.json (os.UserHomeDir)

Container solution: Set PICOCLAW_HOME=/data/.picoclaw
PVC is mounted at /data/.picoclaw/
Config file at /data/.picoclaw/config.json
Workspace at /data/.picoclaw/workspace (default from DefaultConfig)
```

### Pattern 4: Health Server Reuse

**What:** PicoClaw ships `pkg/health.Server` with `/health` and `/ready` endpoints
**When to use:** For K8s probes (K8S-04)
**Source:** Verified from `pkg/health/server.go`

```go
// Source: pkg/health/server.go
// health.NewServer(host, port) creates an HTTP server
// /health always returns 200 {"status":"ok","uptime":"..."}
// /ready returns 503 until SetReady(true) is called, then 200

healthServer := health.NewServer("0.0.0.0", 8080)
healthServer.SetReady(false)  // Not ready until agentLoop is initialized

// After agentLoop is ready:
healthServer.SetReady(true)

// Run in goroutine
go func() {
    if err := healthServer.Start(); err != nil && err != http.ErrServerClosed {
        log.Fatal().Err(err).Msg("health server failed")
    }
}()
```

Kubernetes probe config:
```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 30

readinessProbe:
  httpGet:
    path: /ready
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10
```

### Pattern 5: Proto Service Definition

**What:** The protobuf contract for the sidecar gRPC service
**When to use:** Define first; both sidecar and future CLI depend on generated code

```protobuf
syntax = "proto3";
package emberclaw.v1;
option go_package = "github.com/LastBotInc/ember-claw/gen/emberclaw/v1;emberclaw";

service PicoClawService {
  // Bidirectional streaming for interactive chat (GRPC-02)
  rpc Chat(stream ChatRequest) returns (stream ChatResponse);

  // Single-shot query (GRPC-03)
  rpc Query(QueryRequest) returns (QueryResponse);

  // Instance status (used by CLI, needed for Status RPC)
  rpc Status(StatusRequest) returns (StatusResponse);
}

message ChatRequest {
  string message = 1;
  // session_key is optional; if empty, server assigns one per connection
  string session_key = 2;
}

message ChatResponse {
  string text = 1;
  bool done = 2;   // true when response is complete
  string error = 3;
}

message QueryRequest {
  string message = 1;
  string session_key = 2;  // optional
}

message QueryResponse {
  string text = 1;
  string error = 2;
}

message StatusRequest {}

message StatusResponse {
  bool ready = 1;
  string model = 2;
  string provider = 3;
  int64 uptime_seconds = 4;
}
```

Proto compilation:
```bash
# From project root
protoc --go_out=gen --go_opt=paths=source_relative \
       --go-grpc_out=gen --go-grpc_opt=paths=source_relative \
       proto/emberclaw/v1/service.proto
```

### Pattern 6: gRPC Server Startup with Graceful Shutdown

```go
// Source: standard grpc server pattern, verified against Coralie services
grpcServer := grpc.NewServer(
    grpc.KeepaliveParams(keepalive.ServerParameters{
        MaxConnectionAge:      2 * time.Hour,
        MaxConnectionAgeGrace: 5 * time.Minute,
    }),
)

// Register service
emberclaw.RegisterPicoClawServiceServer(grpcServer, &Server{agentLoop: al})

// Register gRPC health service (for K8s gRPC probes, K8S-04)
grpcHealthSrv := grpchealth.NewServer()
grpchealth.RegisterHealthServer(grpcServer, grpcHealthSrv)
grpcHealthSrv.SetServingStatus("", grpchealth.Serving)

lis, err := net.Listen("tcp", ":50051")
go grpcServer.Serve(lis)

// Graceful shutdown on SIGTERM/SIGINT
sig := make(chan os.Signal, 1)
signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
<-sig
grpcServer.GracefulStop()
agentLoop.Stop()
agentLoop.Close()
```

### Anti-Patterns to Avoid

- **Subprocess wrapping:** Do not shell out to the picoclaw binary. `ProcessDirect()` is the clean API. (See PITFALLS.md Pitfall 1)
- **Two-container pod:** The sidecar IS PicoClaw. One binary, one container. (See PITFALLS.md Pitfall 2)
- **Shared session key:** Never use a fixed session key for all connections. Each gRPC client stream gets its own UUID-based key. (Pitfall 11)
- **Skipping `agentLoop.Run(ctx)`:** The gateway source shows `go agentLoop.Run(ctx)` is called. Without this goroutine, background tasks (cron, heartbeat) won't run. For sidecar this may be optional since we don't use those features, but it is safest to start it.
- **Blocking on ProcessDirect without context timeout:** Always pass a context with appropriate timeout/cancellation so long-running LLM calls can be cancelled on client disconnect.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Config file path resolution | Custom `$HOME` or hardcoded paths | `PICOCLAW_HOME` env var (built into PicoClaw) | Already implemented and tested in PicoClaw internals |
| HTTP health endpoints | Custom `/health` HTTP server | `pkg/health.Server` from PicoClaw | Already ships with `/health` and `/ready` semantics matching K8s expectations |
| Session storage | Custom session map | PicoClaw session store (in AgentLoop) | PicoClaw persists sessions to disk via JSONL store; reuse this |
| gRPC health protocol | Custom health RPC | `google.golang.org/grpc/health` | Standard protocol; K8s understands it natively since v1.24 |
| LLM provider plumbing | Any provider code | `providers.CreateProvider(cfg)` | Already handles multi-provider, fallback chains, API key loading |

**Key insight:** The sidecar is a thin adapter. Every "real" capability lives in PicoClaw's library. The sidecar only does: load config, call `NewAgentLoop`, call `ProcessDirect`, route result to gRPC stream.

---

## Common Pitfalls

### Pitfall 1: Config Path Breaks in Container
**What goes wrong:** Sidecar starts but PicoClaw can't find config, falls back to empty defaults, LLM calls fail silently or use wrong model.
**Why it happens:** PicoClaw defaults to `~/.picoclaw/config.json` via `os.UserHomeDir()`. Container uid 1000 has home `/home/picoclaw` or no home at all.
**How to avoid:** Set `PICOCLAW_HOME=/data/.picoclaw` in the container spec. This is the PVC mount point. Config file at `/data/.picoclaw/config.json`.
**Warning signs:** AgentLoop initializes but all LLM calls return errors about missing API keys or unknown model.

### Pitfall 2: Session Key Collision Breaks Isolation (GRPC-05)
**What goes wrong:** All concurrent gRPC streams share conversation history, causing responses to bleed across clients.
**Why it happens:** Using a static session key like `"grpc:default"` for all connections.
**How to avoid:** Generate `uuid.New().String()` at stream start. Use it as session key for the lifetime of that stream. Let client optionally override via `session_key` field in `ChatRequest`.
**Warning signs:** Two simultaneous chat sessions where one client sees responses intended for the other.

### Pitfall 3: Missing `agentLoop.Run(ctx)` Goroutine
**What goes wrong:** `ProcessDirect` works for basic queries, but some internal AgentLoop features (subagent spawning, async tool results) silently fail.
**Why it happens:** The gateway mode always calls `go agentLoop.Run(ctx)` but it's easy to miss when building a minimal sidecar.
**How to avoid:** Always call `go agentLoop.Run(ctx)` after `NewAgentLoop`, before accepting gRPC connections.
**Warning signs:** Tool calls inside PicoClaw agents time out or return empty results.

### Pitfall 4: ProcessDirect is Blocking — gRPC Stream Context Must Flow Through
**What goes wrong:** Client disconnects mid-response but the server keeps running the LLM call, consuming API credits and goroutine resources.
**Why it happens:** Not passing `stream.Context()` to `ProcessDirect`. If you pass `context.Background()` instead, cancellation never propagates.
**How to avoid:** Always use `ProcessDirect(stream.Context(), message, sessionKey)`. The context carries the gRPC stream's cancellation signal.
**Warning signs:** Server goroutine count grows over time as abandoned LLM calls pile up.

### Pitfall 5: go.mod Dependency Conflict Between PicoClaw and New Dependencies
**What goes wrong:** `go mod tidy` fails or produces unexpected version overrides when PicoClaw's large dependency tree (90+ packages including provider SDKs) conflicts with client-go or other new imports.
**Why it happens:** PicoClaw pulls in many provider SDKs (anthropic, openai, discord, etc.). New deps may require different versions of shared transitive dependencies.
**How to avoid:** Run `go mod tidy` early after adding PicoClaw + grpc as the first two dependencies. For Phase 1 (sidecar only), client-go is NOT needed — only add it in Phase 2 for the CLI. Keeping Phase 1 deps minimal reduces conflict surface.
**Warning signs:** `go mod tidy` shows `go: inconsistent vendoring` or `requires go@... but got go@...`.

### Pitfall 6: Proto Package Path Mismatch
**What goes wrong:** Generated `.pb.go` files import paths don't match the project's module path, causing compile errors.
**Why it happens:** `option go_package` in the proto file must match the Go module + `gen/` path.
**How to avoid:** Set `option go_package = "github.com/LastBotInc/ember-claw/gen/emberclaw/v1;emberclaw"` in `service.proto`. The path before `;` is the import path; after `;` is the package alias.
**Warning signs:** `cannot find package "..."` errors when building sidecar.

---

## Code Examples

### Full Minimal Sidecar Main
```go
// Source: synthesized from pkg/agent/loop.go, cmd/picoclaw/internal/gateway/helpers.go,
//         pkg/health/server.go (all verified by direct source inspection)
package main

import (
    "context"
    "net"
    "os"
    "os/signal"
    "syscall"

    "github.com/rs/zerolog/log"
    "github.com/sipeed/picoclaw/pkg/agent"
    "github.com/sipeed/picoclaw/pkg/bus"
    "github.com/sipeed/picoclaw/pkg/config"
    "github.com/sipeed/picoclaw/pkg/health"
    "github.com/sipeed/picoclaw/pkg/providers"
    "google.golang.org/grpc"
    grpchealth "google.golang.org/grpc/health"
    healthpb "google.golang.org/grpc/health/grpc_health_v1"

    emberclaw "github.com/LastBotInc/ember-claw/gen/emberclaw/v1"
    "github.com/LastBotInc/ember-claw/internal/server"
)

func main() {
    // Config path: PICOCLAW_CONFIG > $PICOCLAW_HOME/config.json > ~/.picoclaw/config.json
    configPath := getConfigPath()
    cfg, err := config.LoadConfig(configPath)
    if err != nil {
        log.Fatal().Err(err).Msg("failed to load config")
    }

    provider, modelID, err := providers.CreateProvider(cfg)
    if err != nil {
        log.Fatal().Err(err).Msg("failed to create provider")
    }
    if modelID != "" {
        cfg.Agents.Defaults.ModelName = modelID
    }

    msgBus := bus.NewMessageBus()
    agentLoop := agent.NewAgentLoop(cfg, msgBus, provider)
    defer agentLoop.Close()

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    go agentLoop.Run(ctx)

    // Health server (HTTP, for K8s probes)
    healthSrv := health.NewServer("0.0.0.0", 8080)
    healthSrv.SetReady(true)
    go healthSrv.Start()

    // gRPC server
    lis, err := net.Listen("tcp", ":50051")
    if err != nil {
        log.Fatal().Err(err).Msg("failed to listen")
    }
    grpcSrv := grpc.NewServer()
    emberclaw.RegisterPicoClawServiceServer(grpcSrv, server.New(agentLoop))
    grpcHealthSrv := grpchealth.NewServer()
    healthpb.RegisterHealthServer(grpcSrv, grpcHealthSrv)
    grpcHealthSrv.SetServingStatus("", grpchealth.Serving)
    go grpcSrv.Serve(lis)

    sig := make(chan os.Signal, 1)
    signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
    <-sig
    grpcSrv.GracefulStop()
    agentLoop.Stop()
}

func getConfigPath() string {
    if p := os.Getenv("PICOCLAW_CONFIG"); p != "" {
        return p
    }
    home := os.Getenv("PICOCLAW_HOME")
    if home == "" {
        h, _ := os.UserHomeDir()
        home = h + "/.picoclaw"
    }
    return home + "/config.json"
}
```

### Session Key Strategy (GRPC-05)
```go
// Source: design based on pkg/agent/loop.go ProcessDirect signature
// Each bidi stream gets a unique session key at connection start.
// If client provides session_key in ChatRequest, honor it (for resume).
// Otherwise assign UUID.

func (s *Server) Chat(stream emberclaw.PicoClawService_ChatServer) error {
    // Assign session key once per stream
    assignedKey := "grpc:" + uuid.New().String()
    sessionKey := assignedKey

    for {
        req, err := stream.Recv()
        if err == io.EOF {
            return nil
        }
        if err != nil {
            return status.Errorf(codes.Canceled, "stream closed: %v", err)
        }
        // Client can override session key (e.g., to resume a previous conversation)
        if req.SessionKey != "" {
            sessionKey = req.SessionKey
        }
        response, err := s.agentLoop.ProcessDirect(stream.Context(), req.Message, sessionKey)
        if err != nil {
            return status.Errorf(codes.Internal, "agent: %v", err)
        }
        if err := stream.Send(&emberclaw.ChatResponse{Text: response, Done: true}); err != nil {
            return err
        }
    }
}
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| PicoClaw config only via `~/.picoclaw/` | `PICOCLAW_HOME` env var overrides base path | Current (confirmed in codebase) | Containers set this env var, no hacks needed |
| Subprocess stdin/stdout bridge | Library import via `ProcessDirect()` | PicoClaw v1+ design | Clean Go API, no TTY/readline issues |
| gRPC health as custom RPC | Standard `grpc.health.v1` + HTTP `/health` `/ready` | K8s 1.24+ | K8s natively understands both formats |

**Deprecated/outdated:**
- Legacy PicoClaw providers config (`providers.openai.api_key` in config.json): replaced by `model_list` format. Both work via migration, but new config should use `model_list`.

---

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` + `github.com/stretchr/testify` v1.11.1 |
| Config file | none (Go testing is built-in, no config file needed) |
| Quick run command | `go test ./internal/server/... -timeout 30s` |
| Full suite command | `go test ./... -timeout 120s` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| GRPC-01 | AgentLoop initializes from config, `ProcessDirect` returns non-empty response | unit (with mock provider) | `go test ./internal/server/... -run TestProcessDirect -timeout 30s` | Wave 0 |
| GRPC-02 | Bidi stream receives message, sends response, handles EOF | unit | `go test ./internal/server/... -run TestChat -timeout 30s` | Wave 0 |
| GRPC-03 | Unary Query handler returns response string | unit | `go test ./internal/server/... -run TestQuery -timeout 30s` | Wave 0 |
| GRPC-04 | `/health` returns 200; `/ready` returns 503 before SetReady, 200 after | unit | `go test ./internal/server/... -run TestHealth -timeout 30s` | Wave 0 |
| GRPC-05 | Two concurrent streams use different session keys, histories are isolated | unit | `go test ./internal/server/... -run TestSessionIsolation -timeout 30s` | Wave 0 |
| K8S-04 | K8s probe YAML fields match health server ports and paths | manual inspection | n/a | Wave 0 (doc only) |

**Note on testing AgentLoop:** Unit tests for gRPC handlers should use a mock/stub for `ProcessDirect` rather than spinning up a full PicoClaw AgentLoop (which requires a live LLM API key). Define a small interface:
```go
type AgentProcessor interface {
    ProcessDirect(ctx context.Context, content, sessionKey string) (string, error)
}
```
The production `*agent.AgentLoop` satisfies this interface. Tests inject a mock.

### Sampling Rate
- **Per task commit:** `go test ./internal/server/... -timeout 30s`
- **Per wave merge:** `go test ./... -timeout 120s`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/server/server_test.go` — covers GRPC-01, GRPC-02, GRPC-03, GRPC-05 via mock
- [ ] `internal/server/health_test.go` — covers GRPC-04
- [ ] `go.mod` and `go.sum` — module not yet initialized
- [ ] `gen/emberclaw/v1/*.pb.go` — proto not yet compiled
- [ ] Framework install: `go mod tidy` after init

---

## Open Questions

1. **PicoClaw module path availability**
   - What we know: PicoClaw's `go.mod` declares `module github.com/sipeed/picoclaw`
   - What's unclear: Is this module published to the public Go module proxy, or must it be used via `go.work` (local workspace) or a `replace` directive?
   - Recommendation: Test with `go get github.com/sipeed/picoclaw@HEAD` first. If not public, use a `go.work` workspace file pointing to `/Users/tuomas/Projects/picoclaw`. Add `go.work` to `.gitignore` and `.dockerignore` for local dev isolation.

2. **go.mod dependency conflicts**
   - What we know: PicoClaw has 90+ transitive dependencies including large provider SDKs.
   - What's unclear: Whether any of these conflict with grpc v1.79.2 until we actually run `go mod tidy`.
   - Recommendation: Run `go mod tidy` as the very first action in Wave 1. If conflicts arise, the sidecar can have its own `go.mod` (separate from CLI, which will add client-go in Phase 2).

3. **`agentLoop.Run(ctx)` necessity for minimal sidecar**
   - What we know: The gateway mode calls `go agentLoop.Run(ctx)`. The sidecar does not use cron, heartbeat, or channels.
   - What's unclear: Whether `ProcessDirect` blocks or fails without `Run` being active.
   - Recommendation: Always call `go agentLoop.Run(ctx)` to match the known-good gateway pattern. Investigate if it can be skipped safely, but default to including it.

---

## Sources

### Primary (HIGH confidence)
- `/Users/tuomas/Projects/picoclaw/pkg/agent/loop.go` — `ProcessDirect` signature (line 627), `NewAgentLoop` constructor (line 79), `AgentLoop` struct
- `/Users/tuomas/Projects/picoclaw/pkg/agent/instance.go` — `AgentInstance`, `NewAgentInstance`, session store initialization
- `/Users/tuomas/Projects/picoclaw/pkg/health/server.go` — Full health server implementation (`NewServer`, `SetReady`, `/health`, `/ready` handlers)
- `/Users/tuomas/Projects/picoclaw/cmd/picoclaw/internal/helpers.go` — `GetPicoclawHome()`, `GetConfigPath()`, `PICOCLAW_HOME` and `PICOCLAW_CONFIG` env vars
- `/Users/tuomas/Projects/picoclaw/pkg/config/config.go` — `LoadConfig(path)` function, `Config` struct
- `/Users/tuomas/Projects/picoclaw/pkg/config/defaults.go` — `DefaultConfig()` with `PICOCLAW_HOME` handling
- `/Users/tuomas/Projects/picoclaw/cmd/picoclaw/internal/gateway/helpers.go` — Full gateway startup sequence showing `NewAgentLoop` + `Run` + health server pattern
- `/Users/tuomas/Projects/picoclaw/go.mod` — PicoClaw module path (`github.com/sipeed/picoclaw`), dependency versions
- `.planning/research/STACK.md` — Verified library versions via Go module proxy (2026-03-13)
- `.planning/research/ARCHITECTURE.md` — Architecture decisions with rationale
- `.planning/research/PITFALLS.md` — Pitfalls catalog

### Secondary (MEDIUM confidence)
- `.planning/research/SUMMARY.md` — Cross-reference for Phase 1 flags
- Coralie umbrella repo gRPC patterns (verified against coralie-conference-service, sip-worker go.mod files)

---

## Metadata

**Confidence breakdown:**
- PicoClaw API (ProcessDirect, health server, config loading): HIGH — direct source code inspection
- gRPC server patterns: HIGH — standard grpc-go patterns, version verified
- Proto definition: HIGH — based on requirements and verified API signatures
- Dependency versions: HIGH — verified via Go module proxy (2026-03-13 research)
- PicoClaw module availability (public vs private): LOW — not yet tested with `go get`
- Dependency conflict prediction: MEDIUM — PicoClaw has many deps; actual conflicts unknown until `go mod tidy`

**Research date:** 2026-03-16
**Valid until:** 2026-04-16 (30 days; grpc-go and protobuf versions are stable)
