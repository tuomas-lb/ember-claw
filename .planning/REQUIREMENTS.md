# Requirements: Ember-Claw

**Defined:** 2026-03-13
**Core Value:** Effortless deployment and interaction with named PicoClaw instances — from `make deploy` to chatting with a running instance should be trivially simple.

## v1 Requirements

Requirements for initial release. Each maps to roadmap phases.

### gRPC Sidecar

- [x] **GRPC-01**: gRPC server binary imports PicoClaw as Go library (using `ProcessDirect()` API)
- [x] **GRPC-02**: Bidirectional streaming RPC for interactive chat sessions
- [x] **GRPC-03**: Unary RPC for single-shot queries
- [x] **GRPC-04**: Health check RPC for readiness/liveness probes
- [x] **GRPC-05**: Session isolation per gRPC client connection

### CLI Management

- [x] **CLI-01**: `eclaw deploy <name>` creates a named PicoClaw instance (Deployment + Service + PVC)
- [x] **CLI-02**: `eclaw list` shows all managed instances with status
- [x] **CLI-03**: `eclaw delete <name>` tears down instance (with PVC deletion prompt)
- [x] **CLI-04**: `eclaw status <name>` shows instance health, uptime, and config
- [x] **CLI-05**: `eclaw logs <name>` streams pod logs with `--follow` support

### CLI Chat

- [ ] **CHAT-01**: `eclaw chat <name>` enters interactive terminal chat via gRPC stream
- [ ] **CHAT-02**: `eclaw chat <name> -m "message"` sends single-shot query and prints response
- [ ] **CHAT-03**: CLI auto-establishes port-forward to target pod for gRPC connection

### Deployment Configuration

- [x] **CONF-01**: User-chosen instance names (e.g., `picoclaw-research`)
- [x] **CONF-02**: Configurable AI provider per instance (API keys via K8s Secrets, model, endpoint)
- [x] **CONF-03**: Configurable resource limits per instance (CPU/memory requests and limits)
- [x] **CONF-04**: Custom environment variables per instance
- [x] **CONF-05**: Persistent storage (PVC) per instance survives pod restarts

### Kubernetes Integration

- [ ] **K8S-01**: Kubernetes manifests target emberchat cluster (rancher-based)
- [x] **K8S-02**: Label-based instance discovery (`app.kubernetes.io/*` labels)
- [x] **K8S-03**: API keys stored in Kubernetes Secrets (not plaintext env vars)
- [x] **K8S-04**: K8s liveness/readiness probes wired to health check endpoint

### Build & Deploy Pipeline

- [ ] **BLD-01**: Multi-stage Dockerfile builds sidecar binary with PicoClaw
- [ ] **BLD-02**: `make build-picoclaw` builds container image (linux/amd64)
- [ ] **BLD-03**: `make push-picoclaw` pushes image to `reg.r.lastbot.com`
- [ ] **BLD-04**: `make deploy-picoclaw` starts interactive deployment wizard (name, AI config, resources, env vars)

## v2 Requirements

Deferred to future release. Tracked but not in current roadmap.

### Operational Convenience

- **OPS-01**: Instance profiles/presets (e.g., `--preset claude-opus`)
- **OPS-02**: Config hot-reload without redeployment
- **OPS-03**: Shell completion for bash/zsh/fish
- **OPS-04**: Output format options (`--output json/yaml/table`)
- **OPS-05**: `eclaw status --all` dashboard view

### Advanced

- **ADV-01**: Deploy-time model/API key validation
- **ADV-02**: Multi-container log selection (`--container` flag)
- **ADV-03**: Version command showing CLI, sidecar, and cluster info

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Web UI / dashboard | CLI-only tool for dev/test fleet |
| Multi-cluster support | Emberchat cluster only |
| Auto-scaling / HPA | Manual instance management |
| Helm charts | Make targets match umbrella repo patterns |
| gRPC authentication | Internal cluster use only |
| Operator pattern (CRD) | Overkill for handful of instances |
| PicoClaw code modifications | All extensions live in ember-claw |
| Instance-to-instance communication | Each instance is independent |
| GPU resource management | PicoClaw is API-calling agent, no GPU needed |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| GRPC-01 | Phase 1 | Complete |
| GRPC-02 | Phase 1 | Complete |
| GRPC-03 | Phase 1 | Complete |
| GRPC-04 | Phase 1 | Complete |
| GRPC-05 | Phase 1 | Complete |
| CLI-01 | Phase 2 | Complete |
| CLI-02 | Phase 2 | Complete |
| CLI-03 | Phase 2 | Complete |
| CLI-04 | Phase 2 | Complete |
| CLI-05 | Phase 2 | Complete |
| CHAT-01 | Phase 2 | Pending |
| CHAT-02 | Phase 2 | Pending |
| CHAT-03 | Phase 2 | Pending |
| CONF-01 | Phase 2 | Complete |
| CONF-02 | Phase 2 | Complete |
| CONF-03 | Phase 2 | Complete |
| CONF-04 | Phase 2 | Complete |
| CONF-05 | Phase 2 | Pending |
| K8S-01 | Phase 3 | Pending |
| K8S-02 | Phase 2 | Pending |
| K8S-03 | Phase 2 | Pending |
| K8S-04 | Phase 1 | Complete |
| BLD-01 | Phase 3 | Pending |
| BLD-02 | Phase 3 | Pending |
| BLD-03 | Phase 3 | Pending |
| BLD-04 | Phase 3 | Pending |

**Coverage:**
- v1 requirements: 26 total
- Mapped to phases: 26
- Unmapped: 0

---
*Requirements defined: 2026-03-13*
*Last updated: 2026-03-13 after initial definition*
