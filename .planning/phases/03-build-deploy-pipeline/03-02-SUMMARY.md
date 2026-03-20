---
phase: 03-build-deploy-pipeline
plan: "02"
subsystem: infra
tags: [e2e, validation, pipeline, registry, deploy, container]

# Dependency graph
requires:
  - phase: 03-01
    provides: Dockerfile, Makefile with build/push/deploy targets
provides:
  - Verified end-to-end pipeline: build -> push -> deploy -> chat
  - Container fixes: PIP_BREAK_SYSTEM_PACKAGES, safety guard disable, pip persistence to PVC
  - Operational improvements: upsert deploys, auto-namespace, status resolution, klog suppression
  - Integration tools: Linear and Slack tools registered in sidecar
  - Documentation: README, architecture.md, deployment-guide.md, tool-development.md
affects: []

# Tech tracking
tech-stack:
  added:
    - "PIP_USER=1 + PYTHONUSERBASE for pip persistence to PVC"
    - "klog suppression (stderrthreshold=FATAL) for clean CLI output"
  patterns:
    - "Upsert semantics for all K8s resources (create-or-update)"
    - "Container-optimized PicoClaw config: no deny patterns, no workspace restriction, 50 max iterations"
    - "Provider-specific API key resolution from env vars"

key-files:
  created:
    - internal/tools/linear/client.go
    - internal/tools/linear/tools.go
    - internal/tools/slack/client.go
    - internal/tools/slack/tools.go
    - internal/envfile/envfile.go
    - internal/cli/models.go
    - internal/cli/setsecret.go
    - docs/tool-development.md
    - docs/deployment-guide.md
  modified:
    - Dockerfile
    - Makefile
    - cmd/eclaw/main.go
    - internal/k8s/resources.go
    - internal/k8s/portforward.go
    - internal/cli/deploy.go
    - internal/cli/list.go
    - internal/server/server.go
    - README.md
    - docs/architecture.md

key-decisions:
  - "PIP_BREAK_SYSTEM_PACKAGES=1 and PIP_USER=1 with PYTHONUSERBASE on PVC for pip persistence across restarts"
  - "enable_deny_patterns=false in generated config.json — containers are isolated, safety guard too restrictive"
  - "max_tool_iterations=50 (up from PicoClaw default of 20) for complex agent tasks"
  - "klog stderrthreshold=FATAL suppresses SPDY 'connection reset by peer' noise from port-forward"
  - "Docker images always tagged :latest alongside versioned tag for easy deployment"

requirements-completed: [K8S-01, BLD-03, BLD-04, CONF-05, K8S-02, K8S-03]

# Metrics
duration: manual (iterative testing over multiple sessions)
completed: 2026-03-19
---

# Phase 3 Plan 02: E2E Pipeline Validation Summary

**End-to-end pipeline verified through iterative live testing: image pushed to reg.r.lastbot.com, instances deployed and chatted with on emberchat cluster, with numerous fixes applied for container runtime, K8s integration, and operational issues**

## Performance

- **Duration:** Iterative (across 2026-03-18 to 2026-03-19 sessions)
- **Tasks:** 2 (RBAC verification + E2E pipeline)
- **Files modified:** 20+

## Accomplishments

- Full pipeline verified: `make build-push-picoclaw` -> `eclaw deploy` -> `eclaw chat` working end-to-end
- Fixed container runtime issues: pip PEP 668 block, safety guard blocking shell commands, pip packages not persisting
- Fixed K8s integration: health probe paths, upsert semantics, auto-namespace creation, status resolution from pod/container state
- Fixed port-forward URL construction for Rancher proxy paths with encoded separators
- Added Linear and Slack integration tools with env-var-based registration
- Added set-secret command for injecting env vars into running instances
- Added models command, .env file loading, provider-specific API key resolution
- Comprehensive documentation: README, architecture, deployment guide, tool development guide

## Key Fixes Applied During Validation

1. **ImagePullBackOff** — added `:latest` tag to build + push targets
2. **Port-forward URL encoding** — proper URL construction for Rancher proxy paths
3. **Health probe 404s** — corrected paths from `/healthz` to `/health` and `/ready`
4. **Deploy "already exists"** — implemented upsert semantics for all K8s resources
5. **Stale list status** — resolved status from container state > pod phase > deployment replicas
6. **PEP 668 pip block** — `PIP_BREAK_SYSTEM_PACKAGES=1` in Dockerfile
7. **Safety guard blocking** — `enable_deny_patterns: false` in generated config
8. **pip packages lost on restart** — `PIP_USER=1` + `PYTHONUSERBASE` pointed at PVC
9. **klog noise** — suppressed SPDY "connection reset by peer" via stderrthreshold=FATAL

## Self-Check: PASSED

- Image pushed to reg.r.lastbot.com/ember-claw-sidecar:0.1.10 and :latest
- Instance deployed and running on emberchat cluster
- Chat sessions working via port-forward
- All fixes committed and pushed

---
*Phase: 03-build-deploy-pipeline*
*Completed: 2026-03-19*
