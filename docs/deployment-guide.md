# Deployment Guide

Step-by-step guide for deploying PicoClaw instances on the emberchat Kubernetes cluster.

## Prerequisites

1. **Go 1.25+** installed
2. **Docker** with buildx plugin
3. **kubectl** configured
4. **Registry access** — set `IMAGE_REGISTRY=your.registry.com` in `.env` and run `docker login your.registry.com`
5. **Kubeconfig** — cluster credentials (see [Kubeconfig Setup](#kubeconfig-setup) below)

## Kubeconfig Setup

The CLI resolves kubeconfig in this order:

1. `--kubeconfig` flag (explicit path)
2. `KUBECONFIG_BASE64` env var (base64-decoded — for CI/automation)
3. `KUBECONFIG` env var (standard path)
4. `~/.kube/config` (default)

**For local development**, set it in `.env`:
```bash
# Option A: Path to kubeconfig file
KUBECONFIG=/path/to/your/kubeconfig.yaml

# Option B: Base64-encoded kubeconfig (for CI/automation)
KUBECONFIG_BASE64=<base64-encoded-content>
```

**To get kubeconfig from Rancher:**
1. Open your Rancher URL and log in
2. Navigate to the target cluster
3. Click **Kubeconfig File** (top-right of cluster dashboard)
4. Save the content to a file or base64-encode it

**Verify access:**
```bash
kubectl --kubeconfig /path/to/kubeconfig get namespaces
```

## First-Time Setup

### 1. Configure `.env`

Create a `.env` file in the project root with your API keys:

```bash
# AI provider API keys (eclaw auto-resolves per provider)
GEMINI_API_KEY=AIza...
ANTHROPIC_API_KEY=sk-ant-api03-...
OPENAI_API_KEY=sk-...

# Optional: integration credentials
LINEAR_API_KEY=lin_api_...
LINEAR_TEAM_ID=<team-uuid>
SLACK_BOT_TOKEN=xoxb-...

# Optional: CalDAV calendar (single account; use --caldav flag for multiple)
CALDAV_URL=https://caldav.example.com/user/
CALDAV_USERNAME=user
CALDAV_PASSWORD=secret

# Optional: kubeconfig for CI
# KUBECONFIG_BASE64=<base64-encoded>
```

The `.env` file is auto-loaded by `eclaw`. Existing environment variables are **not** overridden.

### 2. Build the CLI

```bash
make build-eclaw
```

This produces `./bin/eclaw`.

### 3. Validate API Keys (Optional)

```bash
# List available models (validates the API key)
./bin/eclaw models --provider gemini
./bin/eclaw models --provider openai
./bin/eclaw models --provider anthropic --api-key sk-ant-...
```

### 4. Build and Push the Sidecar Image

```bash
make build-push-picoclaw EMBER_VERSION=0.1
```

This:
- Runs a multi-stage Docker build (Go compilation + Alpine runtime with dev tools)
- Auto-increments the build number in `.ember-build-numbers`
- Tags as `<registry>/ember-claw-sidecar:0.1.<build_number>`
- Pushes to the registry

### 5. Verify Cluster Access

```bash
./bin/eclaw list
```

The `picoclaw` namespace is auto-created on first deploy if it doesn't exist.

## Deploying an Instance

### Interactive (Recommended)

```bash
make deploy-picoclaw EMBER_VERSION=0.1
```

The wizard prompts for:
- **Instance name** — lowercase, alphanumeric + hyphens (e.g., `research`, `test-bot`)
- **AI provider** — `anthropic`, `openai`, `gemini`, `groq`, `deepseek`, `openrouter`, `copilot`
- **API key** — entered silently (not echoed). If `.env` has the provider key, it's used automatically.
- **Model name** — e.g., `gemini-2.5-flash`, `claude-sonnet-4-20250514`, `gpt-4o`

Resource defaults (100m CPU, 128Mi memory, 1Gi storage) can be overridden with Make variables:
```bash
make deploy-picoclaw EMBER_VERSION=0.1 CPU_LIM=1000m MEM_LIM=1Gi STORAGE=5Gi
```

### Non-Interactive

```bash
make deploy-picoclaw \
  NAME=research \
  PROVIDER=gemini \
  API_KEY=AIza... \
  MODEL=gemini-2.5-flash \
  EMBER_VERSION=0.1
```

### Direct CLI

```bash
./bin/eclaw deploy research \
  --provider gemini \
  --model gemini-2.5-flash
```

When `.env` contains `GEMINI_API_KEY`, the `--api-key` flag is optional.

### Re-deploying

Deploying to an existing instance name updates resources in place (upsert). No need to delete first.

```bash
./bin/eclaw deploy research --provider openai --model gpt-4o --api-key sk-...
```

## Verifying Deployment

```bash
# List all instances (shows container-level status)
./bin/eclaw list

# Check specific instance
./bin/eclaw status research

# View logs
./bin/eclaw logs research --follow
```

The `list` command shows real container status:

```
  NAME       STATUS    READY  RESTARTS  AGE
  research   Running   1/1    0         2h
  test-bot   CrashLoop 0/1    5         10m
```

Wait for `Running` status before chatting.

## Chatting

### Interactive Session

```bash
./bin/eclaw chat research
```

Output:
```
Connecting to research...
Connected. Type messages or Ctrl+C to exit.
[research]> Hello, what can you help me with?
I'm a PicoClaw AI assistant. I can help with...
[research]>
```

### Single-Shot Query

```bash
./bin/eclaw chat research -m "Summarize the last meeting notes"
```

## Managing Instance Secrets

Inject environment variables into a running instance without redeploying. The pod restarts automatically.

```bash
# Add a Telegram bot token
eclaw set-secret test-claw-1 TELEGRAM_BOT_TOKEN abc123

# Add integration keys
eclaw set-secret research LINEAR_API_KEY lin_api_xxx
eclaw set-secret my-agent SLACK_BOT_TOKEN xoxb-xxx

# Tune PicoClaw behavior
eclaw set-secret research PICOCLAW_AGENTS_DEFAULTS_MAX_TOOL_ITERATIONS 100
```

### Common PicoClaw Settings

These can be set via `set-secret` using the `PICOCLAW_` env var prefix:

| Env Var | Default | Description |
|---------|---------|-------------|
| `PICOCLAW_AGENTS_DEFAULTS_MAX_TOOL_ITERATIONS` | `50` | Max tool call iterations per message |
| `PICOCLAW_AGENTS_DEFAULTS_RESTRICT_TO_WORKSPACE` | `false` | Restrict file/exec operations to workspace dir |
| `PICOCLAW_AGENTS_DEFAULTS_ALLOW_READ_OUTSIDE_WORKSPACE` | `true` | Allow reading files outside workspace |
| `PICOCLAW_TOOLS_EXEC_ENABLE_DENY_PATTERNS` | `true` | Block dangerous shell commands |

## Configuring Gmail

Add IMAP mailboxes to a running instance using `eclaw set-gmail`. Requires a [Gmail App Password](https://myaccount.google.com/apppasswords).

```bash
# Add a mailbox
eclaw set-gmail my-agent add work \
  --host imap.gmail.com --port 993 \
  --user you@gmail.com --password "xxxx-xxxx-xxxx-xxxx"

# Add another mailbox
eclaw set-gmail my-agent add personal \
  --host imap.gmail.com --port 993 \
  --user personal@gmail.com --password "xxxx-xxxx-xxxx-xxxx"

# Remove a mailbox
eclaw set-gmail my-agent remove work
```

The Gmail MCP server provides 6 tools: `list_mailboxes`, `list_folders`, `search`, `read`, `list_recent`, `count_unread`. All are read-only — the server cannot send or delete emails.

## Managing Instances

### List All

```bash
eclaw list
```

Shows NAME, STATUS (from actual container state), READY (replicas), RESTARTS, and AGE.

### Delete

```bash
eclaw delete research
```

Removes Deployment, Service, Secret, and ConfigMap. Prompts before PVC deletion (data loss).

### View Logs

```bash
eclaw logs research --follow    # Stream logs in real-time
eclaw logs research --lines 50  # Last 50 lines
```

## Updating an Instance

### Change AI Provider/Model

Re-deploy with new config (upserts in place):
```bash
eclaw deploy research --provider openai --api-key sk-... --model gpt-4o
```

### Add/Update Secrets

Use `set-secret` for runtime configuration changes:
```bash
eclaw set-secret research TELEGRAM_BOT_TOKEN new-token-value
```

### Rebuild and Push New Image

After code changes to the sidecar or Dockerfile:
```bash
make build-push-picoclaw EMBER_VERSION=0.1
# Then redeploy instances to pick up the new image
eclaw deploy research --image <registry>/ember-claw-sidecar:0.1.2 ...
```

## Troubleshooting

### ImagePullBackOff

The sidecar image hasn't been pushed to the registry, or the tag doesn't exist.

```bash
# Build and push
make build-push-picoclaw EMBER_VERSION=0.1

# Redeploy with the correct image tag
eclaw deploy research --image <registry>/ember-claw-sidecar:0.1.1 ...
```

### CrashLoopBackOff

Check logs for configuration errors:
```bash
eclaw logs research
```

Common causes:
- Invalid API key
- Wrong provider name
- Missing model name
- PicoClaw config resolution failure

### "max_tool_iterations" / "no response to give"

The PicoClaw agent exhausted its tool iteration budget. Increase it:
```bash
eclaw set-secret research PICOCLAW_AGENTS_DEFAULTS_MAX_TOOL_ITERATIONS 100
```

### "Command blocked by safety guard"

PicoClaw's workspace restriction is blocking commands. This is disabled by default in ember-claw configs, but if you see this on older instances:
```bash
eclaw set-secret research PICOCLAW_AGENTS_DEFAULTS_RESTRICT_TO_WORKSPACE false
eclaw set-secret research PICOCLAW_AGENTS_DEFAULTS_ALLOW_READ_OUTSIDE_WORKSPACE true
```

### "pip install" fails in container

If pip complains about "externally-managed-environment", the image needs rebuilding with the `PIP_BREAK_SYSTEM_PACKAGES=1` env var (already set in current Dockerfile). Rebuild and push:
```bash
make build-push-picoclaw EMBER_VERSION=0.1
```

### Connection Refused on Chat

The pod may not be ready yet. Check status:
```bash
eclaw status research
```

Wait for `Running` status. The sidecar starts the gRPC server after PicoClaw initializes.

### RBAC Permission Denied

Verify your kubeconfig has the required permissions:
```bash
kubectl auth can-i create deployments -n picoclaw
kubectl auth can-i create secrets -n picoclaw
kubectl auth can-i create pods/portforward -n picoclaw
```

All should return `yes`.

### Port-Forward URL Errors

If you see `invalid URL escape "%2F"` errors, your kubeconfig may have a Rancher proxy URL with encoded slashes. This was fixed in ember-claw's port-forward implementation. Ensure you're using the latest `eclaw` binary:
```bash
make build-eclaw
```
