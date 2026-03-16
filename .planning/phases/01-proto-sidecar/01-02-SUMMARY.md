---
phase: 01-proto-sidecar
plan: "02"
subsystem: api
tags: [grpc, go, picoclaw, gRPC-server, health, sidecar, session, uuid]

# Dependency graph
requires:
  - phase: 01-01
    provides: "Go module with gRPC deps, generated emberclaw/v1 stubs, AgentProcessor interface, RED test scaffolds"
provides:
  - Full gRPC PicoClawServiceServer implementation (Chat bidi, Query unary, Status unary)
  - Session key assignment per gRPC stream (session.go)
  - Health server wiring using PicoClaw pkg/health (health.go)
  - Sidecar binary entry point (cmd/sidecar/main.go) with full PicoClaw init + graceful shutdown
  - All RED tests from Plan 01 now GREEN
affects: [02-cli, 03-build]

# Tech tracking
tech-stack:
  added:
    - "github.com/rs/zerolog v1.34.0 (explicit dep, was transitive)"
    - "github.com/caarlos0/env/v11 v11.4.0 (bumped from v11.3.1)"
    - "github.com/google/uuid v1.6.0 (promoted to explicit dep)"
    - "Full picoclaw transitive dep tree pulled via go mod tidy (anthropic-sdk-go, openai-go/v3, etc.)"
  patterns:
    - "Server struct holds AgentProcessor field for mock injection (production: *agent.AgentLoop)"
    - "assignSessionKey(clientKey, prefix) helper centralizes UUID generation for Chat and Query handlers"
    - "grpchealth.Serving status set via healthpb.HealthCheckResponse_SERVING (grpc_health_v1 package constant)"
    - "getConfigPath() implements PICOCLAW_CONFIG > PICOCLAW_HOME > ~/.picoclaw priority chain"

key-files:
  created:
    - internal/server/server.go
    - internal/server/session.go
    - internal/server/health.go
    - cmd/sidecar/main.go
  modified:
    - go.mod
    - go.sum

key-decisions:
  - "grpchealth.Serving constant does not exist in grpc-go v1.79.2 -- use healthpb.HealthCheckResponse_SERVING from grpc_health_v1 package"
  - "Query handler returns errors in QueryResponse.Error field (not as gRPC status errors) so client always gets structured response"
  - "go mod tidy required to pull full picoclaw transitive dep tree for cmd/sidecar compilation (picoclaw/internal/server only needed health + agent)"

patterns-established:
  - "Pattern: session.go helper assignSessionKey(clientKey, prefix) used by both Chat and Query, keeping handlers clean"
  - "Pattern: StartHealthServer wraps PicoClaw health.Server with readyFunc callback for main.go wiring"
  - "Pattern: Server.SetModel/SetProvider/SetReady setters for config-time initialization after New()"

requirements-completed: [GRPC-01, GRPC-02, GRPC-03, GRPC-04, GRPC-05, K8S-04]

# Metrics
duration: 3min
completed: 2026-03-16
---

# Phase 1 Plan 02: gRPC Server Implementation Summary

**Chat/Query/Status gRPC handlers with per-stream UUID session isolation, PicoClaw health wiring, and a compilable sidecar binary importing AgentLoop via ProcessDirect**

## Performance

- **Duration:** 3 min
- **Started:** 2026-03-16T11:05:10Z
- **Completed:** 2026-03-16T11:08:29Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments
- Replaced stub server.go with full Chat (bidi stream), Query (unary), Status handlers using AgentProcessor interface; all 6 RED tests now GREEN
- session.go provides `assignSessionKey` helper ensuring each gRPC stream gets a unique UUID-based key (GRPC-05 session isolation)
- health.go wraps PicoClaw's `pkg/health.Server` for K8s HTTP probes — no custom HTTP server needed
- cmd/sidecar/main.go wires config loading, AgentLoop init, health server (port 8080), gRPC server (port 50051), and SIGTERM/SIGINT graceful shutdown

## Task Commits

Each task was committed atomically:

1. **Task 1: Implement gRPC server handlers and session management** - `48bacd2` (feat)
2. **Task 2: Create sidecar entry point with PicoClaw init and graceful shutdown** - `1e7c318` (feat)

## Files Created/Modified
- `internal/server/server.go` - Full PicoClawServiceServer: Chat, Query, Status handlers; SetModel/SetProvider/SetReady setters
- `internal/server/session.go` - assignSessionKey(clientKey, prefix) helper for per-stream UUID assignment
- `internal/server/health.go` - StartHealthServer wrapping PicoClaw pkg/health.Server
- `cmd/sidecar/main.go` - Sidecar entry point: config, AgentLoop, health server, gRPC server, graceful shutdown
- `go.mod` - Promote zerolog, env/v11, uuid to explicit deps; tidy pulls full picoclaw transitive dep tree
- `go.sum` - Updated checksums for new dependencies

## Decisions Made
- **grpchealth.Serving undefined**: In grpc-go v1.79.2, the `Serving` constant lives in `healthpb` (grpc_health_v1 package) as `HealthCheckResponse_SERVING`, not in the `grpchealth` package as the research example implied. Fixed inline.
- **Query error handling**: Plan specified returning errors in `QueryResponse.Error` (not gRPC status codes). Implemented as specified — callers always get a structured response.
- **go mod tidy scope**: When `internal/server` tests pass fine with just picoclaw as a dep, `cmd/sidecar/main.go` importing `pkg/agent`, `pkg/providers`, `pkg/bus`, etc. requires the full transitive dep tree. Ran `go mod tidy` after adding zerolog and caarlos0/env explicitly.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Corrected grpc health serving constant import path**
- **Found during:** Task 2 (sidecar main.go compilation)
- **Issue:** Research code example used `grpchealth.Serving` which does not exist in grpc-go v1.79.2; the constant is `healthpb.HealthCheckResponse_SERVING` in the `grpc_health_v1` sub-package
- **Fix:** Changed `grpchealth.Serving` to `healthpb.HealthCheckResponse_SERVING` in cmd/sidecar/main.go
- **Files modified:** cmd/sidecar/main.go
- **Verification:** `go build -o /dev/null ./cmd/sidecar/` succeeds
- **Committed in:** 1e7c318 (Task 2 commit)

**2. [Rule 3 - Blocking] Added missing transitive deps via go mod tidy**
- **Found during:** Task 2 (sidecar main.go compilation)
- **Issue:** Importing picoclaw's agent/bus/config/providers/health packages required go.sum entries not yet present; build failed with 16+ missing module errors
- **Fix:** `go get github.com/rs/zerolog` + `go get github.com/caarlos0/env/v11` then `go mod tidy` pulled full dep tree
- **Files modified:** go.mod, go.sum
- **Verification:** `go build -o /dev/null ./cmd/sidecar/` succeeds; `go vet ./...` clean
- **Committed in:** 1e7c318 (Task 2 commit)

---

**Total deviations:** 2 auto-fixed (1 bug — wrong constant name; 1 blocking — missing go.sum entries)
**Impact on plan:** Both fixes required for compilation. No scope creep.

## Issues Encountered
- `net/http` unused import in health.go initial version — caught by compiler, removed immediately.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Phase 1 complete: proto contract, generated stubs, gRPC server implementation, sidecar binary all done
- All 6 requirements met: GRPC-01 through GRPC-05 + K8S-04
- Phase 2 (02-cli) can import generated emberclaw/v1 stubs and connect to sidecar via port-forward
- No blockers: binary compiles, all tests pass, vet clean

## Self-Check: PASSED

- FOUND: internal/server/server.go
- FOUND: internal/server/session.go
- FOUND: internal/server/health.go
- FOUND: cmd/sidecar/main.go
- FOUND: go.mod
- FOUND: go.sum
- FOUND commit: 48bacd2 feat(01-02): implement gRPC server handlers and session management
- FOUND commit: 1e7c318 feat(01-02): create sidecar entry point with PicoClaw init and graceful shutdown

---
*Phase: 01-proto-sidecar*
*Completed: 2026-03-16*
