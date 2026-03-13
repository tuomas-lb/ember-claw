# Ember-Claw

## What This Is

A Kubernetes deployment toolkit and management CLI for running multiple PicoClaw AI assistant instances on the emberchat Kubernetes cluster. Includes a gRPC-based sidecar/wrapper that bridges chat connections to PicoClaw, Kubernetes manifests with interactive deployment via Make targets, and a Go CLI tool for full instance lifecycle management (deploy, list, chat, delete, status, logs).

## Core Value

Effortless deployment and interaction with named PicoClaw instances — from `make deploy` to chatting with a running instance should be trivially simple.

## Requirements

### Validated

(None yet — ship to validate)

### Active

- [ ] Interactive deployment via Make target (prompts for name, AI config, resources, env vars)
- [ ] gRPC sidecar/wrapper in ember-claw that bridges chat to PicoClaw stdin/stdout
- [ ] Go CLI tool with full management: deploy, list, delete, status, logs, chat
- [ ] CLI interactive chat mode (live terminal session via gRPC stream)
- [ ] CLI single-shot query mode (send message, get response)
- [ ] Persistent storage (PVC) per PicoClaw instance
- [ ] User-chosen instance names
- [ ] Configurable AI provider per instance (API keys, model, endpoint)
- [ ] Configurable resource limits per instance (CPU/memory)
- [ ] Custom environment variables per instance
- [ ] Kubernetes manifests targeting emberchat cluster (rancher-based at rancher-2.kuupo.com)

### Out of Scope

- Modifying the upstream PicoClaw codebase — all extensions live in ember-claw
- Multi-cluster support — emberchat cluster only
- Web UI for management — CLI only
- Auto-scaling — manual instance count management
- Authentication/authorization for gRPC — internal cluster use only

## Context

- **PicoClaw** is an ultra-lightweight Go-based AI assistant (<10MB RAM, 1s boot). It's a terminal application that interacts via stdin/stdout. Source lives at `/Users/tuomas/Projects/picoclaw/`.
- **Emberchat Kubernetes** is a Rancher-managed cluster at `rancher-2.kuupo.com`. Kubeconfig at `/Users/tuomas/Projects/ember.kubeconfig.yaml`.
- **Architecture pattern**: A gRPC sidecar/wrapper process runs alongside PicoClaw in each pod. The sidecar bridges gRPC streaming calls to PicoClaw's stdin/stdout. The CLI tool connects to the sidecar via port-forward or cluster-internal service.
- **This is a dev/test fleet** — instances are for experimentation, not production user-facing workloads.
- **Existing umbrella repo pattern**: The parent directory uses Make + Docker + Kubernetes orchestration (see CLAUDE.md). Ember-claw should follow similar conventions.

## Constraints

- **Language**: Go for both the gRPC sidecar and CLI tool (matches PicoClaw ecosystem)
- **Cluster**: Emberchat Kubernetes via Rancher, kubeconfig-based access
- **Platform**: linux/amd64 container images
- **PicoClaw**: Treat as upstream dependency — do not fork or modify, wrap it
- **Deployment**: Interactive Make targets (not Helm charts or operators)

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| gRPC for chat protocol | Efficient binary streaming, Go-native, bidirectional streaming for interactive chat | — Pending |
| Sidecar in ember-claw, not picoclaw | Keep upstream clean, this is deployment-specific tooling | — Pending |
| Go CLI (not shell scripts) | Single binary, matches ecosystem, can embed kubectl-like k8s client | — Pending |
| PVC per instance | PicoClaw has memory/planning features that benefit from persistence | — Pending |
| Make for deployment (not Helm) | Matches existing umbrella repo patterns, interactive prompts are easier in Make+shell | — Pending |

---
*Last updated: 2026-03-13 after initialization*
