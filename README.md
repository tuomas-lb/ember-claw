<p align="center">
  <img src="assets/brand/emberclaw-logo.png" alt="Ember-Claw" width="200">
</p>

# Ember-Claw

Kubernetes deployment toolkit for running multiple [PicoClaw](https://github.com/sipeed/picoclaw) AI assistant instances on a cluster. Includes a gRPC sidecar that imports PicoClaw as a Go library and a CLI tool (`eclaw`) for full instance lifecycle management.

## Architecture

```
                          ┌───────────────────────────┐
                          │     Kubernetes Cluster     │
                          │                            │
┌──────────┐   port-fwd   │  ┌──────────────────────┐  │
│ eclaw CLI │◄────────────►│  │   Pod: picoclaw-NAME │  │
│           │    gRPC      │  │                      │  │
└──────────┘   :50051      │  │  ┌────────────────┐  │  │
                           │  │  │    Sidecar      │  │  │
                           │  │  │ (gRPC server)   │  │  │
                           │  │  │                 │  │  │
                           │  │  │ PicoClaw lib    │  │  │
                           │  │  │ ProcessDirect() │  │  │
                           │  │  └────────────────┘  │  │
                           │  │                      │  │
                           │  │  PVC: /data/.picoclaw│  │
                           │  └──────────────────────┘  │
                           │                            │
                           │  Secret: API keys          │
                           │  ConfigMap: config.json     │
                           └───────────────────────────┘
```

Each instance runs as a single-container pod. The sidecar binary embeds PicoClaw as a Go library (via `AgentLoop.ProcessDirect()`), exposes it over gRPC on port 50051, and serves health checks on port 8080. The CLI communicates with pods through in-process port-forwarding — no Ingress or LoadBalancer needed.

## Quick Start

### Prerequisites

- Go 1.25+
- Docker with buildx
- `kubectl` with access to target cluster
- Kubeconfig for the target cluster
- A Docker registry — configure `IMAGE_REGISTRY` in `.env` or log in with `docker login`

### Registry Setup

The container registry is **not hardcoded** — you must configure it before building/pushing images.

Add to your `.env` file:
```bash
IMAGE_REGISTRY=your.registry.com
```

If `IMAGE_REGISTRY` is not set, eclaw will attempt to auto-detect a registry from your `~/.docker/config.json` (the first non-Docker Hub host you're logged into). For the Makefile targets (`build-picoclaw`, `push-picoclaw`), `IMAGE_REGISTRY` must be set explicitly.

### Build

```bash
# Build the CLI
make build-eclaw

# Build the sidecar Docker image
make build-picoclaw EMBER_VERSION=0.1

# Push to registry
make push-picoclaw EMBER_VERSION=0.1

# Or build + push in one step
make build-push-picoclaw EMBER_VERSION=0.1
```

### Deploy an Instance

**Interactive wizard:**
```bash
make deploy-picoclaw EMBER_VERSION=0.1
```

Prompts for: instance name, AI provider, API key (hidden input), and model name.

**Non-interactive:**
```bash
make deploy-picoclaw \
  NAME=research \
  PROVIDER=gemini \
  API_KEY=AIza... \
  MODEL=gemini-2.5-flash \
  EMBER_VERSION=0.1
```

**Direct CLI:**
```bash
./bin/eclaw deploy research \
  --provider gemini \
  --api-key AIza... \
  --model gemini-2.5-flash
```

### Manage Instances

```bash
# List all instances (shows real container status, restarts, age)
eclaw list

# Show instance details
eclaw status research

# View logs
eclaw logs research
eclaw logs research --follow

# Delete (prompts before removing PVC)
eclaw delete research
```

### Chat with an Instance

**Interactive mode:**
```bash
eclaw chat research
```
Opens a readline prompt. Type messages, get responses. `Ctrl+C` or `Ctrl+D` to exit.

**Single-shot query:**
```bash
eclaw chat research -m "What is the capital of France?"
```

## Configuration

### `.env` File

The `eclaw` CLI auto-loads a `.env` file from the current directory. This is the recommended way to configure API keys and kubeconfig for local development. Existing environment variables are **not** overridden.

```bash
# .env - Container registry (required for build/push/deploy)
IMAGE_REGISTRY=your.registry.com

# API keys per provider
ANTHROPIC_API_KEY=sk-ant-api03-...
OPENAI_API_KEY=sk-...
GEMINI_API_KEY=AIza...
GROQ_API_KEY=gsk_...
DEEPSEEK_API_KEY=sk-...

# Integration credentials
LINEAR_API_KEY=lin_api_...
LINEAR_TEAM_ID=<team-uuid>
SLACK_BOT_TOKEN=xoxb-...

# Kubeconfig (base64-encoded, for CI/automation)
KUBECONFIG_BASE64=<base64-encoded-kubeconfig>
```

When `--api-key` is not passed to `deploy` or `models`, eclaw automatically resolves it from the provider-specific env var (e.g., `GEMINI_API_KEY` for `--provider gemini`).

### Kubeconfig Resolution

The CLI resolves kubeconfig in this order:

1. `--kubeconfig` flag (explicit path)
2. `KUBECONFIG_BASE64` env var (base64-decoded, written to temp file — for CI/automation)
3. `KUBECONFIG` env var (standard path)
4. `~/.kube/config` (default)

### Instance Secrets

Inject environment variables into a running instance's K8s Secret. The pod is automatically restarted to pick up changes.

```bash
# Add a Telegram bot token
eclaw set-secret test-claw-1 TELEGRAM_BOT_TOKEN xoxb-abc123

# Add a Linear API key
eclaw set-secret research LINEAR_API_KEY lin_api_xxx

# Override PicoClaw configuration
eclaw set-secret research PICOCLAW_AGENTS_DEFAULTS_MAX_TOOL_ITERATIONS 100
```

All keys set via `set-secret` are stored in the instance's K8s Secret and injected as environment variables into the pod. PicoClaw reads many settings from env vars using the `PICOCLAW_` prefix.

### PicoClaw Container Configuration

Ember-claw generates a `config.json` for each instance with these container-optimized defaults:

| Setting | Default | Purpose |
|---------|---------|---------|
| `restrict_to_workspace` | `false` | Allow tool execution outside workspace (safe in container) |
| `allow_read_outside_workspace` | `true` | Allow reading files outside workspace |
| `max_tool_iterations` | `50` | Max LLM tool call iterations per message (default PicoClaw is 20) |

These can be overridden per-instance via `set-secret`:
```bash
eclaw set-secret my-instance PICOCLAW_AGENTS_DEFAULTS_MAX_TOOL_ITERATIONS 100
eclaw set-secret my-instance PICOCLAW_AGENTS_DEFAULTS_RESTRICT_TO_WORKSPACE true
```

### Container Runtime Environment

The sidecar Docker image (Alpine 3.23) includes development tools for PicoClaw agents to use:

| Tool | Purpose |
|------|---------|
| `curl`, `wget` | HTTP requests |
| `jq` | JSON processing |
| `bash` | Shell scripting |
| `git` | Version control |
| `python3`, `pip` | Python scripting (pip works without venv) |
| `nodejs`, `npm` | JavaScript/Node.js |
| `go` | Go programming |
| `gcc`, `make` | Build tools |
| `openssh-client` | SSH access |

`PIP_BREAK_SYSTEM_PACKAGES=1` is set in the container so `pip install` works directly without creating a virtual environment.

## CLI Reference

### Global Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--kubeconfig` | `$KUBECONFIG` or `~/.kube/config` | Path to kubeconfig |
| `--namespace` | `picoclaw` | Kubernetes namespace |

### Commands

#### `eclaw deploy <name>`

Create a named PicoClaw instance on the cluster. Creates the namespace automatically if it doesn't exist. Re-deploying to an existing name updates resources in place (upsert).

| Flag | Default | Description |
|------|---------|-------------|
| `--provider` | *required* | AI provider (`anthropic`, `openai`, `gemini`, `groq`, `deepseek`, `openrouter`, `copilot`) |
| `--api-key` | from env | API key (auto-resolved from `<PROVIDER>_API_KEY` env var if not set) |
| `--model` | *required* | Model identifier |
| `--image` | from `ECLAW_IMAGE` or `IMAGE_REGISTRY` env | Container image |
| `--cpu-request` | `100m` | CPU request |
| `--cpu-limit` | `500m` | CPU limit |
| `--memory-request` | `128Mi` | Memory request |
| `--memory-limit` | `512Mi` | Memory limit |
| `--storage-size` | `1Gi` | PVC size |
| `--storage-class` | cluster default | Storage class |
| `--env` | none | Custom env vars (`key=value`, repeatable) |
| `--linear-api-key` | from env | Linear API key (or `LINEAR_API_KEY` env) |
| `--linear-team-id` | from env | Linear team UUID (or `LINEAR_TEAM_ID` env) |
| `--slack-bot-token` | from env | Slack bot token (or `SLACK_BOT_TOKEN` env) |

Instance names must be valid DNS subdomain components: lowercase alphanumeric and hyphens, 3-63 chars.

#### `eclaw list`

List all managed instances with name, status, ready replicas, restart count, and age. Status reflects actual container state (e.g., `CrashLoopBackOff`, `ImagePullBackOff`) rather than just pod phase.

#### `eclaw status <name>`

Show detailed instance health, uptime, provider, and model.

#### `eclaw delete <name>`

Delete an instance. Removes Deployment, Service, Secret, ConfigMap, and PVC. Prompts before PVC deletion.

#### `eclaw logs <name>`

Stream pod logs. Supports `--lines N` (default 100) and `--follow`.

#### `eclaw models`

List available models from a provider and validate your API key.

| Flag | Default | Description |
|------|---------|-------------|
| `--provider` | *required* | AI provider |
| `--api-key` | from env | API key (auto-resolved from `<PROVIDER>_API_KEY` env var) |

```bash
eclaw models --provider gemini --api-key AIza...
eclaw models --provider openai  # uses OPENAI_API_KEY from .env
```

#### `eclaw set-secret <instance> <key> <value>`

Add or update an environment variable in an instance's K8s Secret. The pod is automatically restarted to pick up the change.

```bash
eclaw set-secret test-claw-1 TELEGRAM_BOT_TOKEN abc123
eclaw set-secret research LINEAR_API_KEY lin_api_xxx
eclaw set-secret my-agent SLACK_BOT_TOKEN xoxb-xxx
```

#### `eclaw chat <name>`

Chat with a running instance via gRPC. Without `-m`, opens an interactive readline session. With `-m "message"`, sends a single query and exits.

## Integration Tools

PicoClaw instances can be deployed with built-in integrations for Linear and Slack. These tools are automatically registered when the corresponding API keys are present.

### Linear

Provides issue management: create, search, get, and update issues.

```bash
eclaw deploy my-agent --provider gemini --model gemini-2.5-flash \
  --linear-api-key lin_api_xxx --linear-team-id <uuid>
```

Or set `LINEAR_API_KEY` and `LINEAR_TEAM_ID` in `.env`.

### Slack

Provides message sending and channel listing.

```bash
eclaw deploy my-agent --provider gemini --model gemini-2.5-flash \
  --slack-bot-token xoxb-xxx
```

Or set `SLACK_BOT_TOKEN` in `.env`.

### Adding Custom Integrations

See [docs/tool-development.md](docs/tool-development.md) for how to add new tools.

## Make Targets

| Target | Description |
|--------|-------------|
| `make help` | Show all targets with usage |
| `make build-eclaw` | Build CLI to `./bin/eclaw` |
| `make build-picoclaw` | Build sidecar Docker image |
| `make push-picoclaw` | Push image to `IMAGE_REGISTRY` |
| `make build-push-picoclaw` | Build and push in one step |
| `make deploy-picoclaw` | Interactive deployment wizard |

Set `EMBER_VERSION=x.y` for versioned builds (auto-increments build number). Without it, tags as `production`.

## Project Structure

```
ember-claw/
├── cmd/
│   ├── eclaw/              # CLI entry point
│   └── sidecar/            # gRPC sidecar entry point
├── internal/
│   ├── cli/                # Cobra subcommands
│   ├── envfile/            # .env file loader
│   ├── grpcclient/         # gRPC client for CLI -> sidecar
│   ├── k8s/                # K8s client, resources, port-forwarding
│   ├── providers/          # Provider model listing
│   ├── server/             # gRPC service, session management
│   └── tools/              # PicoClaw tool integrations
│       ├── linear/         #   Linear issue management
│       └── slack/          #   Slack messaging
├── proto/emberclaw/v1/     # Protobuf service definition
├── gen/emberclaw/v1/       # Generated gRPC code
├── assets/brand/           # Logo and branding
├── docs/                   # Documentation
├── Dockerfile              # Multi-stage build (golang:1.25-alpine -> alpine:3.23)
├── Makefile                # Build, push, deploy orchestration
├── .env                    # Local configuration (git-ignored)
└── .ember-build-numbers    # Per-service build counter
```

## gRPC API

Defined in `proto/emberclaw/v1/service.proto`:

| RPC | Type | Description |
|-----|------|-------------|
| `Chat` | Bidirectional streaming | Interactive chat sessions |
| `Query` | Unary | Single-shot question/answer |
| `Status` | Unary | Instance health and config info |

## Kubernetes Resources

Each deployed instance creates:

| Resource | Name Pattern | Purpose |
|----------|-------------|---------|
| Deployment | `picoclaw-<name>` | Sidecar pod running PicoClaw |
| Service | `picoclaw-<name>` | Cluster-internal gRPC endpoint |
| Secret | `picoclaw-<name>-config` | API keys + config.json + env vars |
| ConfigMap | `picoclaw-<name>-env` | Custom environment variables |
| PVC | `picoclaw-<name>-data` | Persistent storage for sessions/workspace |

All resources are labeled with `app.kubernetes.io/managed-by: ember-claw` and `app.kubernetes.io/instance: <name>` for discovery. The namespace is auto-created if it doesn't exist.

## Development

```bash
# Run all tests
go test ./... -race -count=1

# Build check
go build ./... && go vet ./...

# Regenerate protobuf (requires protoc, protoc-gen-go, protoc-gen-go-grpc)
protoc --go_out=. --go-grpc_out=. proto/emberclaw/v1/service.proto
```

## Documentation

- [Deployment Guide](docs/deployment-guide.md) — step-by-step deploy and troubleshooting
- [Architecture](docs/architecture.md) — design decisions and data flow
- [Tool Development](docs/tool-development.md) — adding new PicoClaw integrations
