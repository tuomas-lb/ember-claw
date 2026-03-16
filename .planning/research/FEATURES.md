# Feature Landscape

**Domain:** Kubernetes deployment toolkit with gRPC sidecar and Go CLI for managing PicoClaw AI assistant instances
**Researched:** 2026-03-13
**Confidence:** HIGH (mature domain patterns, direct codebase inspection of PicoClaw)

## Table Stakes

Features users expect. Missing = product feels incomplete.

### Instance Lifecycle Management

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Deploy named instance | Core value proposition -- "give me a PicoClaw named X" | Medium | Generates Deployment + PVC + Secret + ConfigMap, applies via client-go |
| List instances | Cannot manage what you cannot see | Low | Query deployments by label selector |
| Delete instance | Cleanup is non-negotiable. Orphaned resources waste cluster resources. | Low | Delete Deployment + Service + PVC + Secret + ConfigMap. Prompt before PVC deletion. |
| Instance status | Users need to know if instance is healthy, crashlooping, or pending | Low | Map pod phase + container status to human-readable status |
| Instance logs | First thing users check when something goes wrong | Low | Stream pod logs via client-go. Support --follow. |

### Chat Interface

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Interactive chat mode | Core differentiating use case -- talk to a running PicoClaw instance from the terminal | Medium | gRPC bidirectional stream from CLI to sidecar. Sidecar delegates to AgentLoop.ProcessDirect(). Terminal UX: prompt, streaming response, Ctrl+C handling. |
| Single-shot query mode | Quick question without entering a REPL. Essential for scripting. | Low | gRPC unary call. Send message, receive response, exit. |

### Deployment Configuration

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| AI provider configuration per instance | Each instance may use different models/providers. PicoClaw supports 15+ providers. | Medium | Generate config.json into ConfigMap. API keys into Secret. Mount both into pod. |
| Resource limits per instance | Different workloads need different CPU/memory. K8s best practice requires limits. | Low | Template Deployment with configurable requests/limits. PicoClaw is lightweight so defaults can be small. |
| Persistent storage per instance | PicoClaw has memory/planning features that benefit from persistence | Low | PVC per instance. Sessions and workspace survive pod restarts. |

### Build and Image Management

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Docker image build | Must produce deployable container images | Medium | Multi-stage Dockerfile: Go builder compiles sidecar (embeds PicoClaw), Alpine runtime. |
| Image push to registry | Images must reach reg.r.lastbot.com | Low | docker push with versioned tags. Follow .coralie-build-numbers pattern. |
| Make targets for build/push/deploy | Matches existing umbrella repo conventions | Low | Standard targets: build-sidecar, push-sidecar, build-push-sidecar |

## Differentiators

Features that set the product apart. Not expected, but add value.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Interactive deployment wizard (Make) | Guided setup reduces errors. User picks name, model, resources interactively. | Medium | Shell prompts in Makefile with defaults. Also support non-interactive via variables. |
| Follow mode for logs | `eclaw logs --follow` streams logs in real-time | Low | client-go log follow |
| Session key selection | Chat with different session contexts on same instance | Low | Pass --session flag through gRPC |
| Custom environment variables | Escape hatch for any configuration not covered by explicit flags | Low | CLI flag to K8s env mapping |
| Config hot-reload | Change model/provider without redeploying | Medium | PicoClaw supports config file watching. Update ConfigMap, PicoClaw detects change. |
| Shell completion | Professional CLI feel. Cobra provides this nearly for free. | Low | Cobra built-in completion command |
| Output format options | Enable scripting. `--output json` piped to jq. | Low | Table (default), JSON, YAML output modes |
| Instance restart | Restart without redeploy | Low | K8s rollout restart |

## Anti-Features

Features to explicitly NOT build.

| Anti-Feature | Why Avoid | What to Do Instead |
|--------------|-----------|-------------------|
| Web UI / dashboard | Out of scope per PROJECT.md. CLI-only tool. | CLI provides all management. Use --output json for tooling. |
| Multi-cluster support | Out of scope. Emberchat cluster only. | Default to emberchat kubeconfig. |
| Auto-scaling / HPA | Out of scope. Manual instance management only. | Manual deploy and delete. |
| Helm charts | PROJECT.md specifies Make, not Helm. Matches parent patterns. | Raw K8s manifests via client-go or kubectl. Go templates for rendering. |
| Auth on gRPC | Internal cluster use only. Port-forward provides auth via kubeconfig. | Trust cluster network. |
| Operator pattern (CRD + controller) | Massive complexity for a handful of instances. | Imperative CLI commands. Standard K8s resources. |
| PicoClaw modification | Upstream dependency. "All extensions live in ember-claw." | Import as library, configure via config.json injection. |
| Instance-to-instance communication | Multi-agent orchestration is scope creep. | Each instance is independent. |
| GPU resource management | PicoClaw calls APIs, does not run model inference. | CPU/memory only. |
| Gateway mode channels | PicoClaw's gateway runs Telegram, Discord, etc. channels. Not needed for gRPC access. | Use AgentLoop.ProcessDirect() API directly. |

## Feature Dependencies

```
Proto definitions --> Sidecar gRPC server --> CLI gRPC client
                                                 |
                                                 +--> chat command
                                                 +--> query command

client-go setup --> deploy command --> delete command
                --> list command
                --> status command
                --> logs command
                --> port-forward (for chat/query)

Sidecar binary --> Dockerfile --> deploy command (needs image)
PicoClaw library import --> Sidecar binary

Manifest templates --> deploy command
Secret generation --> deploy command
```

## MVP Recommendation

Prioritize (in order):

1. **Proto definitions + sidecar with AgentLoop integration** -- without this, nothing else works
2. **Dockerfile + Make build targets** -- need a container image to deploy
3. **Deploy command** (create K8s resources for a named instance) -- needed to get a running instance
4. **List command** -- essential for managing multiple instances
5. **Delete command** -- essential cleanup
6. **Chat command** (interactive gRPC streaming) -- primary interaction, validates entire stack
7. **Query command** (single-shot) -- trivial once chat works
8. **Status command** -- debugging
9. **Logs command** -- debugging

Defer:
- Config hot-reload: Manual delete+redeploy works for now
- Shell completion: Low priority
- Output format options: Table is fine for MVP
- Instance profiles/presets: Syntactic sugar over explicit flags
- Custom env vars: Can be added to deploy command later
- Session key selection: Default session is fine for MVP

## Sources

- PROJECT.md requirements analysis (direct)
- PicoClaw API analysis: AgentLoop.ProcessDirect() for programmatic access
- PicoClaw gateway mode: channels-based, not needed for gRPC bridging
- Parent repo Makefile: Make-based deployment patterns
