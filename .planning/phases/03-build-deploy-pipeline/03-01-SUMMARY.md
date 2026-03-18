---
phase: 03-build-deploy-pipeline
plan: "01"
subsystem: infra
tags: [docker, makefile, dockerfile, alpine, buildx, linux/amd64, picoclaw]

# Dependency graph
requires:
  - phase: 01-proto-sidecar
    provides: cmd/sidecar binary entry point
  - phase: 02-cli-k8s-integration
    provides: eclaw CLI with deploy command and kubeconfig/namespace flags
provides:
  - Multi-stage Dockerfile producing static sidecar binary on alpine:3.23 runtime
  - Makefile with build-picoclaw, push-picoclaw, build-push-picoclaw, build-eclaw, deploy-picoclaw, help targets
  - .dockerignore excluding go.work, .planning/, bin/, .git/, .ember-build-numbers
  - .ember-build-numbers build counter file (auto-created by make build-picoclaw)
  - Interactive deploy wizard collecting name/provider/api-key/model via shell read with non-interactive override
affects: [deploy-workflow, cluster-targeting, build-pipeline]

# Tech tracking
tech-stack:
  added: [docker buildx, alpine:3.23, golang:1.25-alpine, GNU Make]
  patterns:
    - Two-stage Go Docker build with module layer caching
    - .ember-build-numbers file for EMBER_VERSION.N build counter
    - macOS-compatible sed -i.bak for build counter update
    - Interactive Makefile wizard with non-interactive override via env vars
    - GO variable auto-detects go binary for portability across environments

key-files:
  created:
    - Dockerfile
    - .dockerignore
    - Makefile
  modified: []

key-decisions:
  - "GO ?= $(shell which go ...) variable makes build-eclaw portable across environments where go is not on PATH"
  - "SHELL := /bin/bash and export PATH in Makefile ensures grep/sed/cut/head available in recipe shells"
  - "Non-root picoclaw user (uid/gid 1000) matches PVC mount path /home/picoclaw/.picoclaw ownership expectations"
  - "deploy-picoclaw uses build-eclaw as prerequisite to auto-compile eclaw before wizard runs"
  - "API key collected with read -s (silent mode) to prevent terminal echo"

patterns-established:
  - "Pattern: build-picoclaw increments .ember-build-numbers counter and tags image as EMBER_VERSION.N"
  - "Pattern: deploy-picoclaw wizard supports both interactive (terminal) and non-interactive (CI env vars) modes"
  - "Pattern: Makefile targets match umbrella repo naming - build-{service}, push-{service}, deploy-{service}"

requirements-completed: [K8S-01, BLD-01, BLD-02, BLD-03, BLD-04]

# Metrics
duration: 16min
completed: 2026-03-18
---

# Phase 3 Plan 01: Build + Deploy Pipeline Summary

**Multi-stage Dockerfile (golang:1.25-alpine -> alpine:3.23) and Makefile with build-picoclaw/push-picoclaw/deploy-picoclaw targets matching umbrella repo conventions, targeting reg.r.lastbot.com and emberchat cluster namespace picoclaw**

## Performance

- **Duration:** 16 min
- **Started:** 2026-03-18T20:36:58Z
- **Completed:** 2026-03-18T20:52:53Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments

- Dockerfile builds a static linux/amd64 sidecar binary using two-stage Go build; docker buildx build verified passing
- Makefile implements all 6 targets with umbrella repo pattern: build-eclaw, build-picoclaw, push-picoclaw, build-push-picoclaw, deploy-picoclaw, help
- Build numbering (.ember-build-numbers) increments correctly with EMBER_VERSION; macOS sed -i.bak used for safe in-place update
- Interactive deploy wizard collects name/provider/api-key (silent)/model with sensible resource defaults and delegates to eclaw
- Non-interactive override for CI: `make deploy-picoclaw NAME=alice PROVIDER=anthropic API_KEY=sk-xxx MODEL=...`

## Task Commits

Each task was committed atomically:

1. **Task 1: Dockerfile and .dockerignore** - `1b9b410` (feat)
2. **Task 2: Makefile with build, push, deploy, and help targets** - `36d1ed6` (feat)

## Files Created/Modified

- `Dockerfile` - Two-stage Go build: golang:1.25-alpine builder, alpine:3.23 runtime with picoclaw user (uid 1000)
- `.dockerignore` - Excludes go.work, go.work.sum, .planning/, bin/, .git/, .ember-build-numbers, *.md
- `Makefile` - build-eclaw, build-picoclaw, push-picoclaw, build-push-picoclaw, deploy-picoclaw, help

## Decisions Made

- Used `GO ?= $(shell which go ...)` variable instead of bare `go` command so build-eclaw works when `/usr/local/go/bin` is not on the default Make shell PATH
- Added `SHELL := /bin/bash` and `export PATH :=` at top of Makefile so grep/sed/cut/head are available in multi-line recipe shells (macOS /bin/sh does not have these on PATH)
- Added `.PHONY` declarations for all targets to prevent make from treating target names as file targets

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed build counter grep returning multiple lines causing arithmetic failure**
- **Found during:** Task 2 verification (second make build-picoclaw run)
- **Issue:** When .ember-build-numbers had duplicate entries (from a failed run), `grep "^$SERVICE_NAME:" | cut -d: -f2` returned two lines, causing `$((...))` arithmetic to fail with "bad math expression: operator expected"
- **Fix:** Added `| head -1` to both grep pipelines that extract build number to always take the first match
- **Files modified:** Makefile
- **Verification:** Second versioned build correctly tagged as 0.1.2 with single-entry .ember-build-numbers file
- **Committed in:** 36d1ed6 (Task 2 commit)

**2. [Rule 2 - Missing Critical] Added PATH and SHELL settings for Make recipe portability**
- **Found during:** Task 2 verification (make build-eclaw in agent environment)
- **Issue:** Make's /bin/sh recipe shell did not have go, grep, sed, head, cut on PATH; build-eclaw and build-picoclaw both failed
- **Fix:** Added `SHELL := /bin/bash`, `export PATH := /usr/local/go/bin:/usr/local/bin:/usr/bin:/bin:$(PATH)`, and `GO ?= $(shell which go ...)` to ensure all required commands are available in recipe shells
- **Files modified:** Makefile
- **Verification:** `make build-eclaw` and `make build-picoclaw EMBER_VERSION=0.1` both succeed without PATH pre-configuration
- **Committed in:** 36d1ed6 (Task 2 commit)

---

**Total deviations:** 2 auto-fixed (1 bug fix, 1 missing critical)
**Impact on plan:** Both fixes necessary for correctness and portability. No scope creep.

## Issues Encountered

- Docker credential helper not on agent PATH — resolved by confirming docker-credential-desktop at /usr/local/bin and running with PATH="/usr/local/bin:$PATH". Not a code issue; standard developer setup.
- GNU Make 3.81 shell recipe PATH propagation: `export PATH :=` in Makefile correctly exports to recipe subshells but `go` was not found via bare command lookup in agent environment. Resolved with `GO ?= $(shell which go ...)` variable that falls back to /usr/local/go/bin/go explicitly.

## User Setup Required

None — no external service configuration required for building. Developer must:
1. `docker login reg.r.lastbot.com` before running `make push-picoclaw` (standard pre-push step, documented in help text)
2. Ensure kubeconfig exists at `/Users/tuomas/Projects/ember.kubeconfig.yaml` or override with `KUBECONFIG_PATH=...`

## Next Phase Readiness

This is the final plan in the final phase. The full ember-claw build and deploy pipeline is complete:
- Phase 1: gRPC sidecar server with PicoClaw ProcessDirect integration
- Phase 2: eclaw CLI with K8s resource management and chat commands
- Phase 3: Dockerfile + Makefile wiring both into a Docker image + interactive deploy workflow

Remaining manual verifications (not blocking plan completion):
- `docker login reg.r.lastbot.com` and `make push-picoclaw` against live registry
- `make deploy-picoclaw` end-to-end against emberchat cluster (requires RBAC verification per RESEARCH.md Pitfall 7)

## Self-Check: PASSED

- FOUND: Dockerfile
- FOUND: .dockerignore
- FOUND: Makefile
- FOUND: 03-01-SUMMARY.md
- FOUND commit: 1b9b410 (Task 1)
- FOUND commit: 36d1ed6 (Task 2)

---
*Phase: 03-build-deploy-pipeline*
*Completed: 2026-03-18*
