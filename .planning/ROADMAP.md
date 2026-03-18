# Roadmap: Ember-Claw

## Overview

Ember-Claw delivers a Kubernetes deployment toolkit for PicoClaw AI assistant instances in three phases, following the dependency chain: build the gRPC sidecar that wraps PicoClaw as a Go library, then the CLI that manages instances and connects to them, then the build/deploy pipeline that wires everything for production use. Each phase produces a working, testable artifact.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [x] **Phase 1: Proto + Sidecar** - gRPC server binary importing PicoClaw as library with bidirectional chat, single-shot query, and health checks (completed 2026-03-16)
- [x] **Phase 2: CLI + K8s Integration** - Go CLI tool managing full instance lifecycle (deploy/list/delete/status/logs/chat) via client-go and gRPC (completed 2026-03-16)
- [ ] **Phase 3: Build + Deploy Pipeline** - Dockerfile, Make targets, interactive deployment wizard, and K8s manifests for the emberchat cluster

## Phase Details

### Phase 1: Proto + Sidecar
**Goal**: A running gRPC server that wraps PicoClaw as a Go library -- the foundation that both CLI and deployment depend on
**Depends on**: Nothing (first phase)
**Requirements**: GRPC-01, GRPC-02, GRPC-03, GRPC-04, GRPC-05, K8S-04
**Success Criteria** (what must be TRUE):
  1. Sidecar binary starts, imports PicoClaw via `ProcessDirect()` API, and responds to gRPC calls on port 50051
  2. A gRPC client can open a bidirectional streaming chat session and exchange multiple messages with PicoClaw
  3. A gRPC client can send a single-shot query and receive a complete response via unary RPC
  4. Kubernetes liveness and readiness probes pass against the health check endpoint
  5. Two simultaneous gRPC client connections maintain isolated sessions (different session keys, independent conversation state)
**Plans**: 2 plans

Plans:
- [x] 01-01-PLAN.md — Go module init, proto definition, code generation, test scaffolds (RED)
- [x] 01-02-PLAN.md — gRPC server implementation, health checks, sidecar binary (GREEN)

### Phase 2: CLI + K8s Integration
**Goal**: Developers can manage the full lifecycle of named PicoClaw instances on the emberchat cluster from a single `eclaw` binary
**Depends on**: Phase 1
**Requirements**: CLI-01, CLI-02, CLI-03, CLI-04, CLI-05, CHAT-01, CHAT-02, CHAT-03, CONF-01, CONF-02, CONF-03, CONF-04, CONF-05, K8S-02, K8S-03
**Success Criteria** (what must be TRUE):
  1. `eclaw deploy <name>` creates a named PicoClaw instance with Deployment, Service, PVC, Secret, and ConfigMap on the emberchat cluster
  2. `eclaw list` shows all managed instances with name, status, and age; `eclaw status <name>` shows detailed health, uptime, and config
  3. `eclaw delete <name>` tears down all resources for an instance, prompting before PVC deletion
  4. `eclaw chat <name>` opens an interactive terminal session via gRPC bidirectional stream, and `eclaw chat <name> -m "message"` sends a single-shot query -- both using auto-established port-forward
  5. Instances are configurable at deploy time: AI provider/model/API key (stored as K8s Secret), CPU/memory limits, custom env vars, and user-chosen names
**Plans**: 3 plans

Plans:
- [x] 02-01-PLAN.md — K8s client abstraction, labels, resource CRUD with fake-clientset tests (completed 2026-03-16)
- [x] 02-02-PLAN.md — Cobra CLI binary with deploy, list, delete, status, logs subcommands
- [x] 02-03-PLAN.md — gRPC client, port-forward, interactive/single-shot chat command

### Phase 3: Build + Deploy Pipeline
**Goal**: Complete workflow from `make deploy-picoclaw` to chatting with a running instance, matching the umbrella repo's build conventions
**Depends on**: Phase 2
**Requirements**: K8S-01, BLD-01, BLD-02, BLD-03, BLD-04
**Success Criteria** (what must be TRUE):
  1. Multi-stage Dockerfile builds a minimal sidecar image containing PicoClaw for linux/amd64
  2. `make build-picoclaw` builds the container image and `make push-picoclaw` pushes it to `reg.r.lastbot.com`
  3. `make deploy-picoclaw` launches an interactive wizard that prompts for instance name, AI provider, API key, model, resource limits, and environment variables, then deploys to the emberchat cluster
  4. Kubernetes manifests target the emberchat cluster with correct namespace, labels, and resource definitions
**Plans**: 2 plans

Plans:
- [x] 03-01-PLAN.md — Dockerfile, .dockerignore, and Makefile with build/push/deploy targets (completed 2026-03-18)
- [ ] 03-02-PLAN.md — RBAC verification and end-to-end pipeline validation on emberchat cluster

## Progress

**Execution Order:**
Phases execute in numeric order: 1 -> 2 -> 3

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Proto + Sidecar | 2/2 | Complete    | 2026-03-16 |
| 2. CLI + K8s Integration | 3/3 | Complete    | 2026-03-16 |
| 3. Build + Deploy Pipeline | 1/2 | In progress | - |
