---
phase: 02-cli-k8s-integration
plan: "02"
subsystem: cli
tags: [cobra, cli, kubernetes, tablewriter, color, eclaw]

# Dependency graph
requires:
  - phase: 02-cli-k8s-integration
    plan: "01"
    provides: "internal/k8s/client.go, resources.go: DeployInstance, ListInstances, DeleteInstance, DeletePVC, GetInstanceStatus, GetInstanceLogs"
provides:
  - "cmd/eclaw/main.go: eclaw binary entry point"
  - "internal/cli/root.go: NewRootCommand with --kubeconfig and --namespace persistent flags"
  - "internal/cli/deploy.go: deploy subcommand with --provider/--api-key/--model required flags and resource/image/env flags"
  - "internal/cli/list.go: list subcommand with tablewriter table (NAME/STATUS/READY/AGE)"
  - "internal/cli/delete.go: delete subcommand with --purge flag, color warnings, y/N confirmation"
  - "internal/cli/status.go: status subcommand with key-value display"
  - "internal/cli/logs.go: logs subcommand with --follow/-f and --tail, Ctrl+C via signal+context"
  - "internal/cli/format.go: shared formatAge() helper"
affects: [02-03, 02-04, 02-05, 02-06]

# Tech tracking
tech-stack:
  added:
    - "github.com/spf13/cobra v1.10.2 (CLI framework)"
    - "github.com/fatih/color v1.18.0 (terminal color output)"
    - "github.com/olekukonko/tablewriter v1.1.3 (ASCII table rendering)"
  patterns:
    - "PersistentPreRunE initialises k8sClient once for all subcommands; help/completion are excluded"
    - "cobra.ExactArgs(1) enforces instance name argument on all lifecycle commands"
    - "context.WithCancel + os.Signal handling in logs --follow for clean Ctrl+C"
    - "formatAge() in format.go is shared helper: <1m => seconds, <1h => minutes, <24h => hours+minutes, else days"

key-files:
  created:
    - cmd/eclaw/main.go
    - internal/cli/root.go
    - internal/cli/deploy.go
    - internal/cli/list.go
    - internal/cli/delete.go
    - internal/cli/status.go
    - internal/cli/logs.go
    - internal/cli/format.go
  modified:
    - go.mod
    - go.sum

key-decisions:
  - "InstanceSummary.DeploymentName/DesiredReplicas/ReadyReplicas used in list output; STATUS derived as Running/Degraded/Pending from replica counts (plan's interface spec used Status/Ready/Total field names that don't match actual 02-01 struct)"
  - "main.go imports only internal/cli -- zero picoclaw imports in the binary entry point per RESEARCH anti-pattern"
  - "PersistentPreRunE skips k8s.NewClient for 'help' and 'completion' commands to avoid kubeconfig requirement when printing help offline"

patterns-established:
  - "Pattern: k8sClient package-level variable in cli package, set in PersistentPreRunE before each subcommand runs"
  - "Pattern: deploy validates instance name with regex before calling k8sClient.DeployInstance"

requirements-completed: [CLI-01, CLI-02, CLI-03, CLI-04, CLI-05, CONF-01, CONF-02, CONF-03, CONF-04]

# Metrics
duration: 4min
completed: 2026-03-16
---

# Phase 2 Plan 02: CLI Subcommands Summary

**Cobra CLI binary (eclaw) with 5 lifecycle subcommands wired to the k8s client abstraction: deploy, list, delete, status, and logs with tablewriter table output and color-coded confirmation prompts**

## Performance

- **Duration:** 4 min
- **Started:** 2026-03-16T12:18:07Z
- **Completed:** 2026-03-16T12:21:26Z
- **Tasks:** 2
- **Files modified:** 10

## Accomplishments

- eclaw binary compiles, `go build ./...` and `go vet ./...` both clean; zero picoclaw imports in cmd/eclaw/main.go
- All 5 subcommands (deploy, list, delete, status, logs) registered with correct flags and wired to internal/k8s methods
- tablewriter v1.1.3 used for list output; fatih/color used for delete warnings; signal-based context cancellation for logs --follow

## Task Commits

Each task was committed atomically:

1. **Task 1: Root Cobra command + eclaw entry point** - `9737a8d` (feat)
2. **Task 2: Deploy, list, delete, status, logs subcommands** - `d96695a` (feat)

**Plan metadata:** (pending)

## Files Created/Modified

- `cmd/eclaw/main.go` - Binary entry point; executes root command; exits 1 on error
- `internal/cli/root.go` - NewRootCommand: persistent --kubeconfig/--namespace flags; PersistentPreRunE builds k8sClient
- `internal/cli/deploy.go` - Deploy subcommand: --provider/--api-key/--model required; resource/image/env flags; name regex validation
- `internal/cli/list.go` - List subcommand: tablewriter table with NAME/STATUS/READY/AGE; "no instances" message
- `internal/cli/delete.go` - Delete subcommand: --purge with yellow/red color warning and y/N stdin confirmation
- `internal/cli/status.go` - Status subcommand: aligned key-value display of deployment/pod/provider/model/age
- `internal/cli/logs.go` - Logs subcommand: --follow/-f and --tail; Ctrl+C via signal+context cancellation
- `internal/cli/format.go` - formatAge() helper: human-readable duration (s/m/h/d)
- `go.mod` / `go.sum` - Added cobra v1.10.2, color v1.18.0, tablewriter v1.1.3

## Decisions Made

- **InstanceSummary field mismatch fixed (Rule 1):** The plan's interface spec listed `Status string; Ready int32; Total int32` but 02-01 built `DeploymentName string; DesiredReplicas int32; ReadyReplicas int32`. Fixed list.go to use actual fields and derive Status from replica counts (Running/Degraded/Pending).
- **main.go imports only internal/cli:** The binary entry point has zero picoclaw imports, consistent with the RESEARCH anti-pattern doc.
- **Help/completion excluded from PersistentPreRunE:** k8s.NewClient requires a reachable kubeconfig; skipping it for help and completion allows `eclaw --help` to work without cluster access.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] InstanceSummary field names in list.go didn't match 02-01 implementation**
- **Found during:** Task 2 (list subcommand)
- **Issue:** Plan's `<interfaces>` block specified `Status string; Ready int32; Total int32` on InstanceSummary, but the actual struct from 02-01 has `DeploymentName string; DesiredReplicas int32; ReadyReplicas int32` -- causing compile error
- **Fix:** Updated list.go to use `inst.DeploymentName` for context, `inst.ReadyReplicas`/`inst.DesiredReplicas` for ready column, and derived STATUS string (Running/Degraded/Pending) from replica counts
- **Files modified:** internal/cli/list.go
- **Verification:** `go build ./...` succeeds; `/tmp/eclaw list --help` renders correctly
- **Committed in:** d96695a (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (Rule 1 - Bug)
**Impact on plan:** Required fix for compilation; display outcome matches plan intent (NAME/STATUS/READY/AGE table).

## Issues Encountered

None - aside from the InstanceSummary field mismatch (documented as deviation above).

## User Setup Required

None - no external service configuration required. All commands compile and show correct help without cluster access.

## Next Phase Readiness

- `internal/cli/` package complete with all lifecycle subcommands -- ready for Plan 02-03
- Plan 02-03 (chat subcommand) can add `newChatCommand()` and wire it in `root.go` with `root.AddCommand(..., newChatCommand())`
- Port-forward scaffolding (internal/k8s/portforward.go) is deferred to Plan 02-03 or later as planned

## Self-Check: PASSED

All 7 CLI source files exist on disk. Both task commits (9737a8d, d96695a) present in git log. `go build ./...` and `go vet ./...` both clean.

---
*Phase: 02-cli-k8s-integration*
*Completed: 2026-03-16*
