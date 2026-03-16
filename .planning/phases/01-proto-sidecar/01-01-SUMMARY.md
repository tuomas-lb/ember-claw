---
phase: 01-proto-sidecar
plan: "01"
subsystem: api
tags: [grpc, protobuf, go, picoclaw, bufconn, testing]

# Dependency graph
requires: []
provides:
  - Go module initialized with PicoClaw + gRPC dependencies
  - proto/emberclaw/v1/service.proto with Chat (bidi), Query (unary), Status (unary) RPCs
  - Generated gRPC Go stubs in gen/emberclaw/v1/ (PicoClawServiceServer interface)
  - AgentProcessor interface in internal/server/interfaces.go for mock injection
  - RED test scaffolds for all gRPC handlers (Chat, Query, session isolation, health)
affects: [01-02, 02-cli, 03-build]

# Tech tracking
tech-stack:
  added:
    - "github.com/sipeed/picoclaw v0.2.4 (public module proxy, no go.work needed)"
    - "google.golang.org/grpc v1.79.2"
    - "google.golang.org/protobuf v1.36.11"
    - "github.com/rs/zerolog v1.34.0"
    - "github.com/caarlos0/env/v11 v11.4.0"
    - "github.com/google/uuid v1.6.0"
    - "github.com/stretchr/testify v1.11.1"
    - "google.golang.org/grpc/test/bufconn (in-memory gRPC transport for tests)"
  patterns:
    - "AgentProcessor interface for mock injection (production: *agent.AgentLoop, tests: mockProcessor)"
    - "bufconn in-memory gRPC listener for fast unit tests without network"
    - "UnimplementedPicoClawServiceServer embedded in stub for compile-safe RED tests"
    - "PicoClaw health.Server tested directly via httptest (no custom HTTP server needed)"

key-files:
  created:
    - go.mod
    - go.sum
    - proto/emberclaw/v1/service.proto
    - gen/emberclaw/v1/service.pb.go
    - gen/emberclaw/v1/service_grpc.pb.go
    - internal/server/interfaces.go
    - internal/server/server.go
    - internal/server/server_test.go
    - internal/server/health_test.go
    - .gitignore
  modified: []

key-decisions:
  - "PicoClaw is available on public Go module proxy (github.com/sipeed/picoclaw) — no go.work or replace directive needed"
  - "Proto compilation uses --go_opt=module= and --go-grpc_opt=module= flags (not paths=source_relative) to correctly output files to gen/emberclaw/v1/"
  - "Server stub embeds UnimplementedPicoClawServiceServer so tests compile and fail (Unimplemented) as intended RED phase"
  - "Health tests use httptest + RegisterOnMux (not Start()) to test /ready 503 behavior without setting ready=true"

patterns-established:
  - "Pattern: AgentProcessor interface decouples gRPC handlers from PicoClaw AgentLoop for testability"
  - "Pattern: bufconn for in-memory gRPC testing — no actual port binding, no network flakiness"
  - "Pattern: TDD RED — stub implements interface minimally, tests compile but fail with Unimplemented"

requirements-completed: []  # GRPC-01..05 are scaffolded (RED tests written); completion tracked in 01-02 (GREEN)

# Metrics
duration: 6min
completed: 2026-03-16
---

# Phase 1 Plan 01: Proto + Test Scaffolds Summary

**gRPC protobuf service (Chat bidi, Query unary, Status unary) compiled to Go stubs with AgentProcessor mock interface and RED test scaffolds covering all 5 GRPC requirements**

## Performance

- **Duration:** 6 min
- **Started:** 2026-03-16T10:54:36Z
- **Completed:** 2026-03-16T11:01:35Z
- **Tasks:** 2
- **Files modified:** 10

## Accomplishments
- Go module initialized with PicoClaw (public module — no go.work needed), gRPC, zerolog, uuid, testify, bufconn
- Proto service definition compiled to `gen/emberclaw/v1/` with correct import paths using `--go_opt=module=` flag
- AgentProcessor interface defined for mock injection — production uses `*agent.AgentLoop`, tests use `mockProcessor`
- RED test scaffolds: Chat bidi stream, Query unary, session isolation (FAIL: Unimplemented), health endpoints (PASS: use PicoClaw health.Server directly)

## Task Commits

Each task was committed atomically:

1. **Task 1: Initialize Go module, define proto, generate gRPC code** - `0411292` (feat)
2. **Task 2: Create AgentProcessor interface and RED test scaffolds** - `08fa564` (test)

## Files Created/Modified
- `go.mod` - Module definition: github.com/LastBotInc/ember-claw with all Phase 1 deps
- `go.sum` - Dependency checksums
- `proto/emberclaw/v1/service.proto` - PicoClawService: Chat (bidi stream), Query (unary), Status (unary)
- `gen/emberclaw/v1/service.pb.go` - Generated protobuf message types (ChatRequest, QueryResponse, etc.)
- `gen/emberclaw/v1/service_grpc.pb.go` - Generated gRPC stubs (PicoClawServiceServer interface)
- `internal/server/interfaces.go` - AgentProcessor interface with ProcessDirect method
- `internal/server/server.go` - Minimal stub embedding UnimplementedPicoClawServiceServer
- `internal/server/server_test.go` - RED tests: TestChatBidiStream, TestChatMultipleMessages, TestQuery, TestSessionIsolation
- `internal/server/health_test.go` - Health tests: TestHealthEndpoints (PASS), TestGRPCHealth (placeholder)
- `.gitignore` - Excludes go.work, go.work.sum, IDE files

## Decisions Made
- **PicoClaw is on public module proxy**: `go get github.com/sipeed/picoclaw@HEAD` succeeded immediately. Research noted this was LOW confidence; resolved as public.
- **Proto compilation flags**: `--go_opt=module=github.com/LastBotInc/ember-claw` strips the module path to place generated files in `gen/emberclaw/v1/` (not `gen/proto/emberclaw/v1/` which `paths=source_relative` would produce).
- **Health test approach**: PicoClaw's `health.Server.Start()` auto-sets `ready=true`. Tests use `RegisterOnMux` + `httptest.NewServer` to test `/ready` 503 behavior without calling Start().

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Corrected protoc output path flags**
- **Found during:** Task 1 (proto compilation)
- **Issue:** `--go_opt=paths=source_relative` places output at `gen/proto/emberclaw/v1/` (mirrors proto source path). Plan expected `gen/emberclaw/v1/`.
- **Fix:** Switched to `--go_opt=module=github.com/LastBotInc/ember-claw` and `--go-grpc_opt=module=...`, with `--go_out=.` (project root). This strips the module path from go_package, placing files at the correct `gen/emberclaw/v1/` path.
- **Files modified:** Generated files moved to correct location; no source files changed
- **Verification:** `go build ./gen/emberclaw/v1/...` passes
- **Committed in:** 0411292 (Task 1 commit)

---

**Total deviations:** 1 auto-fixed (1 bug — incorrect protoc flag)
**Impact on plan:** Required fix for correct output path. No scope creep.

## Issues Encountered
- `go vet` initially failed with "cannot use svc as PicoClawServiceServer" — resolved by embedding `UnimplementedPicoClawServiceServer` in the stub, which is the correct gRPC-go pattern for stubs.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Foundation complete: proto contract defined, generated code compiles, test scaffolds in RED state
- Plan 02 implements `internal/server/server.go` Chat and Query handlers to make RED tests GREEN
- No blockers: PicoClaw is available publicly, no dependency conflicts found

## Self-Check: PASSED

All claimed files exist. All commits verified.

- FOUND: go.mod
- FOUND: proto/emberclaw/v1/service.proto
- FOUND: gen/emberclaw/v1/service.pb.go
- FOUND: gen/emberclaw/v1/service_grpc.pb.go
- FOUND: internal/server/interfaces.go
- FOUND: internal/server/server_test.go
- FOUND: internal/server/health_test.go
- FOUND commit: 0411292 feat(01-01): initialize Go module, define proto, generate gRPC code
- FOUND commit: 08fa564 test(01-01): add AgentProcessor interface and RED test scaffolds

---
*Phase: 01-proto-sidecar*
*Completed: 2026-03-16*
