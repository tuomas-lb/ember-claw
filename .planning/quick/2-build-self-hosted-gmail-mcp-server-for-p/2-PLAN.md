---
phase: quick
plan: 2
type: execute
wave: 1
depends_on: []
files_modified:
  - tools/gmail-mcp/package.json
  - tools/gmail-mcp/tsconfig.json
  - tools/gmail-mcp/src/index.ts
  - internal/cli/gmail.go
  - internal/cli/root.go
autonomous: true
requirements: [GMAIL-MCP]

must_haves:
  truths:
    - "Gmail MCP server starts via stdio and responds to MCP tool calls"
    - "Server connects to Gmail via IMAP with app passwords and lists/searches/reads emails"
    - "eclaw set-gmail command patches instance config.json to register the gmail MCP server"
  artifacts:
    - path: "tools/gmail-mcp/package.json"
      provides: "Node.js project with @modelcontextprotocol/sdk and imapflow deps"
    - path: "tools/gmail-mcp/src/index.ts"
      provides: "MCP server with 6 gmail tools (list_mailboxes, list_folders, search, read, list_recent, count_unread)"
      min_lines: 150
    - path: "internal/cli/gmail.go"
      provides: "set-gmail CLI command following set-caldav pattern"
  key_links:
    - from: "tools/gmail-mcp/src/index.ts"
      to: "imapflow"
      via: "IMAP connection per mailbox"
      pattern: "new ImapFlow"
    - from: "internal/cli/gmail.go"
      to: "k8sClient.PullConfig/PushConfig"
      via: "config patching"
      pattern: "k8sClient\\.PullConfig"
---

<objective>
Build a self-hosted Gmail MCP server for PicoClaw that connects to multiple Gmail mailboxes via IMAP + App Passwords, plus the eclaw CLI command to configure it on instances.

Purpose: Give PicoClaw instances read-only access to Gmail mailboxes without external MCP providers.
Output: TypeScript MCP server in tools/gmail-mcp/ and Go CLI command `eclaw set-gmail`.
</objective>

<execution_context>
@/Users/tuomas/.claude/get-shit-done/workflows/execute-plan.md
@/Users/tuomas/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/STATE.md
@internal/cli/caldav.go (pattern reference for set-gmail command)
@internal/cli/root.go (where to register new command)

<interfaces>
<!-- From internal/cli/caldav.go - pattern for config patching commands -->
Key pattern: Pull config -> parse JSON -> navigate to tools.mcp.servers -> add/remove server entry -> push config

From internal/cli/root.go:
```go
// Registration pattern - add to root.AddCommand() list
root.AddCommand(
    newSetCalDAVCommand(),
    // add: newSetGmailCommand(),
)
```

From internal/k8s (used via package-level k8sClient):
```go
k8sClient.PullConfig(ctx, instanceName) ([]byte, error)
k8sClient.PushConfig(ctx, instanceName, configJSON []byte) error
```
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: Create Gmail MCP server (TypeScript)</name>
  <files>tools/gmail-mcp/package.json, tools/gmail-mcp/tsconfig.json, tools/gmail-mcp/src/index.ts</files>
  <action>
Create `tools/gmail-mcp/` directory with a complete TypeScript MCP server.

**package.json:**
- name: "gmail-mcp"
- type: "module"
- main: "dist/index.js"
- bin: { "gmail-mcp": "dist/index.js" }
- scripts: { "build": "tsc", "start": "node dist/index.js" }
- dependencies: @modelcontextprotocol/sdk (latest), imapflow (latest)
- devDependencies: typescript, @types/node

**tsconfig.json:**
- target: ES2022, module: Node16, moduleResolution: Node16
- outDir: ./dist, rootDir: ./src
- strict: true, esModuleInterop: true

**src/index.ts** - The MCP server implementing these tools:

1. **gmail_list_mailboxes** - No args. Returns array of configured mailbox names and emails.

2. **gmail_list_folders** - Args: { mailbox: string }. Connects to the specified mailbox via IMAP, lists all folders/labels (INBOX, [Gmail]/Sent Mail, etc.), returns folder names with message counts.

3. **gmail_search** - Args: { mailbox?: string, from?: string, subject?: string, since?: string (ISO date), before?: string (ISO date), body?: string, limit?: number (default 20) }. If mailbox omitted, searches all mailboxes. Uses IMAP SEARCH command with criteria. Returns list of { uid, from, subject, date, snippet } objects.

4. **gmail_read** - Args: { mailbox: string, folder?: string (default "INBOX"), uid: number }. Fetches full email by UID. Returns { from, to, cc, subject, date, body_text, body_html }. Prefer text/plain body, fall back to text/html. Parse MIME properly via imapflow's download() or fetch with bodyStructure.

5. **gmail_list_recent** - Args: { mailbox: string, folder?: string (default "INBOX"), limit?: number (default 10) }. Lists most recent N emails from folder. Returns { uid, from, subject, date, flags } per message.

6. **gmail_count_unread** - Args: { mailbox?: string }. If mailbox omitted, counts across all. Uses IMAP STATUS command for UNSEEN count. Returns { mailbox, folder, unread } per folder (at minimum INBOX).

**Config loading:** Read GMAIL_MCP_CONFIG env var. Parse as JSON with schema:
```typescript
interface GmailConfig {
  mailboxes: Array<{
    name: string;
    email: string;
    password: string;
    host?: string;  // default: imap.gmail.com
    port?: number;  // default: 993
  }>;
}
```

**Connection management:** Create ImapFlow client per mailbox on-demand (connect when tool is called, disconnect after). Use TLS (secure: true). Set logger to false to suppress imapflow debug output to stderr (which would corrupt stdio MCP transport).

**MCP server setup:** Use StdioServerTransport from @modelcontextprotocol/sdk. Register all 6 tools with proper JSON Schema input definitions. Add #!/usr/bin/env node shebang to index.ts.

**Error handling:** If a mailbox name is not found in config, return a clear error message. If IMAP connection fails, return error with mailbox name and reason. Never crash the server on individual tool errors.
  </action>
  <verify>
    <automated>cd /Users/tuomas/Projects/ember-claw/tools/gmail-mcp && npm install && npm run build 2>&1 | tail -5</automated>
  </verify>
  <done>TypeScript compiles without errors. dist/index.js exists with shebang. All 6 tools registered in the MCP server. Config parsing handles defaults for host/port.</done>
</task>

<task type="auto">
  <name>Task 2: Add eclaw set-gmail CLI command</name>
  <files>internal/cli/gmail.go, internal/cli/root.go</files>
  <action>
Create `internal/cli/gmail.go` following the exact pattern from caldav.go.

**Command:** `eclaw set-gmail <instance>`

**Flags:**
- --name (required): Mailbox name (e.g., "work", "personal") - used as part of MCP server key
- --email: Gmail address
- --password: Gmail app password (16 char)
- --remove: Remove the named mailbox config instead of adding

**Behavior (add mode):**
1. Pull config via k8sClient.PullConfig(ctx, instance)
2. Parse JSON, navigate to tools.mcp.servers (create path if missing, same as caldav.go)
3. Server key: "gmail" (single server, NOT per-mailbox like caldav)
4. If "gmail" server entry already exists, read its existing env.GMAIL_MCP_CONFIG, parse the JSON, add/update the mailbox entry by name
5. If "gmail" server entry does not exist, create it:
   ```go
   servers["gmail"] = map[string]interface{}{
       "enabled": true,
       "type":    "stdio",
       "command": "npx",
       "args":    []string{"gmail-mcp"},
       "env": map[string]string{
           "GMAIL_MCP_CONFIG": configJSON,
       },
   }
   ```
6. The GMAIL_MCP_CONFIG value is a JSON string containing the mailboxes array
7. Push updated config, print success with green checkmark

**Behavior (remove mode):**
1. Pull config, find "gmail" server entry
2. Parse GMAIL_MCP_CONFIG env, remove the mailbox with matching name
3. If no mailboxes remain, delete the entire "gmail" server entry
4. Push config

**Register in root.go:** Add `newSetGmailCommand()` to the AddCommand list.

**Validation:** --name always required. In add mode, --email and --password also required.
  </action>
  <verify>
    <automated>cd /Users/tuomas/Projects/ember-claw && go build ./cmd/eclaw/ && ./bin/eclaw set-gmail --help 2>&1 | head -20</automated>
  </verify>
  <done>`eclaw set-gmail --help` shows usage with --name, --email, --password, --remove flags. `go vet ./internal/cli/` passes clean.</done>
</task>

</tasks>

<verification>
- `cd tools/gmail-mcp && npm run build` compiles without errors
- `cd . && go build ./cmd/eclaw/` compiles without errors
- `./bin/eclaw set-gmail --help` displays correct usage
- `node tools/gmail-mcp/dist/index.js` starts without crash (will exit when stdin closes, which is expected for stdio transport)
</verification>

<success_criteria>
- Gmail MCP server exists at tools/gmail-mcp/ with all 6 read-only tools implemented
- TypeScript compiles cleanly and produces dist/index.js
- eclaw set-gmail command registered and functional (patches config.json with gmail MCP server entry)
- Multi-mailbox config supported via GMAIL_MCP_CONFIG JSON env var
</success_criteria>

<output>
After completion, create `.planning/quick/2-build-self-hosted-gmail-mcp-server-for-p/2-SUMMARY.md`
</output>
