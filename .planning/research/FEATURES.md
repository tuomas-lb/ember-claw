# Feature Landscape

**Domain:** Kubernetes deployment toolkit with gRPC sidecar and Go CLI for managing PicoClaw AI assistant instances
**Researched:** 2026-03-13
**Confidence:** HIGH (mature domain patterns, direct codebase inspection of PicoClaw)

## Table Stakes

Features users expect. Missing = product feels incomplete.

### Instance Lifecycle Management

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Deploy named instance | Core value proposition. Every K8s management tool needs "create a thing with a name." | Medium | Generates Deployment + Service + PVC manifests from templates, applies via client-go or kubectl |
| List instances | Cannot manage what you cannot see. `kubectl get pods` is not ergonomic for this use case. | Low | Query pods/deployments by label selector (e.g. `app=picoclaw, managed-by=ember-claw`) |
| Delete instance | Cleanup is non-negotiable. Orphaned pods waste cluster resources. | Low | Delete Deployment + Service + PVC. Must prompt for PVC deletion since it destroys data. |
| Instance status | Users need to know if their instance is healthy, crashlooping, or pending. | Low | Map pod phase + container status + restart count to human-readable status |
| Instance logs | First thing users check when something goes wrong. | Low | Stream pod logs via client-go or `kubectl logs`. Must support both sidecar and picoclaw containers. |

### Chat Interface

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Interactive chat mode | Core differentiating use case -- talk to a running PicoClaw instance from the terminal. | High | gRPC bidirectional stream from CLI to sidecar. Sidecar bridges to PicoClaw stdin/stdout (or gateway API). Terminal UX matters: prompt, streaming response display, Ctrl+C handling. |
| Single-shot query mode | Quick question without entering a REPL. Essential for scripting and automation. | Medium | gRPC unary or server-stream call. Send message, receive response, exit. Must handle timeout. |

### Deployment Configuration

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| AI provider configuration per instance | Each instance may use different models/providers. PicoClaw supports 15+ providers. | Medium | Inject config.json into pod via ConfigMap or Secret. Must handle API keys securely (Secrets, not ConfigMaps). |
| Resource limits per instance | Different workloads need different CPU/memory. K8s best practice requires limits. | Low | Template Deployment with configurable requests/limits. Sane defaults (PicoClaw is <10MB so defaults can be tiny). |
| Persistent storage per instance | PicoClaw has memory/planning features that benefit from persistence. Losing state on restart = broken experience. | Medium | PVC per instance with appropriate StorageClass. Must survive pod restarts. |
| Custom environment variables | Escape hatch for any configuration not covered by explicit flags. | Low | Merge user env vars into Deployment spec. |

### Build and Image Management

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Docker image build | Must produce deployable container images. | Medium | Multi-stage Dockerfile: build PicoClaw + sidecar, runtime image. Follow existing Makefile patterns from parent umbrella repo. |
| Image push to registry | Images must reach the cluster's registry (`reg.r.lastbot.com`). | Low | `docker push` with versioned tags. Follow `.coralie-build-numbers` pattern. |
| Make targets for build/push/deploy | Matches existing umbrella repo conventions. Users expect `make build-X`, `make deploy-X`. | Low | Standard Makefile targets. Interactive prompts for instance name and configuration. |

### Connectivity

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| kubectl port-forward based access | Simplest path to reach pod gRPC from local CLI. No ingress setup needed. | Low | CLI automatically sets up port-forward when connecting to an instance. Teardown on disconnect. |
| Cluster-internal Service | Instances need stable DNS names within the cluster for inter-service communication. | Low | ClusterIP Service per instance. |

## Differentiators

Features that set the product apart. Not expected, but add significant value.

### Smart Deployment UX

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Interactive deployment wizard (Make) | Guided setup reduces errors. User picks name, model, resources interactively. Much better than editing YAML. | Medium | Shell-based prompts in Makefile. Default values for everything. Validate input before applying. |
| Deploy-time model validation | Catch misconfiguration before it wastes cluster resources. | Low | Verify API key format, check provider name against known list, warn if model name looks wrong. |
| Instance profiles/presets | "Deploy a GPT-5.4 instance" or "Deploy a Claude instance" as a single command with sensible defaults. | Low | Named presets mapping to provider configs. `ember-claw deploy --preset claude-opus` |

### Operational Convenience

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| `status --all` dashboard | See all instances at a glance with health, uptime, model, resource usage. | Medium | Table output showing name, status, model, age, CPU/memory. Like `kubectl get pods` but domain-specific. |
| Config hot-reload | Change AI model or parameters without redeploying. PicoClaw gateway supports config reload. | Medium | Update ConfigMap/Secret, trigger pod restart or use PicoClaw's built-in config reload (if gateway supports it). |
| Log streaming with follow | `ember-claw logs my-instance -f` for real-time monitoring. | Low | Pass `--follow` to pod log stream. |
| Multi-container log selection | Choose between sidecar logs and picoclaw logs. | Low | `--container` flag. Default to picoclaw container. |

### Developer Experience

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Shell completion (bash/zsh/fish) | Professional CLI feel. Cobra provides this nearly for free. | Low | Cobra's built-in `completion` command. Register instance names as completable arguments. |
| Output format options (table/json/yaml) | Enable scripting and automation. JSON output piped to jq is standard practice. | Low | `--output` flag with table (default), json, yaml. |
| Kubeconfig auto-detection | Automatically find and use the emberchat kubeconfig. Reduce setup friction. | Low | Check `KUBECONFIG` env, then `~/.kube/config`, then the known path `/Users/tuomas/Projects/ember.kubeconfig.yaml`. |
| Version/update info | Users should know what version they're running. | Low | `ember-claw version` showing CLI version, sidecar version, connected cluster info. |

### Architecture Insight (from PicoClaw analysis)

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Gateway mode instead of stdin/stdout sidecar | PicoClaw already has a gateway mode (HTTP server on port 18790) with health/ready endpoints. The sidecar could use PicoClaw's gateway API instead of wrapping stdin/stdout. This is dramatically simpler and more robust. | LOW (if using gateway) vs HIGH (if stdin/stdout) | **Critical finding**: PicoClaw's Dockerfile already runs `CMD ["gateway"]`. The sidecar should act as a gRPC-to-HTTP bridge to PicoClaw's gateway, NOT a stdin/stdout wrapper. This changes the architecture significantly. |
| Health probe integration | PicoClaw gateway already exposes `/health` and `/ready`. Wire these directly into K8s liveness/readiness probes. | Low | Kubernetes probes can hit PicoClaw's health server directly. No sidecar involvement needed for health. |

## Anti-Features

Features to explicitly NOT build.

| Anti-Feature | Why Avoid | What to Do Instead |
|--------------|-----------|-------------------|
| Web UI / dashboard | Explicitly out of scope per PROJECT.md. Adds massive complexity for a dev/test tool. | CLI-only. Use `--output json` for any tooling that wants to build on top. |
| Multi-cluster support | Explicitly out of scope. Only emberchat cluster. Adding multi-cluster support adds kubeconfig management, context switching, namespace collision handling. | Hardcode or default to emberchat cluster kubeconfig. |
| Auto-scaling / HPA | Explicitly out of scope. Manual instance management only. Auto-scaling AI instances is a different problem (cost management, queue depth, etc). | Manual `deploy` and `delete` commands. |
| Helm charts | PROJECT.md specifies Make targets, not Helm. Helm adds packaging complexity inappropriate for a single-cluster dev tool. | Raw K8s manifests applied via client-go or kubectl. Templating via Go `text/template`. |
| Authentication/authorization for gRPC | Explicitly out of scope. Internal cluster use only. Adding auth means TLS certs, token management, RBAC -- overkill for a dev fleet. | Trust cluster network. Service mesh can add mTLS later if needed. |
| Operator pattern (CRD + controller) | Massive complexity for managing a handful of instances. Operators are for platform teams managing hundreds of tenants. | Imperative CLI commands. State lives in standard K8s resources (Deployments, Services, PVCs). |
| PicoClaw code modifications | PROJECT.md: "all extensions live in ember-claw." Forking upstream creates maintenance burden. | Wrap PicoClaw via its existing gateway API. Configuration via config.json injection. |
| Instance-to-instance communication | Multi-agent orchestration is a separate project. Adding message routing between PicoClaw instances is scope creep. | Each instance is independent. If needed later, it's a new project. |
| Persistent chat history in CLI | The CLI is a thin client. PicoClaw itself manages session state via its memory/planning features. | Rely on PicoClaw's built-in session management (sessions persist in PVC). |
| GPU resource management | PicoClaw is an API-calling agent, not a model inference server. It doesn't need GPU. | Only CPU/memory resource configuration. |

## Feature Dependencies

```
Docker Image Build --> Image Push --> Deploy Instance
                                          |
                                          +--> Instance Status
                                          +--> Instance Logs
                                          +--> Instance Delete
                                          +--> Chat (Interactive)
                                          +--> Chat (Single-shot)

gRPC Proto Definition --> Sidecar Implementation --> CLI Chat Commands
                                    |
                                    +--> Port-forward Setup

K8s Manifest Templates --> Deploy Instance --> PVC Creation
                                    |
                                    +--> ConfigMap/Secret (AI config)
                                    +--> Service Creation

PicoClaw Gateway Understanding --> Sidecar Design Decision
  (gateway mode vs stdin/stdout)     (HTTP bridge vs process wrapper)
```

### Critical Path

1. **gRPC proto definition** -- everything else in the chat pipeline depends on this
2. **K8s manifest templates** -- everything in the deployment pipeline depends on this
3. **Sidecar implementation** -- chat features are blocked until this works
4. **CLI framework (Cobra)** -- all user-facing commands depend on this

### Parallel Tracks

After the critical path items, these can proceed independently:
- **Track A (Deploy)**: Make targets, interactive wizard, profiles/presets
- **Track B (Chat)**: Interactive mode, single-shot mode, streaming display
- **Track C (Ops)**: Status dashboard, log streaming, shell completion

## MVP Recommendation

### Phase 1: Deploy and Observe (must ship first)

Prioritize:
1. **Docker image** with PicoClaw + gRPC sidecar (both running in one pod)
2. **K8s manifest templates** (Deployment + Service + PVC)
3. **`ember-claw deploy <name>`** -- create a named instance with defaults
4. **`ember-claw list`** -- show all instances
5. **`ember-claw status <name>`** -- show instance health
6. **`ember-claw logs <name>`** -- stream logs
7. **`ember-claw delete <name>`** -- teardown instance
8. **Make targets** for build/push matching umbrella repo pattern

Rationale: You need to deploy and observe before you can chat. This validates the K8s integration, image building, and basic lifecycle management.

### Phase 2: Chat Interface (core value)

Prioritize:
1. **gRPC service definition** (proto file)
2. **Sidecar gRPC-to-PicoClaw bridge** (use gateway HTTP API, not stdin/stdout)
3. **`ember-claw chat <name>`** -- interactive terminal chat via gRPC stream
4. **`ember-claw chat <name> -m "message"`** -- single-shot query
5. **Port-forward automation** in CLI

Rationale: This is the core differentiating value. After Phase 1 proves deployment works, Phase 2 delivers the "chat with your instance" experience.

### Phase 3: Polish and Convenience

Defer:
- **Instance profiles/presets**: Nice but not blocking adoption. `--preset` is syntactic sugar over flags.
- **Config hot-reload**: Can always just delete and redeploy for now.
- **Shell completion**: Low effort but low priority.
- **Output format options**: `--output json` is useful for scripting but not day-one critical.
- **Status dashboard (`--all`)**: `list` command covers the basic case.
- **Deploy-time validation**: Users can inspect logs if config is wrong.

## Key Insight: Gateway Mode Changes Everything

**The most important research finding**: PicoClaw already runs as a gateway server (HTTP on port 18790) with health and ready endpoints. The Dockerfile's default CMD is `["gateway"]`. This means:

1. **The sidecar should be a gRPC-to-HTTP bridge**, not a stdin/stdout process wrapper. This is dramatically simpler to implement and more reliable.
2. **Health probes come for free** -- K8s can probe PicoClaw's `/health` and `/ready` endpoints directly.
3. **The sidecar complexity drops significantly** -- it just needs to translate gRPC streaming calls to HTTP requests to PicoClaw's gateway API.
4. **PicoClaw's agent `-m` flag** provides a simpler alternative for single-shot queries, but gateway mode is better for long-running instances because it manages sessions, channels, and state internally.

The decision between gateway-mode bridging vs stdin/stdout wrapping should be settled in Phase 1 research, but gateway mode is strongly recommended based on PicoClaw's existing architecture.

## Sources

- Direct codebase inspection of PicoClaw at `/Users/tuomas/Projects/picoclaw/`
  - `cmd/picoclaw/main.go` -- Cobra-based CLI with agent, gateway, onboard commands
  - `cmd/picoclaw/internal/agent/command.go` -- Interactive and single-shot agent modes
  - `cmd/picoclaw/internal/gateway/command.go` -- Gateway server mode
  - `pkg/health/server.go` -- HTTP health/ready server on port 18790
  - `config/config.example.json` -- Full configuration schema with 15+ AI providers
  - `docker/Dockerfile` -- Multi-stage build, runs as `gateway` by default
- Umbrella repo Makefile at `/Users/tuomas/Projects/Makefile` -- Build/push/deploy patterns
- Project definition at `/Users/tuomas/Projects/ember-claw/.planning/PROJECT.md`
- Training data: Kubernetes deployment patterns (kubectl, client-go, Cobra CLI patterns, gRPC service design, sidecar patterns) -- HIGH confidence, these are mature and well-established domains
