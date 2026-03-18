---
phase: 3
slug: build-deploy-pipeline
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-18
---

# Phase 3 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing stdlib + testify v1.11.1 + docker buildx + make |
| **Config file** | none |
| **Quick run command** | `go build ./... && go vet ./...` |
| **Full suite command** | `go test ./... -race -count=1` |
| **Estimated runtime** | ~15 seconds (code), ~60 seconds (docker build) |

---

## Sampling Rate

- **After every task commit:** Run `go build ./... && go vet ./...`
- **After every plan wave:** Run `go test ./... -race -count=1`
- **Before `/gsd:verify-work`:** Docker build succeeds + Make targets work + manual deploy test
- **Max feedback latency:** 30 seconds (code), 120 seconds (docker)

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 03-01-01 | 01 | 1 | BLD-01 | smoke | `docker buildx build --platform linux/amd64 -f Dockerfile -t test:latest .` | ❌ W0 | ⬜ pending |
| 03-01-02 | 01 | 1 | BLD-02, BLD-03 | smoke | `make build-picoclaw` | ❌ W0 | ⬜ pending |
| 03-02-01 | 02 | 2 | BLD-04 | manual | Interactive wizard test | N/A | ⬜ pending |
| 03-02-02 | 02 | 2 | K8S-01 | manual | `kubectl get deployments -n picoclaw` | N/A | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `Dockerfile` — multi-stage build for sidecar
- [ ] `Makefile` — build/push/deploy targets
- [ ] `.dockerignore` — exclude non-build files

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Interactive deploy wizard | BLD-04 | Requires terminal stdin interaction | Run `make deploy-picoclaw`, enter values, verify instance created |
| K8s manifests target emberchat | K8S-01 | Requires live cluster | Deploy to emberchat, verify resources in picoclaw namespace |
| Image push to registry | BLD-03 | Requires registry auth | Run `make push-picoclaw`, verify image in registry |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 120s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
