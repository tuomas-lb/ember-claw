---
phase: 02-cli-k8s-integration
plan: "03"
subsystem: cli
tags: [grpc, portforward, spdy, readline, interactive-chat, bufconn]

# Dependency graph
requires:
  - phase: 02-cli-k8s-integration
    plan: "01"
    provides: "internal/k8s/client.go: Client struct with FindRunningPod, restConfig"
  - phase: 02-cli-k8s-integration
    plan: "02"
    provides: "internal/cli/root.go: NewRootCommand, k8sClient package var, AddCommand pattern"
provides:
  - "internal/k8s/portforward.go: PortForwardResult + PortForwardPod (SPDY in-process tunnel)"
  - "internal/grpcclient/client.go: DialSidecar (grpc.NewClient, insecure, returns PicoClawServiceClient)"
  - "internal/grpcclient/client_test.go: bufconn tests for Query RPC and Chat bidi stream"
  - "internal/cli/chat.go: chat subcommand with -m single-shot and interactive readline modes"
affects: [02-04, 02-05, 02-06]

# Tech tracking
tech-stack:
  added:
    - "github.com/chzyer/readline v1.5.1 (interactive readline REPL for chat)"
    - "github.com/moby/spdystream (transitive, pulled by portforward transport)"
  patterns:
    - "grpc.NewClient (not deprecated grpc.Dial) for all gRPC dial operations"
    - "Port 0 for OS-assigned ephemeral local port in PortForwardPod"
    - "bufconn test pattern: bufconn.Listen + grpc.NewClient with ContextDialer for in-memory gRPC testing"
    - "defer close(pf.StopChan) / defer grpcConn.Close() for port-forward + gRPC cleanup"

key-files:
  created:
    - internal/k8s/portforward.go
    - internal/grpcclient/client.go
    - internal/grpcclient/client_test.go
    - internal/cli/chat.go
  modified:
    - internal/cli/root.go
    - go.mod
    - go.sum

key-decisions:
  - "bufconn tests use grpc.NewClient with passthrough:///bufconn target + ContextDialer (not grpc.Dial)"
  - "PortForwardPod guards against nil restConfig to give clear error for fake-clientset use"
  - "TestDialSidecar_ReturnsClient tests API shape only (lazy dial); live connectivity tested against cluster"
  - "chat.go uses defer stream.CloseSend() with nolint:errcheck (CloseSend on defer is best-effort)"

patterns-established:
  - "Pattern: grpcclient.DialSidecar(ctx, pf.LocalPort) after PortForwardPod -- zero-config sidecar connectivity"
  - "Pattern: readline.New(fmt.Sprintf('[%s]> ', instanceName)) for per-instance prompt branding"

requirements-completed: [CHAT-01, CHAT-02, CHAT-03]

# Metrics
duration: 4min
completed: 2026-03-16
---

# Phase 2 Plan 03: gRPC Client and Chat Command Summary

**gRPC DialSidecar helper + K8s SPDY port-forward + interactive readline chat command wiring FindRunningPod -> PortForwardPod -> DialSidecar -> bidi stream or single-shot Query RPC, verified by 3 bufconn unit tests**

## Performance

- **Duration:** 4 min
- **Started:** 2026-03-16T12:24:04Z
- **Completed:** 2026-03-16T12:28:01Z
- **Tasks:** 2 (Task 1 TDD: RED commit + GREEN commit; Task 2 direct)
- **Files modified:** 7

## Accomplishments

- `grpc.NewClient` used throughout (not deprecated `grpc.Dial`); vet clean
- 3 bufconn unit tests (TestDialSidecar_ReturnsClient, TestQueryRPC, TestChatStream) prove Query and Chat RPCs work end-to-end against a real mock server
- `PortForwardPod` compiles with full SPDY transport (portforward.New + spdy.RoundTripperFor)
- `eclaw chat <name>` wires full pipeline: FindRunningPod -> PortForwardPod -> DialSidecar -> interactive readline or single-shot Query
- `go build ./...`, `go test ./...`, and `go vet ./...` all clean

## Task Commits

Each task was committed atomically (TDD: RED commit before GREEN):

1. **Task 1 RED: Failing gRPC client tests** - `5df5099` (test)
2. **Task 1 GREEN: grpcclient/client.go + portforward.go** - `02f7b39` (feat)
3. **Task 2: chat subcommand + root.go wire** - `ddf8c69` (feat)

## Files Created/Modified

- `internal/k8s/portforward.go` - PortForwardResult{LocalPort, StopChan}; PortForwardPod using spdy.RoundTripperFor, portforward.New with port 0, waits on readyChan, returns assigned port
- `internal/grpcclient/client.go` - DialSidecar: grpc.NewClient("localhost:{port}"), insecure creds, returns PicoClawServiceClient + *grpc.ClientConn
- `internal/grpcclient/client_test.go` - 3 tests: shape test, Query RPC via bufconn, Chat bidi stream via bufconn (sends 3 msgs, verifies 3 echo responses)
- `internal/cli/chat.go` - newChatCommand (ExactArgs(1), -m/--message flag), runChat pipeline, runSingleShot (Query RPC), runInteractive (readline + Chat bidi stream)
- `internal/cli/root.go` - Added newChatCommand() to root.AddCommand
- `go.mod` / `go.sum` - Added chzyer/readline v1.5.1 as direct dep; transitive SPDY deps pulled in

## Decisions Made

- **bufconn pattern uses passthrough:///bufconn target:** grpc.NewClient requires a valid target URI; using `passthrough:///bufconn` with a ContextDialer provides the bufconn dialer without a real address.
- **PortForwardPod nil-guards restConfig:** NewClientFromClientset (for tests) leaves restConfig nil. The guard gives a clear error message rather than a nil pointer panic when someone tries to port-forward from a test context.
- **TestDialSidecar_ReturnsClient tests lazy dial only:** grpc.NewClient is non-blocking (lazy connection). Testing it dials to port 50051 which likely has nothing -- that's fine because the test only verifies the returned types are non-nil.
- **defer stream.CloseSend() with nolint:** CloseSend on a defer path is fire-and-forget; the error is best-effort and not actionable in a cleanup path.

## Deviations from Plan

None - plan executed exactly as written.

The task called for adding readline with `go get github.com/chzyer/readline@v1.5.1` -- executed exactly as specified. All interfaces matched between plan and implementation.

## Issues Encountered

None - no blocking issues or unexpected compilation errors.

## User Setup Required

None for unit tests. Live cluster testing requires:
- Valid kubeconfig with access to the target namespace
- A running PicoClaw sidecar deployment with `eclaw deploy`
- K8s RBAC permission for `pods/portforward` subresource

## Next Phase Readiness

- Phase 2 complete: all 3 CLI+K8s plans done
- `eclaw chat` is the final missing command -- all 6 CLI subcommands now exist
- Phase 3 (build + deploy pipeline) can proceed

## Self-Check: PASSED

All created files exist on disk. All 3 task commits (5df5099, 02f7b39, ddf8c69) present in git log. client_test.go is 130 lines (min 60). chat.go is 133 lines (min 60). `go build ./...`, `go test ./...`, `go vet ./...` all clean.

---
*Phase: 02-cli-k8s-integration*
*Completed: 2026-03-16*
