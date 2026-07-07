# Architecture

## Overview

Ember-Claw is a deployment toolkit with three components:

1. **Sidecar binary** — gRPC server that imports PicoClaw as a Go library
2. **CLI tool (`eclaw`)** — manages instance lifecycle and provides chat interface
3. **Build pipeline** — Dockerfile + Makefile for building, pushing, and deploying

## Design Decisions

### PicoClaw as Library, Not Subprocess

PicoClaw's interactive CLI mode uses `readline` with terminal escape codes, making stdin/stdout parsing fragile. Instead, the sidecar imports PicoClaw's `pkg/agent` package and calls `AgentLoop.ProcessDirect(ctx, message, sessionKey)`, which returns a clean string response. This is the same API PicoClaw's own gateway mode uses internally.

### Single Container, Not Two-Container Sidecar

Since the gRPC server embeds PicoClaw directly via Go library import, there is no second process to run. Each pod contains a single container running the sidecar binary.

### Port-Forward, Not Ingress

The CLI communicates with pods via in-process port-forwarding (using `client-go` SPDY transport). This avoids Ingress setup, TLS certificates, and external exposure for a dev/test fleet. Authentication is handled by the kubeconfig.

The port-forward implementation handles Rancher proxy URLs (which contain encoded path separators) by properly constructing the SPDY transport URL.

### Imperative CLI, Not Operator

Managing a handful of AI instances does not justify a CRD + controller pattern. Standard Kubernetes resources (Deployment, Service, PVC, Secret, ConfigMap) are created and deleted imperatively through the CLI.

### Upsert Semantics

The `deploy` command uses upsert (create-or-update) for all resources. Redeploying to an existing instance name updates it in place without requiring a delete first.

### Auto-Namespace Creation

The `picoclaw` namespace is created automatically on first deploy if it doesn't exist, eliminating a manual setup step.

## Data Flow

### Chat Session

```
User input
  -> eclaw CLI (readline)
  -> gRPC ChatRequest (bidirectional stream)
  -> port-forward tunnel (SPDY)
  -> Sidecar gRPC server
  -> AgentLoop.ProcessDirect(ctx, message, sessionKey)
  -> PicoClaw -> LLM Provider API
  -> Response string
  -> gRPC ChatResponse
  -> CLI prints to terminal
```

### Deploy Instance

```
eclaw deploy <name> --provider X --api-key Y --model Z
  -> Validate instance name (DNS-safe)
  -> Ensure namespace exists (auto-create)
  -> Create/update Secret (config.json + API key + integration keys)
  -> Create/update ConfigMap (custom env vars)
  -> Create/update PVC (persistent storage)
  -> Create/update Deployment (sidecar image, mounts, probes, limits)
  -> Create/update Service (ClusterIP, port 50051)
```

### Set Secret

```
eclaw set-secret <name> <key> <value>
  -> Read existing Secret
  -> Add/update key in Secret.Data
  -> Update Secret
  -> Annotate Deployment with restartedAt timestamp
  -> Kubernetes triggers rolling restart
  -> New pod picks up updated env vars
```

### Health Checking

Two health systems run in parallel:

1. **HTTP health server** (port 8080) — serves `/health` (liveness) and `/ready` (readiness) endpoints, used by Kubernetes probes
2. **gRPC health service** — standard `grpc.health.v1.Health` protocol for K8s 1.24+ native gRPC probes

### Instance Status Resolution

The `list` command resolves status from multiple K8s sources (most specific wins):

1. **Container state** (highest priority): `CrashLoopBackOff`, `ImagePullBackOff`, `ErrImagePull`, `Terminated:OOMKilled`
2. **Pod phase**: `Running`, `Pending`, `Failed`, `Succeeded`
3. **Deployment ready replicas**: fallback for clusters where pod status is delayed

Restart count is aggregated from all container statuses in the pod.

## Configuration Chain

### CLI Configuration

```
.env file (auto-loaded, does not override existing env)
  -> Environment variables (KUBECONFIG, KUBECONFIG_BASE64, *_API_KEY)
  -> CLI flags (--kubeconfig, --api-key, --provider, etc.)
```

API key resolution for `deploy` and `models`:
1. `--api-key` flag (explicit)
2. `<PROVIDER>_API_KEY` env var (e.g., `GEMINI_API_KEY`, `OPENAI_API_KEY`, `BYTEPLUS_API_KEY`)

### Provider → model_list mapping

`buildPicoClawConfig` turns `--provider`/`--model` into a PicoClaw `model_list` entry whose `model` field is `"<protocol>/<model-id>"`. The protocol prefix selects PicoClaw's provider adapter (`CreateProviderFromConfig`); each provider also gets a default `api_base`.

`providerProtocol(provider, apiBase)` maps the eclaw provider name to a PicoClaw protocol. Most names are protocols as-is, but several must be remapped because PicoClaw would reject the raw name (`default: unknown protocol`) and crash-loop the pod:

| `--provider` | protocol | default `api_base` |
|--------------|----------|--------------------|
| `byteplus` | `volcengine` | `https://ark.ap-southeast.bytepluses.com/api/v3` |
| `kimi` | `moonshot` | `https://api.moonshot.cn/v1` |
| `xai` | `openai` | `https://api.x.ai/v1` |
| `google` | `gemini` | (Gemini default) |

`--api-base` (or `ECLAW_API_BASE`) overrides the default for any provider — region/plan-specific endpoints (e.g. BytePlus's Coding Plan `/api/coding/v3`) or self-hosted gateways. An **unrecognized** provider name combined with an explicit `--api-base` is treated as a generic OpenAI-compatible endpoint (protocol `openai`). `DeployInstance` calls `validateProvider` first and returns an error — before creating any cluster resources — if the provider resolves to a protocol PicoClaw doesn't accept and no `--api-base` was given, so a misconfiguration surfaces as a CLI error rather than a CrashLoopBackOff.

### PicoClaw Configuration in Container

```
config.json (generated by ember-claw, stored in K8s Secret, mounted at /config/)
  -> Environment variables from Secret (envFrom)
  -> PicoClaw env var overrides (PICOCLAW_* prefix)
```

Generated `config.json` includes container-optimized defaults:
- `restrict_to_workspace: false` — safe in container isolation
- `allow_read_outside_workspace: true` — agents need filesystem access
- `max_tool_iterations: 200` — well above PicoClaw's default of 20 (coding/fleet bots with playwright + many MCP tools otherwise hit the "no response to give" fallback)

## Session Management

Each gRPC connection gets a session key (either provided by the client or auto-assigned from the instance name). PicoClaw maintains conversation state per session key, enabling multiple independent conversations on the same instance.

## Container Runtime

The sidecar image is built as a two-stage Docker build:

1. **Builder stage** (`golang:1.25-alpine`): Compiles the sidecar binary with PicoClaw embedded
2. **Runtime stage** (`alpine:3.23`): Minimal base with development tools

The runtime includes: `curl`, `wget`, `jq`, `bash`, `git`, `python3`, `pip`, `nodejs`, `npm`, `go`, `gcc`, `make`, `openssh-client`. These tools are available to PicoClaw agents during conversations.

Key container environment:
- `PIP_BREAK_SYSTEM_PACKAGES=1` — allows pip install without venv
- `GOPATH=/home/picoclaw/go` — Go workspace for the picoclaw user
- Non-root user `picoclaw` (uid 1000) for security
- PVC mounted at `/home/picoclaw/.picoclaw` for persistent storage

## Resource Layout

```
Namespace: picoclaw
|
+-- Deployment: picoclaw-research
|   +-- Pod: picoclaw-research-xxxxx
|       +-- Container: sidecar
|           +-- Port 50051 (gRPC)
|           +-- Port 8080 (health: /health, /ready)
|           +-- Volume: /home/picoclaw/.picoclaw (from PVC)
|           +-- Volume: /config (config.json from Secret)
|           +-- Env: from Secret (API keys, integration tokens)
|           +-- Env: from ConfigMap (custom env vars)
|
+-- Service: picoclaw-research (ClusterIP -> 50051)
+-- Secret: picoclaw-research-config (config.json + API keys + tokens)
+-- ConfigMap: picoclaw-research-env (custom env vars)
+-- PVC: picoclaw-research-data (1Gi default)
|
+-- [--fleet-admin] ServiceAccount/Role/RoleBinding: picoclaw-research-fleet
+-- [--shared-pvc]  PVC: <shared-name> (fleet-wide, mounted at /home/picoclaw/shared)
```

## HTTP Server (port 8080)

The sidecar runs a single HTTP server on 8080 serving K8s probes (`/health`, `/ready`), the web control UI (`/`), and the authenticated control API (`/api/status`, `/api/chat`). `/api/chat` calls `AgentLoop.ProcessDirect` with the same session-key semantics as the gRPC API (`web:<uuid>` prefix, client-provided `session_id` honored). Auth is a bearer token from the `CONTROL_TOKEN` env var, compared in constant time; with no token configured the API returns 503 (fail closed). The server's write timeout is 15 minutes because chat requests block while the agent runs tool loops; `eclaw expose` matches this with 900s nginx proxy-read/send-timeout annotations.

The channel manager's own webhook HTTP server is intentionally not started — it would collide with this server on port 8080. Telegram uses long polling and needs no webhook endpoint; webhook-based channels are not supported in container mode.

## Fleet Dashboard

The fleet dashboard (`dashboard/`, deployed by `eclaw dashboard deploy`) is a separate control-plane pod, one per namespace, that operators and fleet-admin bots share. It is a Go server (chi router + in-cluster Kubernetes client + gRPC client to instances on `:50051`) serving an embedded React/Vite SPA. It lists instances (by the `managed-by=eclaw` labels), deploys/deletes them, streams pod logs, and proxies chat over the instance gRPC `Query` RPC. Chat history is persisted to Postgres (optional, via `DATABASE_URL`) keyed by a stable per-instance browser session, so conversations survive navigation.

It runs under a **namespace-scoped** ServiceAccount/Role (deployments, services, secrets, configmaps, PVCs, pods, pods/log) — never cluster-wide — so it can only see and manage its own namespace, including instance secrets. Access control is **mutual-TLS** at the nginx ingress: `eclaw mtls init` mints a CA (stored as the `auth-tls-secret`) and a `client.p12` for browsers. The dashboard is registry-agnostic: it deploys new instances using the image in its `SIDECAR_IMAGE` env.

### Live chat streaming (processing steps)

The dashboard chat shows the agent's work *as it happens* — reasoning and tool-call intents — instead of only the final answer. PicoClaw v0.2.3 exposes no per-step callback for the gRPC/web path (reasoning is only routed to configured messaging "reasoning channels" via the shared outbound bus), so the sidecar surfaces steps by **wrapping the LLM provider** (`internal/stream`). The wrapper sees every `LLMResponse` the agent receives — including `Reasoning`/`ReasoningContent` and the `ToolCalls` it is about to execute — and emits them as `Step`s through a `Sink` stashed in the request context. Because the agent calls `Provider.Chat` with the same context that flows from `ProcessDirect`, each Sink correlates to exactly one chat turn even under concurrent sessions.

The gRPC `Chat` bidi stream carries steps as `ChatResponse` frames with `done=false` and a JSON `stream.Step` envelope in `text`; the final answer is a single `done=true` frame. No proto change was needed. The dashboard's `HandleChat` opens one `ChatStream` per WebSocket, forwards step frames to the browser as `{"step":{...}}` (not persisted) and the final frame as `{"text":...,"done":true}` (persisted). `ChatPanel` renders steps live and clears them when the answer arrives. Steps are best-effort: a full sink buffer drops them rather than blocking the agent.

## Fleet Control (--fleet-admin)

When deployed with `--fleet-admin`, the instance pod runs under ServiceAccount `picoclaw-<name>-fleet`, bound to a namespace-scoped Role covering deployments, services, secrets, configmaps, PVCs, serviceaccounts, pods (+ `pods/log`, `pods/portforward`), ingresses, and roles/rolebindings — everything the eclaw CLI touches. The `eclaw` binary ships in the container image; client-go falls back to the in-cluster ServiceAccount config when no kubeconfig exists, so `eclaw list/deploy/logs/chat/delete` work directly inside the pod. `ECLAW_NAMESPACE` and `ECLAW_IMAGE` are injected so the namespace and container image resolve without a `.env`.

Kubernetes RBAC forbids privilege escalation, so a fleet-admin instance can grant `--fleet-admin` to instances it deploys (same Role), but can never grant permissions beyond its own.

## Shared Storage (--shared-pvc)

A shared PVC is created on demand (default 10Gi) and mounted at `/home/picoclaw/shared` in every instance deployed with the same `--shared-pvc <name>`. It carries the `managed-by=eclaw` + `component=shared-storage` labels but **no instance label**, so `eclaw delete` never removes it. With `ReadWriteOnce` storage classes (e.g. `local-path`), Kubernetes co-schedules all sharing pods onto the volume's node via volume node affinity.

## Integration Tools

Tools are registered in the sidecar at startup based on environment variables. No env var = tool not registered.

| Integration | Env Vars Required | Tools Provided |
|-------------|------------------|----------------|
| Linear | `LINEAR_API_KEY`, `LINEAR_TEAM_ID` | create, search, get, update issues |
| Slack | `SLACK_BOT_TOKEN` | send messages, list channels |

See [tool-development.md](tool-development.md) for adding new integrations.

## Module Dependencies

```
github.com/tuomas-lb/ember-claw
+-- github.com/sipeed/picoclaw v0.2.3     # PicoClaw agent library
+-- google.golang.org/grpc v1.79.2        # gRPC framework
+-- google.golang.org/protobuf v1.36.11   # Protobuf runtime
+-- k8s.io/client-go v0.35.2              # Kubernetes API client
+-- github.com/spf13/cobra v1.10.2        # CLI framework
+-- github.com/rs/zerolog v1.34.0         # Structured logging
+-- github.com/chzyer/readline v1.5.1     # Interactive terminal
+-- github.com/olekukonko/tablewriter     # Table output
+-- github.com/fatih/color v1.18.0        # Colored output
```
