# Domain Pitfalls

**Domain:** Kubernetes deployment toolkit + gRPC sidecar + Go CLI (ember-claw for PicoClaw)
**Researched:** 2026-03-13
**Confidence:** HIGH (based on PicoClaw source analysis and established K8s/gRPC patterns)

## Critical Pitfalls

Mistakes that cause rewrites or major issues.

### Pitfall 1: Wrong Abstraction Layer for PicoClaw Integration

**What goes wrong:** Building a stdin/stdout pipe bridge to PicoClaw's CLI process. PicoClaw is NOT a simple stdin/stdout tool -- its interactive mode uses `readline` with escape codes, ANSI colors, prompt strings, and terminal history. Parsing that output is brittle and error-prone.
**Why it happens:** Treating PicoClaw as a black-box binary instead of inspecting its Go API. The PROJECT.md describes "bridging stdin/stdout" but this is the wrong abstraction.
**Consequences:** A stdin/stdout bridge must handle: readline escape sequences, prompt detection, ANSI color stripping, output boundary detection (where does one response end?), TTY allocation in containers, and pipe buffering issues. All of these are solved problems if you import PicoClaw as a library.
**Prevention:**
1. Import `github.com/sipeed/picoclaw/pkg/agent` as a Go dependency
2. Use `agent.NewAgentLoop(cfg, msgBus, provider)` to create instances
3. Use `agentLoop.ProcessDirect(ctx, message, sessionKey)` for request/response
4. This is NOT modifying PicoClaw -- it is using its public Go API
**Detection:** If you find yourself writing code to parse terminal output, detect prompts, or allocate PTYs -- stop. Use the library API.

### Pitfall 2: Unnecessary Two-Container Sidecar Pattern

**What goes wrong:** Deploying two containers in one pod (PicoClaw process + gRPC sidecar process) when the sidecar imports PicoClaw as a library. The "sidecar" term in the project description implies two containers, but since the gRPC server embeds PicoClaw directly, there is no second container.
**Why it happens:** Conflating the term "sidecar" (deployment pattern for separate processes) with "wrapper" (code-level adapter).
**Consequences:** Two containers sharing volumes, startup ordering issues, health check coordination, doubled resource requests. All unnecessary.
**Prevention:** Build a single Go binary that IS the gRPC server with PicoClaw embedded. One container per pod. Use "sidecar" only as a conceptual descriptor in documentation.
**Detection:** If your K8s manifest has two containers and one is just PicoClaw with no network ports, you have an unnecessary sidecar.

### Pitfall 3: PVC Lifecycle Not Tied to Instance Lifecycle

**What goes wrong:** `eclaw delete` deletes the Deployment but leaves the PVC behind. Orphaned PVCs accumulate, consuming storage and confusing users. OR: deleting the PVC aggressively destroys data the user wanted to keep.
**Why it happens:** PVCs have independent lifecycle from Pods by design. Kubernetes does not auto-delete PVCs when a Deployment is deleted.
**Consequences:** Storage quota exhaustion, user confusion, name collisions on re-creation.
**Prevention:**
1. `eclaw delete <name>` deletes compute resources only (Deployment, Service, ConfigMap)
2. `eclaw delete <name> --purge` also deletes PVC and Secret
3. `eclaw list` shows PVC status alongside instance status
4. Consistent naming: PVC = `picoclaw-{name}-data`
**Detection:** Run `kubectl get pvc -n <namespace>` and compare with running instances.

### Pitfall 4: API Key Leaks in Manifests

**What goes wrong:** PicoClaw requires AI provider API keys per instance. Keys end up in plaintext in Deployment manifests, shell history, or ConfigMaps.
**Why it happens:** The interactive Make target pattern (prompting for values) feels safe, but values still end up as plaintext in applied manifests.
**Consequences:** Keys visible in `kubectl get deployment -o yaml`, in shell history, or in git if manifests are committed.
**Prevention:**
1. Use Kubernetes Secrets for all API keys
2. `eclaw deploy --api-key` stores the value in a Secret, never in the Deployment spec
3. Mount Secret as a file into PicoClaw's config directory
4. Never log or echo API key values
**Detection:** If `kubectl get deployment <name> -o yaml` shows API key values in plaintext env vars, this pitfall is active.

## Moderate Pitfalls

### Pitfall 5: Port-Forward Fragility for Chat

**What goes wrong:** Using `kubectl port-forward` for sustained gRPC streaming connections. Port-forward is designed for debugging, not production use. It drops connections silently, has no reconnection logic, and breaks on pod restarts.
**Why it happens:** Easiest way to reach a pod without configuring ingress. Developers prototype with it and never replace it.
**Prevention:**
1. Use client-go's SPDY port-forward API (not shelling out to kubectl)
2. Port-forward to the Service, not directly to the pod (survives pod restarts)
3. Implement reconnection logic in the CLI: detect stream breaks, re-establish, display "reconnecting..."
4. Set reasonable gRPC keepalive parameters
**Detection:** If the CLI shells out to `kubectl port-forward` as a subprocess, this pitfall is active.

### Pitfall 6: PicoClaw Config File Path Resolution

**What goes wrong:** PicoClaw defaults to `~/.picoclaw/config.json` for its config file. When imported as a library, the "home" directory inside a container is different from what PicoClaw expects. Config loading fails or uses wrong paths.
**Why it happens:** PicoClaw's `internal.LoadConfig()` resolves `~/.picoclaw/` using `os.UserHomeDir()`. In a container running as uid 1000, this resolves to `/home/picoclaw/`.
**Prevention:**
1. Mount PVC at `/home/picoclaw/.picoclaw/` (matching PicoClaw's expected path)
2. OR: Set `HOME` environment variable in the container to point to the data directory
3. OR: Use PicoClaw's `config.LoadConfig(path)` function directly with an explicit path
4. Test config loading early in development with the actual container user
**Detection:** If the sidecar starts but cannot find config, or uses an empty/default config, check the path resolution.

### Pitfall 7: gRPC Streaming Connection Lifecycle Mismanagement

**What goes wrong:** Interactive chat streams not properly cleaned up on client disconnect. Server-side goroutines leak, or half-closed streams leave the server in a broken state.
**Why it happens:** gRPC streaming requires explicit lifecycle management. Unlike unary RPCs, streams must handle: client cancellation, server-side send after client closes, network interruption.
**Prevention:**
1. Always use context with timeout/cancellation for stream operations
2. Handle `io.EOF` (clean close) and `codes.Canceled` (client went away) distinctly
3. Use gRPC keepalive: `keepalive.ServerParameters{MaxConnectionAge: 1h}`
4. Add concurrent stream limits to prevent resource exhaustion
5. Log stream lifecycle events for debugging
**Detection:** Monitor goroutine count over time. If it only goes up, streams are leaking.

### Pitfall 8: Make Interactive Prompts Are Untestable

**What goes wrong:** Interactive Make targets (prompting for instance name, API keys) are impossible to script, break in CI, and create inconsistent deployments.
**Why it happens:** Make is a build tool, not an interactive CLI framework.
**Prevention:**
1. Make targets accept ALL parameters as variables: `make deploy INSTANCE=my-bot API_KEY=xxx`
2. Interactive prompts are a convenience layer ON TOP of variable-based targets
3. The Go CLI (`eclaw deploy`) should be the primary deployment interface
4. Validate instance names against K8s naming rules
**Detection:** If `make deploy` cannot be run non-interactively, this pitfall is active.

### Pitfall 9: Rancher-Managed Cluster RBAC Surprises

**What goes wrong:** The emberchat cluster is Rancher-managed. Rancher adds RBAC restrictions that cause "forbidden" errors for some operations but not others.
**Why it happens:** Rancher kubeconfigs often have scoped permissions. Creating PVCs may require different permissions than creating Deployments.
**Prevention:**
1. Test ALL required operations with the actual kubeconfig before writing code
2. Required operations: create/list/delete for Deployments, PVCs, Secrets, ConfigMaps, Services, Pods (for logs), Pod/portforward
3. Document minimum RBAC permissions required
4. Use an existing namespace, do not create new ones
**Detection:** Any `kubectl` operation returning 403 Forbidden.

## Minor Pitfalls

### Pitfall 10: Instance Naming Collisions

**What goes wrong:** Users choose names that are invalid K8s resource names (uppercase, special chars, too long) or collide with existing resources.
**Prevention:** Validate names: `^[a-z][a-z0-9-]{0,61}[a-z0-9]$`. Prefix all resources with `picoclaw-`. Check for existing resources before creating.

### Pitfall 11: PicoClaw Session State Confusion

**What goes wrong:** If the sidecar uses a fixed session key for all connections, all users share one conversation. If random per stream, reconnecting starts fresh.
**Prevention:** Use instance name as default session key. Allow override via `--session` flag. This way reconnecting resumes the same conversation by default.

### Pitfall 12: Resource Limits That Starve PicoClaw

**What goes wrong:** Setting CPU/memory limits too low. PicoClaw advertises <10MB RAM for the core binary, but with AI provider SDKs, conversation history, and gRPC overhead, actual usage is 50-100MB.
**Prevention:** Default requests: 50m CPU, 64Mi memory. Default limits: 200m CPU, 256Mi memory. Profile actual usage. Make configurable via deploy flags.

### Pitfall 13: PicoClaw Module Dependency Conflicts

**What goes wrong:** PicoClaw's go.mod has 90+ dependencies (including provider SDKs, MCP, etc.). Adding client-go (which also has many dependencies) may cause version conflicts in the sidecar binary.
**Prevention:** Use `go mod tidy` early. If conflicts arise, the sidecar and CLI can be in separate Go modules (separate go.mod files) within the same repo. Only the sidecar needs PicoClaw; only the CLI needs client-go.

### Pitfall 14: Ignoring PicoClaw's Health Check Endpoint

**What goes wrong:** Implementing custom health checks instead of using PicoClaw's existing `/health` and `/ready` endpoints on port 18790. Pod appears "healthy" even when PicoClaw's agent loop has not initialized.
**Prevention:** Start PicoClaw's health server alongside gRPC server. Use `/ready` as K8s readiness probe. Alternatively, implement gRPC health protocol that delegates to PicoClaw's internal state.

## Phase-Specific Warnings

| Phase Topic | Likely Pitfall | Mitigation |
|-------------|---------------|------------|
| Architecture design | Pitfall 1 (wrong abstraction), Pitfall 2 (unnecessary sidecar) | Import PicoClaw as library. Single binary, one container. |
| Manifest creation | Pitfall 3 (PVC lifecycle), Pitfall 4 (secrets), Pitfall 12 (resources) | PVC naming convention. K8s Secrets for API keys. Profile before setting limits. |
| gRPC server | Pitfall 7 (stream lifecycle), Pitfall 11 (session keys), Pitfall 14 (health) | Context-based cleanup. Instance-name session keys. Delegate to PicoClaw health. |
| CLI tool | Pitfall 5 (port-forward), Pitfall 8 (Make prompts), Pitfall 10 (naming) | client-go port-forward. CLI as primary, Make as wrapper. Validate names. |
| Pre-development | Pitfall 9 (Rancher RBAC) | Test all K8s operations with actual kubeconfig first. |
| Docker build | Pitfall 6 (config paths), Pitfall 13 (dependency conflicts) | Test config loading in container. Separate go.mod if needed. |

## Sources

- PicoClaw source code at `/Users/tuomas/Projects/picoclaw/` (direct analysis)
  - `cmd/picoclaw/internal/agent/helpers.go` -- readline-based interactive mode
  - `pkg/agent/loop.go` -- AgentLoop.ProcessDirect() API
  - `pkg/health/server.go` -- Health check server on port 18790
  - `docker/Dockerfile` -- Container build, onboard command, user creation
  - `go.mod` -- Public module path, dependency tree
- Coralie umbrella repo patterns at `/Users/tuomas/Projects/Makefile`
- Coralie deployment manifests at `/Users/tuomas/Projects/coralie-gemini-worker/deploy/`
- PROJECT.md at `/Users/tuomas/Projects/ember-claw/.planning/PROJECT.md`
