---
phase: 1
slug: proto-sidecar
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-16
---

# Phase 1 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go stdlib `testing` + `github.com/stretchr/testify` v1.11.1 |
| **Config file** | none (Go testing is built-in) |
| **Quick run command** | `go test ./internal/server/... -timeout 30s` |
| **Full suite command** | `go test ./... -timeout 120s` |
| **Estimated runtime** | ~10 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/server/... -timeout 30s`
- **After every plan wave:** Run `go test ./... -timeout 120s`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 30 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 01-01-01 | 01 | 1 | GRPC-01 | unit (mock provider) | `go test ./internal/server/... -run TestProcessDirect -timeout 30s` | ❌ W0 | ⬜ pending |
| 01-01-02 | 01 | 1 | GRPC-02 | unit | `go test ./internal/server/... -run TestChat -timeout 30s` | ❌ W0 | ⬜ pending |
| 01-01-03 | 01 | 1 | GRPC-03 | unit | `go test ./internal/server/... -run TestQuery -timeout 30s` | ❌ W0 | ⬜ pending |
| 01-01-04 | 01 | 1 | GRPC-04 | unit | `go test ./internal/server/... -run TestHealth -timeout 30s` | ❌ W0 | ⬜ pending |
| 01-01-05 | 01 | 1 | GRPC-05 | unit | `go test ./internal/server/... -run TestSessionIsolation -timeout 30s` | ❌ W0 | ⬜ pending |
| 01-01-06 | 01 | 1 | K8S-04 | manual | n/a | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/server/server_test.go` — covers GRPC-01, GRPC-02, GRPC-03, GRPC-05 via mock
- [ ] `internal/server/health_test.go` — covers GRPC-04
- [ ] `go.mod` and `go.sum` — module initialization
- [ ] `gen/emberclaw/v1/*.pb.go` — proto compilation

*Note: Tests use a mock `AgentProcessor` interface rather than a live PicoClaw AgentLoop.*

```go
type AgentProcessor interface {
    ProcessDirect(ctx context.Context, content, sessionKey string) (string, error)
}
```

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| K8s probe YAML fields match health server ports/paths | K8S-04 | YAML template validation, not runtime | Inspect Deployment template: liveness probe hits `/health`, readiness hits `/ready` on correct port |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 30s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
