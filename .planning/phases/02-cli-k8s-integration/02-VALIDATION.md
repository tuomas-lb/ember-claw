---
phase: 2
slug: cli-k8s-integration
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-16
---

# Phase 2 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (stdlib) + testify v1.11.1 + k8s.io/client-go/kubernetes/fake |
| **Config file** | none (go test runs from project root) |
| **Quick run command** | `go test ./internal/k8s/... ./internal/grpcclient/... -timeout 30s` |
| **Full suite command** | `go test ./... -timeout 60s` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/k8s/... ./internal/grpcclient/... -timeout 30s`
- **After every plan wave:** Run `go test ./... -timeout 60s`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 30 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 02-01-01 | 01 | 1 | CLI-01, K8S-02, K8S-03, CONF-01..05 | unit (fake client) | `go test ./internal/k8s/... -run TestDeployInstance -v` | ❌ W0 | ⬜ pending |
| 02-01-02 | 01 | 1 | CLI-02 | unit (fake client) | `go test ./internal/k8s/... -run TestListInstances -v` | ❌ W0 | ⬜ pending |
| 02-01-03 | 01 | 1 | CLI-03 | unit (fake client) | `go test ./internal/k8s/... -run TestDeleteInstance -v` | ❌ W0 | ⬜ pending |
| 02-01-04 | 01 | 1 | CLI-04 | unit (fake client) | `go test ./internal/k8s/... -run TestInstanceStatus -v` | ❌ W0 | ⬜ pending |
| 02-01-05 | 01 | 1 | CLI-05 | unit (fake client) | `go test ./internal/k8s/... -run TestInstanceLogs -v` | ❌ W0 | ⬜ pending |
| 02-02-01 | 02 | 2 | CHAT-01 | unit (bufconn) | `go test ./internal/grpcclient/... -run TestChatStream -v` | ❌ W0 | ⬜ pending |
| 02-02-02 | 02 | 2 | CHAT-02 | unit (bufconn) | `go test ./internal/grpcclient/... -run TestQueryRPC -v` | ❌ W0 | ⬜ pending |
| 02-02-03 | 02 | 2 | CHAT-03 | manual | n/a (requires live cluster) | N/A | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/k8s/resources_test.go` — covers CLI-01..05, K8S-02, K8S-03, CONF-03, CONF-05 via fake clientset
- [ ] `internal/grpcclient/client_test.go` — covers CHAT-01, CHAT-02 using bufconn from Phase 1
- [ ] client-go dependency added: `go get k8s.io/client-go@v0.33.0 && go mod tidy`

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Port-forward to pod gRPC | CHAT-03 | SPDY protocol not supported by fake clientset | 1. Deploy instance to emberchat cluster 2. Run `eclaw chat <name>` 3. Verify gRPC connection establishes via port-forward |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 30s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
