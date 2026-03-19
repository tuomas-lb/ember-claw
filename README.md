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
- Kubeconfig for the emberchat cluster

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
  PROVIDER=anthropic \
  API_KEY=sk-ant-... \
  MODEL=claude-sonnet-4-20250514 \
  EMBER_VERSION=0.1
```

**Direct CLI:**
```bash
./bin/eclaw deploy research \
  --provider anthropic \
  --api-key sk-ant-... \
  --model claude-sonnet-4-20250514 \
  --kubeconfig /path/to/kubeconfig
```

### Manage Instances

```bash
# List all instances
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

## CLI Reference

### Global Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--kubeconfig` | `$KUBECONFIG` or `~/.kube/config` | Path to kubeconfig |
| `--namespace` | `picoclaw` | Kubernetes namespace |

### Environment Variables

| Variable | Description |
|----------|-------------|
| `KUBECONFIG` | Standard kubeconfig path (used when `--kubeconfig` not set) |
| `KUBECONFIG_BASE64` | Base64-encoded kubeconfig content (for CI/automation) |

### Commands

#### `eclaw deploy <name>`

Create a named PicoClaw instance on the cluster.

| Flag | Default | Description |
|------|---------|-------------|
| `--provider` | *required* | AI provider (`anthropic`, `openai`, `gemini`, etc.) |
| `--api-key` | *required* | API key for the provider |
| `--model` | *required* | Model identifier |
| `--image` | `reg.r.lastbot.com/ember-claw-sidecar:latest` | Container image |
| `--cpu-request` | `100m` | CPU request |
| `--cpu-limit` | `500m` | CPU limit |
| `--memory-request` | `128Mi` | Memory request |
| `--memory-limit` | `512Mi` | Memory limit |
| `--storage-size` | `1Gi` | PVC size |
| `--storage-class` | cluster default | Storage class |
| `--env` | none | Custom env vars (`key=value`, repeatable) |

Instance names must be valid DNS subdomain components: lowercase alphanumeric and hyphens, 3-63 chars.

#### `eclaw list`

List all managed instances with name, status, and age.

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
| `--provider` | *required* | AI provider (`openai`, `gemini`, `anthropic`, `groq`, `deepseek`, `openrouter`) |
| `--api-key` | *required* | API key to validate and list models for |

```bash
eclaw models --provider gemini --api-key AIza...
```

#### `eclaw chat <name>`

Chat with a running instance via gRPC. Without `-m`, opens an interactive readline session. With `-m "message"`, sends a single query and exits.

## Make Targets

| Target | Description |
|--------|-------------|
| `make help` | Show all targets with usage |
| `make build-eclaw` | Build CLI to `./bin/eclaw` |
| `make build-picoclaw` | Build sidecar Docker image |
| `make push-picoclaw` | Push image to `reg.r.lastbot.com` |
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
│   ├── cli/                # Cobra subcommands (deploy, list, delete, status, logs, chat, models)
│   ├── grpcclient/         # gRPC client for CLI → sidecar communication
│   ├── k8s/                # Kubernetes client, resource creation, port-forwarding
│   ├── providers/          # Provider model listing (OpenAI, Gemini, Anthropic, etc.)
│   └── server/             # gRPC service implementation, session management
├── proto/emberclaw/v1/     # Protobuf service definition
├── gen/emberclaw/v1/       # Generated gRPC code
├── Dockerfile              # Multi-stage build (golang:1.25-alpine → alpine:3.23)
├── Makefile                # Build, push, deploy orchestration
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
| Secret | `picoclaw-<name>` | API key storage |
| ConfigMap | `picoclaw-<name>` | PicoClaw config.json |
| PVC | `picoclaw-<name>` | Persistent storage for sessions/workspace |

All resources are labeled with `app.kubernetes.io/managed-by: ember-claw` and `app.kubernetes.io/instance: <name>` for discovery.

## Configuration

The sidecar resolves PicoClaw config via the standard priority chain:

1. `PICOCLAW_CONFIG` env var (full path to config.json)
2. `$PICOCLAW_HOME/config.json`
3. `~/.picoclaw/config.json`

In containers, `PICOCLAW_HOME` is set to `/data/.picoclaw` (the PVC mount path).

## Development

```bash
# Run all tests
go test ./... -race -count=1

# Build check
go build ./... && go vet ./...

# Regenerate protobuf (requires protoc, protoc-gen-go, protoc-gen-go-grpc)
protoc --go_out=. --go-grpc_out=. proto/emberclaw/v1/service.proto
```
