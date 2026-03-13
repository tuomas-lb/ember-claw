# Domain Pitfalls

**Domain:** Kubernetes deployment toolkit + gRPC sidecar + Go CLI (ember-claw for PicoClaw)
**Researched:** 2026-03-13
**Confidence:** HIGH (based on PicoClaw source analysis, Kubernetes sidecar docs, and existing Coralie patterns)

## Critical Pitfalls

Mistakes that cause rewrites or major issues.

### Pitfall 1: Wrong Abstraction Layer for PicoClaw Integration

**What goes wrong:** The PROJECT.md describes bridging "PicoClaw stdin/stdout" via a gRPC sidecar. But PicoClaw is NOT a simple stdin/stdout tool. It has a `ProcessDirect(ctx, message, sessionKey)` Go API, a full `gateway` mode with HTTP health checks (port 18790), a channel/plugin architecture, and session management. Building a stdin/stdout pipe bridge wastes effort and produces a fragile, inferior integration.

**Why it happens:** Treating PicoClaw as a black-box binary rather than inspecting its internal API. The PROJECT.md constraint says "do not fork or modify" PicoClaw, which is correct -- but that does not mean you must treat it as an opaque process. PicoClaw is a Go library with importable packages.

**Consequences:** A stdin/stdout bridge must handle: readline escape sequences, prompt detection, ANSI color codes, output boundary detection (where does one response end?), TTY allocation in containers, and buffering issues. All of these are solved problems if you instead import PicoClaw's `pkg/agent` package directly.

**Prevention:**
1. Import `github.com/sipeed/picoclaw/pkg/agent` as a Go dependency in the gRPC sidecar
2. Use `agent.NewAgentLoop(cfg, msgBus, provider)` to create instances programmatically
3. Use `agentLoop.ProcessDirect(ctx, message, sessionKey)` for request/response
4. This is NOT modifying PicoClaw -- it is using its public Go API as intended
5. The gRPC sidecar becomes a thin gRPC-to-AgentLoop adapter, not a process wrapper

**Detection:** If you find yourself writing code to parse terminal output, detect prompts, or allocate PTYs -- you chose the wrong integration point. Stop and use the library API.

**Phase impact:** Phase 1 (gRPC sidecar design). Getting this wrong means rewriting the entire sidecar later.

---

### Pitfall 2: Sidecar vs. Embedded Architecture Confusion

**What goes wrong:** The PROJECT.md describes a "gRPC sidecar/wrapper that runs alongside PicoClaw in each pod." A true Kubernetes sidecar pattern means two separate containers in one pod. But since the gRPC server imports PicoClaw as a Go library (per Pitfall 1 prevention), there is no reason for two containers. You end up with a single Go binary that IS the gRPC server WITH PicoClaw embedded.

**Why it happens:** Conflating the term "sidecar" (a deployment pattern for separate processes) with "wrapper" (a code-level adapter). When the wrapper imports the wrapped library directly, the sidecar container distinction evaporates.

**Consequences:** Unnecessary complexity: two containers sharing volumes, startup ordering issues, health check coordination, doubled resource requests. The Kubernetes native sidecar init container pattern (`restartPolicy: Always`) adds lifecycle complexity with reverse-order shutdown, reduced graceful termination time, and probe requirements -- all for no benefit when a single binary suffices.

**Prevention:**
1. Build a single Go binary: `ember-claw-server` that imports PicoClaw's agent package and exposes gRPC
2. One container per pod, not two
3. Use "sidecar" in documentation only to describe the conceptual role (adapter/bridge), not the deployment topology
4. If there is a genuine reason to run PicoClaw as a separate process (e.g., crash isolation, different resource profiles), THEN use the sidecar pattern -- but justify it explicitly

**Detection:** If your Kubernetes manifest has two containers and one of them is just PicoClaw with no network ports, you have an unnecessary sidecar.

**Phase impact:** Phase 1 (architecture). Determines entire manifest structure and Docker build strategy.

---

### Pitfall 3: kubectl port-forward as Primary Chat Path

**What goes wrong:** Using `kubectl port-forward` as the mechanism for CLI-to-pod gRPC connections. Port-forward is designed for debugging, not sustained use. It drops connections silently under load, has no reconnection logic, creates one TCP tunnel per invocation, and breaks on pod restarts.

**Why it happens:** It is the easiest way to reach a pod from outside the cluster without configuring ingress or NodePort services. Developers prototype with it and never replace it.

**Consequences:**
- Interactive chat sessions drop mid-conversation with no error message
- gRPC streaming connections break silently (port-forward buffers mask the disconnection for seconds)
- Each `ember-claw chat <instance>` invocation spawns a `kubectl port-forward` subprocess that must be lifecycle-managed
- Pod restarts require manual reconnection
- Cannot connect to multiple instances simultaneously without port conflicts

**Prevention:**
1. For dev/test (which this is): Use a Kubernetes Service (ClusterIP) + `kubectl port-forward` to the Service, not directly to the pod. Services survive pod restarts.
2. Better: Use the Kubernetes API directly from the Go CLI via `client-go` SPDY executor for port-forwarding. This gives programmatic reconnection and cleanup.
3. Best for interactive chat: Implement reconnection logic in the CLI. Detect stream breaks, re-establish the port-forward, and resume. Display a clear "reconnecting..." message.
4. Consider a single NodePort or LoadBalancer service for the fleet, with instance routing via gRPC metadata.

**Detection:** If the CLI shells out to `kubectl port-forward` as a subprocess, this pitfall is active.

**Phase impact:** Phase 2 (CLI tool). The chat command's reliability depends entirely on the connection strategy.

---

### Pitfall 4: PVC Lifecycle Not Tied to Instance Lifecycle

**What goes wrong:** PersistentVolumeClaims (PVCs) are created for each PicoClaw instance for persistent memory/planning data, but `ember-claw delete <instance>` only deletes the Deployment/Pod and leaves the PVC behind. Over time, orphaned PVCs accumulate, consuming storage and confusing users. Alternatively, deleting the PVC on instance deletion destroys data the user expected to keep.

**Why it happens:** PVCs have independent lifecycle from Pods by design. Kubernetes does not auto-delete PVCs when a Deployment is deleted. StatefulSets can be configured with `persistentVolumeClaimRetentionPolicy`, but raw Deployments cannot.

**Consequences:**
- Storage quota exhaustion from orphaned PVCs
- User confusion: "I deleted the instance, why is there still a volume?"
- Data loss if PVC deletion is aggressive but user wanted to redeploy with same data
- Name collisions if user re-creates an instance and the old PVC still exists

**Prevention:**
1. Make PVC deletion explicit and separate: `ember-claw delete <instance>` deletes compute resources, `ember-claw delete <instance> --purge` also deletes PVC
2. `ember-claw list` should show PVC status alongside instance status (including orphaned PVCs)
3. Add `ember-claw cleanup` command to find and delete orphaned PVCs
4. Use consistent naming: PVC name = `picoclaw-data-<instance-name>` so the association is obvious
5. Consider using a StatefulSet instead of Deployment if PVC lifecycle management is important

**Detection:** Run `kubectl get pvc -n <namespace>` and compare with running instances. If there are PVCs with no matching pod, the lifecycle is broken.

**Phase impact:** Phase 1 (manifest design) and Phase 2 (CLI delete command).

---

### Pitfall 5: API Key and Secret Management in Manifests

**What goes wrong:** PicoClaw requires AI provider API keys (OpenAI, Anthropic, etc.) per instance. These get hardcoded into Deployment manifests, Make target prompts, or environment variable values in the CLI. Secrets end up in git history, shell history, or plaintext in Kubernetes manifests.

**Why it happens:** The "interactive Make target" pattern (prompting for values) feels safe because it is interactive, but the values still end up as plaintext in the applied manifest. PicoClaw uses environment variables for configuration (`caarlos0/env` package), making it tempting to pass API keys as plain env vars.

**Consequences:**
- API keys in `kubectl get deployment -o yaml` output (visible to anyone with cluster read access)
- Keys in shell history if passed via Make variables
- Keys in git if manifests are committed
- No key rotation mechanism

**Prevention:**
1. Use Kubernetes Secrets for all API keys. The CLI should create a Secret per instance and reference it via `envFrom` in the pod spec.
2. The `deploy` command should accept `--api-key` flag but store the value in a Secret, never in the Deployment spec.
3. Alternatively, accept `--secret-name` to reference a pre-existing Secret.
4. PicoClaw's config file (`~/.picoclaw/config.yaml`) can also hold keys -- mount the Secret as a config file volume.
5. Never log or echo API key values in Make targets or CLI output.

**Detection:** If `kubectl get deployment <name> -o yaml` shows API key values in plaintext env vars, this pitfall is active.

**Phase impact:** Phase 1 (manifest templates) and Phase 2 (CLI deploy command).

## Moderate Pitfalls

### Pitfall 6: PicoClaw Config File vs. Environment Variables Mismatch

**What goes wrong:** PicoClaw supports configuration via both `~/.picoclaw/config.yaml` and environment variables (using `caarlos0/env/v11`). The ember-claw toolkit configures instances via env vars in the pod spec, but PicoClaw reads its config file first. If the container image has a baked-in config from `picoclaw onboard` (which the Dockerfile runs), env vars may not override all settings, or the config file may contain stale defaults.

**Why it happens:** Dual configuration sources with unclear precedence. The `caarlos0/env` library typically overrides config file values, but only for fields that have env tags. Some config fields may only be configurable via the config file.

**Prevention:**
1. Test every configurable parameter to confirm env var override works
2. Mount a per-instance config.yaml via ConfigMap/Secret rather than relying on env vars
3. Document which approach (env vars vs. config file) is the canonical configuration method for ember-claw
4. If using config file: the PVC mount at `~/.picoclaw/` will include both data AND config, which means config changes require volume edits

**Detection:** Instance behaves differently than expected despite correct env vars. Check `picoclaw status` output inside the pod to see actual resolved config.

**Phase impact:** Phase 1 (container configuration strategy).

---

### Pitfall 7: gRPC Streaming Connection Lifecycle Mismanagement

**What goes wrong:** The interactive chat mode uses gRPC bidirectional streaming. Streams are not properly cleaned up on client disconnect, server-side goroutines leak, or half-closed streams leave the server in a broken state.

**Why it happens:** gRPC streaming requires explicit lifecycle management. Unlike unary RPCs, streams must handle: client cancellation (context cancel), server-side send after client closes, network interruption (no explicit close), and keepalive timeouts.

**Consequences:**
- Goroutine leaks on the sidecar (one per abandoned chat session)
- Memory growth over time as leaked `AgentLoop` instances accumulate
- Deadlocked streams blocking new connections
- Silent failures where the client shows "connected" but messages are dropped

**Prevention:**
1. Always use context with timeout/cancellation for stream operations
2. Implement proper `ServerStream.RecvMsg` error handling: `io.EOF` = clean close, `codes.Canceled` = client went away
3. Use a session manager with explicit cleanup: when a stream ends, close the associated `AgentLoop`
4. Set gRPC keepalive parameters: `keepalive.ServerParameters{MaxConnectionAge: 1h, MaxConnectionAgeGrace: 5m}` to prevent zombie connections
5. Add a maximum concurrent streams limit to prevent resource exhaustion
6. Log stream lifecycle events (open/close/error) for debugging

**Detection:** Monitor goroutine count over time (`runtime.NumGoroutine()`). If it only goes up, streams are leaking.

**Phase impact:** Phase 1 (gRPC server implementation).

---

### Pitfall 8: Makefile Interactive Prompts Are Fragile and Untestable

**What goes wrong:** Interactive Make targets that prompt for instance name, API keys, resource limits, etc. are hard to test, impossible to script, break in CI/CD, and create inconsistent deployments because there is no validation of user input.

**Why it happens:** Make is a build tool, not an interactive CLI framework. Using `read -p` in shell snippets within Make targets creates a poor user experience with no input validation, no defaults display, no ability to go back, and no dry-run mode.

**Consequences:**
- Typos in instance names create resources that are hard to find/delete
- No input validation (invalid K8s names, missing required fields)
- Cannot replay or audit a deployment ("what settings did I use?")
- Impossible to use in scripts or automation
- Different users create instances with different configurations because prompts are free-form

**Prevention:**
1. Make targets should accept ALL parameters as Make variables: `make deploy INSTANCE=my-bot API_KEY=xxx MODEL=gpt-4`
2. Interactive prompts should be a convenience layer ON TOP of variable-based targets, not the only path
3. Validate instance names against Kubernetes naming rules (lowercase, alphanumeric, hyphens, max 63 chars)
4. Show defaults and current values during prompts
5. Better yet: the Go CLI (`ember-claw deploy`) should be the primary deployment interface, with Make targets as thin wrappers. The Go CLI can do proper input validation, TUI prompts (with `survey` or `bubbletea`), and dry-run output.

**Detection:** If `make deploy` cannot be run non-interactively, this pitfall is active.

**Phase impact:** Phase 2 (CLI tool design) and Phase 1 (Make target design).

---

### Pitfall 9: Rancher-Managed Cluster RBAC Surprises

**What goes wrong:** The emberchat cluster is Rancher-managed (at `rancher-2.kuupo.com`). Rancher adds its own RBAC layer, namespace restrictions, and resource quotas. The CLI tool or Make targets attempt operations that the kubeconfig user lacks permissions for, producing cryptic "forbidden" errors.

**Why it happens:** Rancher kubeconfigs often have scoped permissions (per-project or per-namespace). Operations like creating PVCs, Secrets, or Services may require different permission levels than creating Deployments.

**Consequences:**
- `ember-claw deploy` works but `ember-claw delete` fails (different RBAC for delete)
- PVC creation fails silently while Deployment creation succeeds
- Namespace creation is blocked (Rancher manages namespaces)
- Service account token issues with `client-go` when Rancher rotates tokens

**Prevention:**
1. Early in development, test ALL required operations with the actual kubeconfig: create/list/delete for Deployments, PVCs, Secrets, Services, Pods (for logs), and Pod/exec (for shell access)
2. Document the minimum RBAC permissions required
3. The CLI should fail fast with a clear error on permission issues, not retry or hang
4. Do NOT create a new namespace -- use an existing one that Rancher has already provisioned
5. Pin the namespace in configuration rather than making it configurable (reduces RBAC surface)

**Detection:** If any `kubectl` operation returns 403 Forbidden, document it immediately and add the required permission to the prerequisites.

**Phase impact:** Phase 0 (pre-development validation). Test permissions BEFORE writing code.

---

### Pitfall 10: Multi-Container Image Build Complexity

**What goes wrong:** Building the ember-claw server requires PicoClaw as a Go dependency. But PicoClaw is a separate repository (`github.com/sipeed/picoclaw`). The Dockerfile cannot `go mod download` if the PicoClaw repo is private, or if you are using a local fork/version. The build process either fails or uses the wrong PicoClaw version.

**Why it happens:** Go modules resolve dependencies from remote repositories by default. If PicoClaw is private or you need local modifications, `go mod replace` directives or vendor directories are needed, and these interact poorly with Docker build contexts.

**Consequences:**
- Docker build fails with "module not found" or authentication errors
- Local development uses a different PicoClaw version than the Docker build
- `go mod replace` with local paths does not work inside Docker (the path does not exist in the build context)

**Prevention:**
1. If PicoClaw is a public GitHub repo (it appears to be, based on the npm-style module path `github.com/sipeed/picoclaw`): no issue, `go mod download` will work
2. If using a specific branch/commit: pin it in `go.mod` with `go get github.com/sipeed/picoclaw@<commit>`
3. For local development: use `go.work` workspace file (NOT `go mod replace`) so local changes are picked up without modifying `go.mod`
4. In the Dockerfile: use multi-stage build with dependency caching (`COPY go.mod go.sum` first, then `go mod download`, then `COPY . .`)
5. Add `go.work` to `.dockerignore` so local workspace config does not leak into Docker builds

**Detection:** If `docker build` fails on `go mod download` or produces a binary with the wrong PicoClaw version, this pitfall is active.

**Phase impact:** Phase 1 (Docker build setup).

## Minor Pitfalls

### Pitfall 11: Instance Naming Collisions and Kubernetes Name Constraints

**What goes wrong:** Users choose instance names that are invalid Kubernetes resource names (uppercase, special characters, too long) or that collide with existing resources in the namespace.

**Prevention:**
1. Validate names: must match regex `^[a-z][a-z0-9-]{0,61}[a-z0-9]$`
2. Prefix all resources with `picoclaw-` to avoid collisions with other services in the namespace
3. Check for existing resources before creating: `kubectl get deployment picoclaw-<name>` should return 404
4. The CLI should sanitize names (lowercase, replace underscores with hyphens) and confirm with the user

**Phase impact:** Phase 2 (CLI deploy command).

---

### Pitfall 12: PicoClaw Session State Confusion

**What goes wrong:** PicoClaw uses session keys (`sessionKey` parameter) to maintain conversation history. If the gRPC sidecar uses a fixed session key for all connections, or generates random ones per stream, the user experience breaks: either all users share one conversation, or reconnecting starts a fresh conversation.

**Prevention:**
1. The session key should be deterministic per instance + user combination
2. For single-user instances (which this is): use the instance name as the session key
3. Store session key mapping so reconnecting to the same instance resumes the same conversation
4. Expose session management in the CLI: `ember-claw chat <instance> --session <name>` to allow multiple conversation threads

**Phase impact:** Phase 1 (gRPC service design) and Phase 2 (CLI chat command).

---

### Pitfall 13: Resource Limits That Starve PicoClaw

**What goes wrong:** Setting CPU/memory limits too low. PicoClaw advertises <10MB RAM, but that is the core binary. In practice, it loads AI provider SDKs, maintains conversation history in memory, and may run MCP tools. With the gRPC server overhead added, actual memory usage may be 50-100MB.

**Prevention:**
1. Profile actual memory usage in a container with a realistic workload before setting defaults
2. Default resource requests: 50m CPU, 64Mi memory
3. Default resource limits: 200m CPU, 256Mi memory
4. Make limits configurable per instance via CLI flags
5. Monitor actual usage after deployment: `kubectl top pod`

**Phase impact:** Phase 1 (manifest defaults).

---

### Pitfall 14: Ignoring PicoClaw's Health Check Endpoint

**What goes wrong:** The gRPC sidecar implements its own health check or uses a simple TCP probe, ignoring that PicoClaw already has a health server on port 18790 with `/health` and `/ready` endpoints. You end up with a pod that Kubernetes considers "healthy" even though PicoClaw's agent loop is not initialized.

**Prevention:**
1. If embedding PicoClaw as a library: start the PicoClaw health server alongside the gRPC server and use `/ready` as the Kubernetes readiness probe
2. Or: implement gRPC health checking protocol (`grpc.health.v1.Health`) that delegates to PicoClaw's internal health state
3. Do not consider the pod ready until `ProcessDirect` would succeed

**Phase impact:** Phase 1 (health check design).

---

### Pitfall 15: Logs Command Without Container Awareness

**What goes wrong:** `ember-claw logs <instance>` simply wraps `kubectl logs`, but does not handle: multiple containers in a pod (if sidecar pattern is used), previous container logs after a restart, following/streaming logs, and log output interleaving between PicoClaw and gRPC server output.

**Prevention:**
1. If single container (recommended): `kubectl logs` wrapper is fine, but support `--follow` and `--previous` flags
2. Parse and colorize log output (PicoClaw uses zerolog JSON format in gateway mode)
3. Add `--since` flag for time-bounded log retrieval
4. If the pod has restarted, automatically show previous logs with a separator

**Phase impact:** Phase 2 (CLI logs command).

## Phase-Specific Warnings

| Phase Topic | Likely Pitfall | Mitigation |
|-------------|---------------|------------|
| Architecture design | Pitfall 1 (wrong abstraction), Pitfall 2 (unnecessary sidecar) | Inspect PicoClaw API before designing. Single binary, not two containers. |
| Manifest creation | Pitfall 4 (PVC lifecycle), Pitfall 5 (secrets), Pitfall 13 (resource limits) | PVC naming convention. Kubernetes Secrets for API keys. Profile before setting limits. |
| gRPC server | Pitfall 7 (stream lifecycle), Pitfall 12 (session keys), Pitfall 14 (health checks) | Context-based cleanup. Deterministic session keys. Delegate to PicoClaw health. |
| CLI tool | Pitfall 3 (port-forward fragility), Pitfall 8 (Make prompts), Pitfall 11 (naming) | Programmatic port-forward via client-go. CLI as primary interface, Make as wrapper. Validate names. |
| Pre-development | Pitfall 9 (Rancher RBAC) | Test all K8s operations with actual kubeconfig before writing any code. |
| Docker build | Pitfall 10 (PicoClaw dependency) | Confirm public module access. Use go.work for local dev, pin version in go.mod for Docker. |
| Instance management | Pitfall 4 (PVC orphans), Pitfall 15 (logs) | Explicit --purge flag. Structured log output. |

## Key Insight: The Biggest Risk Is Misunderstanding PicoClaw

The single most impactful finding from this research: PicoClaw is not a stdin/stdout terminal application to be wrapped with pipes. It is a Go library with a clean programmatic API (`pkg/agent.AgentLoop.ProcessDirect`), a gateway server mode, a health check server, and a plugin architecture. The entire ember-claw architecture should be built as a Go program that imports PicoClaw, not one that wraps it as a subprocess. This eliminates half the pitfalls (stdin parsing, TTY allocation, process lifecycle, sidecar coordination) and dramatically simplifies the project.

## Sources

- PicoClaw source code at `/Users/tuomas/Projects/picoclaw/` (direct analysis)
  - `cmd/picoclaw/internal/agent/helpers.go` -- ProcessDirect API
  - `pkg/agent/loop.go` -- AgentLoop struct and public interface
  - `pkg/health/server.go` -- Health check server on port 18790
  - `docker/Dockerfile` -- Container build pattern, onboard command
  - `docker/docker-compose.yml` -- Gateway mode as default entrypoint
  - `go.mod` -- Public module path `github.com/sipeed/picoclaw`
- Kubernetes sidecar container documentation (kubernetes.io/docs/concepts/workloads/pods/sidecar-containers/) -- lifecycle, shutdown ordering, probe requirements
- Existing Coralie umbrella repo patterns at `/Users/tuomas/Projects/Makefile` -- Make target conventions
- PROJECT.md at `/Users/tuomas/Projects/ember-claw/.planning/PROJECT.md` -- requirements and constraints
