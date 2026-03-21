---
phase: quick
plan: 2
subsystem: tools
tags: [mcp, gmail, imap, typescript, imapflow, cli]

requires:
  - phase: 02-cli-k8s-integration
    provides: CLI framework with config patching (caldav.go pattern)
provides:
  - Gmail MCP server at tools/gmail-mcp/ with 6 read-only IMAP tools
  - eclaw set-gmail CLI command for multi-mailbox configuration
affects: [cli, mcp-servers]

tech-stack:
  added: ["@modelcontextprotocol/sdk", "imapflow"]
  patterns: [multi-account MCP config via JSON env var, single server key with embedded mailbox array]

key-files:
  created:
    - tools/gmail-mcp/package.json
    - tools/gmail-mcp/tsconfig.json
    - tools/gmail-mcp/src/index.ts
    - internal/cli/gmail.go
  modified:
    - internal/cli/root.go

key-decisions:
  - "Single 'gmail' MCP server key with multi-mailbox JSON config (not per-mailbox server entries like caldav)"
  - "imapflow search() returns false|number[] -- guarded with || [] fallback"
  - "On-demand IMAP connections (connect per tool call, disconnect after) to avoid long-lived connections"

patterns-established:
  - "Multi-account MCP pattern: single server entry with JSON env var containing account array"

requirements-completed: [GMAIL-MCP]

duration: 3min
completed: 2026-03-21
---

# Quick Task 2: Gmail MCP Server Summary

**Self-hosted Gmail MCP server with 6 read-only IMAP tools and eclaw set-gmail CLI command for multi-mailbox configuration**

## Performance

- **Duration:** 3 min
- **Started:** 2026-03-21T20:01:45Z
- **Completed:** 2026-03-21T20:04:35Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments
- TypeScript MCP server with 6 tools: list_mailboxes, list_folders, search, read, list_recent, count_unread
- Multi-mailbox support via GMAIL_MCP_CONFIG JSON env var with per-mailbox IMAP credentials
- eclaw set-gmail CLI command that patches instance config.json to register/update/remove Gmail mailboxes
- On-demand IMAP connections with TLS, suppressed logger to prevent stdio corruption

## Task Commits

Each task was committed atomically:

1. **Task 1: Create Gmail MCP server (TypeScript)** - `05cd425` (feat)
2. **Task 2: Add eclaw set-gmail CLI command** - `64fe474` (feat)

## Files Created/Modified
- `tools/gmail-mcp/package.json` - Node.js project with MCP SDK and imapflow deps
- `tools/gmail-mcp/tsconfig.json` - TypeScript config targeting ES2022/Node16
- `tools/gmail-mcp/src/index.ts` - MCP server with 6 gmail tools (363 lines)
- `internal/cli/gmail.go` - set-gmail CLI command following caldav.go pattern
- `internal/cli/root.go` - Registered newSetGmailCommand()

## Decisions Made
- Used single "gmail" server key with multi-mailbox JSON config instead of per-mailbox server entries (unlike caldav which uses caldav-{name} keys). This is because all mailboxes share one IMAP server process.
- Guarded imapflow search() return type (can be `false | number[]`) with `|| []` fallback
- Set imapflow logger to `false` to prevent debug output on stderr corrupting MCP stdio transport

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed imapflow TypeScript type errors**
- **Found during:** Task 1 (TypeScript build)
- **Issue:** imapflow `search()` returns `false | number[]` and `fetchOne()` returns `false | FetchMessageObject` -- strict mode rejected `.length`, `.slice()`, `.envelope` access
- **Fix:** Added `|| []` fallback for search results and null guard with error throw for fetchOne
- **Files modified:** tools/gmail-mcp/src/index.ts
- **Verification:** TypeScript compiles cleanly after fix
- **Committed in:** 05cd425 (Task 1 commit)

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** Type-safety fix required by strict TypeScript mode. No scope creep.

## Issues Encountered
None beyond the TypeScript type error documented above.

## User Setup Required
None - no external service configuration required. Users configure Gmail mailboxes via `eclaw set-gmail` at runtime.

## Next Phase Readiness
- Gmail MCP server ready for use with PicoClaw instances
- Users need Gmail App Passwords (not regular passwords) for IMAP access

---
*Quick Task: 2*
*Completed: 2026-03-21*
