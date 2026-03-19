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

### Imperative CLI, Not Operator

Managing a handful of AI instances does not justify a CRD + controller pattern. Standard Kubernetes resources (Deployment, Service, PVC, Secret, ConfigMap) are created and deleted imperatively through the CLI.

## Data Flow

### Chat Session

```
User input
  → eclaw CLI (readline)
  → gRPC ChatRequest (bidirectional stream)
  → port-forward tunnel (SPDY)
  → Sidecar gRPC server
  → AgentLoop.ProcessDirect(ctx, message, sessionKey)
  → PicoClaw → LLM Provider API
  → Response string
  → gRPC ChatResponse
  → CLI prints to terminal
```

### Deploy Instance

```
eclaw deploy <name> --provider X --api-key Y --model Z
  → Validate instance name (DNS-safe)
  → Create Secret (API key)
  → Create ConfigMap (PicoClaw config.json with provider/model)
  → Create PVC (persistent storage)
  → Create Deployment (sidecar image, mounts, probes, resource limits)
  → Create Service (ClusterIP, port 50051)
```

### Health Checking

Two health systems run in parallel:

1. **HTTP health server** (port 8080) — PicoClaw's built-in `/health` and `/ready` endpoints, used by Kubernetes liveness and readiness probes
2. **gRPC health service** — standard `grpc.health.v1.Health` protocol for K8s 1.24+ native gRPC probes

## Session Management

Each gRPC connection gets a session key (either provided by the client or auto-assigned from the instance name). PicoClaw maintains conversation state per session key, enabling multiple independent conversations on the same instance.

## Resource Layout

```
Namespace: picoclaw
│
├── Deployment: picoclaw-research
│   └── Pod: picoclaw-research-xxxxx
│       └── Container: sidecar
│           ├── Port 50051 (gRPC)
│           ├── Port 8080 (health)
│           ├── Volume: /data/.picoclaw (from PVC)
│           ├── Volume: /config (from ConfigMap)
│           └── Env: from Secret
│
├── Service: picoclaw-research (ClusterIP → 50051)
├── Secret: picoclaw-research (API key)
├── ConfigMap: picoclaw-research (config.json)
└── PVC: picoclaw-research (1Gi default)
```

## Module Dependencies

```
github.com/LastBotInc/ember-claw
├── github.com/sipeed/picoclaw v0.2.3     # PicoClaw agent library
├── google.golang.org/grpc v1.79.2        # gRPC framework
├── google.golang.org/protobuf v1.36.11   # Protobuf runtime
├── k8s.io/client-go v0.35.2              # Kubernetes API client
├── github.com/spf13/cobra v1.10.2        # CLI framework
├── github.com/rs/zerolog v1.34.0         # Structured logging
├── github.com/chzyer/readline v1.5.1     # Interactive terminal
├── github.com/olekukonko/tablewriter     # Table output
└── github.com/fatih/color v1.18.0        # Colored output
```
