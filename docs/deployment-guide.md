# Deployment Guide

Step-by-step guide for deploying PicoClaw instances on the emberchat Kubernetes cluster.

## Prerequisites

1. **Go 1.25+** installed
2. **Docker** with buildx plugin
3. **kubectl** configured
4. **Registry access** — `docker login reg.r.lastbot.com`
5. **Kubeconfig** — cluster credentials at `/Users/tuomas/Projects/ember.kubeconfig.yaml` or set via `KUBECONFIG` env

## First-Time Setup

### 1. Build the CLI

```bash
make build-eclaw
```

This produces `./bin/eclaw`.

### 2. Build and Push the Sidecar Image

```bash
make build-push-picoclaw EMBER_VERSION=0.1
```

This:
- Runs a multi-stage Docker build (Go compilation + Alpine runtime)
- Auto-increments the build number in `.ember-build-numbers`
- Tags as `reg.r.lastbot.com/ember-claw-sidecar:0.1.<build_number>`
- Pushes to the registry

### 3. Verify Cluster Access

```bash
kubectl --kubeconfig /Users/tuomas/Projects/ember.kubeconfig.yaml \
  get namespaces | grep picoclaw
```

The `picoclaw` namespace should exist. If not:
```bash
kubectl --kubeconfig /Users/tuomas/Projects/ember.kubeconfig.yaml \
  create namespace picoclaw
```

## Deploying an Instance

### Interactive (Recommended)

```bash
make deploy-picoclaw EMBER_VERSION=0.1
```

The wizard prompts for:
- **Instance name** — lowercase, alphanumeric + hyphens (e.g., `research`, `test-bot`)
- **AI provider** — `anthropic`, `openai`, `copilot`, etc.
- **API key** — entered silently (not echoed)
- **Model name** — e.g., `claude-sonnet-4-20250514`, `gpt-4o`

Resource defaults (100m CPU, 128Mi memory, 1Gi storage) can be overridden with Make variables:
```bash
make deploy-picoclaw EMBER_VERSION=0.1 CPU_LIM=1000m MEM_LIM=1Gi
```

### Non-Interactive

```bash
make deploy-picoclaw \
  NAME=research \
  PROVIDER=anthropic \
  API_KEY=sk-ant-api03-xxxx \
  MODEL=claude-sonnet-4-20250514 \
  EMBER_VERSION=0.1
```

### Direct CLI

```bash
./bin/eclaw deploy research \
  --provider anthropic \
  --api-key sk-ant-api03-xxxx \
  --model claude-sonnet-4-20250514 \
  --image reg.r.lastbot.com/ember-claw-sidecar:0.1.1 \
  --kubeconfig /Users/tuomas/Projects/ember.kubeconfig.yaml
```

## Verifying Deployment

```bash
# List all instances
./bin/eclaw list --kubeconfig /Users/tuomas/Projects/ember.kubeconfig.yaml

# Check specific instance
./bin/eclaw status research --kubeconfig /Users/tuomas/Projects/ember.kubeconfig.yaml

# View logs
./bin/eclaw logs research --kubeconfig /Users/tuomas/Projects/ember.kubeconfig.yaml
```

Wait for the instance status to show `Running` before attempting to chat.

## Chatting

### Interactive Session

```bash
./bin/eclaw chat research --kubeconfig /Users/tuomas/Projects/ember.kubeconfig.yaml
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
./bin/eclaw chat research -m "Summarize the last meeting notes" \
  --kubeconfig /Users/tuomas/Projects/ember.kubeconfig.yaml
```

## Managing Instances

### List All

```bash
eclaw list
```

### Delete

```bash
eclaw delete research
```

This removes the Deployment, Service, Secret, and ConfigMap. You will be prompted before the PVC is deleted (data loss).

### View Logs

```bash
eclaw logs research --follow    # Stream logs in real-time
eclaw logs research --lines 50  # Last 50 lines
```

## Updating an Instance

To change the AI provider, model, or other configuration:

1. Delete the existing instance: `eclaw delete research`
2. Redeploy with new config: `eclaw deploy research --provider openai --api-key ... --model gpt-4o`

## Troubleshooting

### ImagePullBackOff

The sidecar image hasn't been pushed to the registry. Run:
```bash
make build-push-picoclaw EMBER_VERSION=0.1
```

Then redeploy with the correct image tag:
```bash
eclaw deploy research --image reg.r.lastbot.com/ember-claw-sidecar:0.1.1 ...
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

### Connection Refused on Chat

The pod may not be ready yet. Check status:
```bash
eclaw status research
```

Wait for `Running` status before chatting. The sidecar starts the gRPC server after PicoClaw initializes.

### RBAC Permission Denied

Verify your kubeconfig has the required permissions:
```bash
kubectl --kubeconfig /path/to/kubeconfig auth can-i create deployments -n picoclaw
kubectl --kubeconfig /path/to/kubeconfig auth can-i create secrets -n picoclaw
kubectl --kubeconfig /path/to/kubeconfig auth can-i create pods/portforward -n picoclaw
```

All should return `yes`.
