# Tool Development Guide

How to add new tools to ember-claw PicoClaw instances.

## Architecture

```
internal/tools/<name>/
├── client.go     # API client (HTTP/GraphQL/etc.)
└── tools.go      # PicoClaw tool implementations
```

Each tool implements PicoClaw's `tools.Tool` interface from `github.com/sipeed/picoclaw/pkg/tools`. Tools are registered in the sidecar at startup and become available to the AI agent during conversations.

## Tool Interface

```go
import "github.com/sipeed/picoclaw/pkg/tools"

type Tool interface {
    Name() string                                           // Unique identifier (e.g. "linear_create_issue")
    Description() string                                    // What the tool does (shown to AI)
    Parameters() map[string]any                            // JSON Schema for input parameters
    Execute(ctx context.Context, args map[string]any) *tools.ToolResult
}
```

## Result Types

```go
tools.NewToolResult("response for AI")       // Normal result
tools.ErrorResult("what went wrong")         // Error (IsError=true)
tools.SilentResult("response for AI")        // No user-visible output
```

## Step-by-Step: Adding a New Tool

### 1. Create the package

```
mkdir -p internal/tools/myservice
```

### 2. Write the API client (`client.go`)

```go
package myservice

import (
    "context"
    "net/http"
    "time"
)

type Client struct {
    apiKey string
    http   *http.Client
}

func NewClient(apiKey string) *Client {
    return &Client{
        apiKey: apiKey,
        http:   &http.Client{Timeout: 15 * time.Second},
    }
}

func (c *Client) DoSomething(ctx context.Context, input string) (string, error) {
    // Call external API
    return "result", nil
}
```

### 3. Write the tool wrapper (`tools.go`)

```go
package myservice

import (
    "context"
    "fmt"
    "github.com/sipeed/picoclaw/pkg/tools"
)

func NewMyTool(client *Client) tools.Tool {
    return &myTool{client: client}
}

type myTool struct {
    client *Client
}

func (t *myTool) Name() string { return "myservice_do_something" }

func (t *myTool) Description() string {
    return "Does something useful via MyService API."
}

func (t *myTool) Parameters() map[string]any {
    return map[string]any{
        "type": "object",
        "properties": map[string]any{
            "input": map[string]any{
                "type":        "string",
                "description": "The input to process",
            },
        },
        "required": []string{"input"},
    }
}

func (t *myTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
    input, _ := args["input"].(string)

    result, err := t.client.DoSomething(ctx, input)
    if err != nil {
        return tools.ErrorResult(fmt.Sprintf("failed: %v", err))
    }
    return tools.NewToolResult(result)
}
```

### 4. Register in the sidecar (`cmd/sidecar/main.go`)

Add to the `registerTools` function:

```go
if apiKey := os.Getenv("MYSERVICE_API_KEY"); apiKey != "" {
    client := myservice.NewClient(apiKey)
    agentLoop.RegisterTool(myservice.NewMyTool(client))
    log.Info().Msg("myservice tools registered")
}
```

### 5. Add deploy flag (`internal/cli/deploy.go`)

Add a flag variable and wire it to `DeployOptions`:

```go
// In var block:
myserviceAPIKey string

// In flag registration:
cmd.Flags().StringVar(&myserviceAPIKey, "myservice-api-key", "", "MyService API key (or MYSERVICE_API_KEY env)")

// In DeployOptions construction:
MyServiceAPIKey: envDefault(myserviceAPIKey, "MYSERVICE_API_KEY"),
```

### 6. Add to Secret (`internal/k8s/resources.go`)

In `DeployOptions`, add the field:
```go
MyServiceAPIKey string
```

In `DeployInstance`, add to `secretData`:
```go
if opts.MyServiceAPIKey != "" {
    secretData["MYSERVICE_API_KEY"] = opts.MyServiceAPIKey
}
```

### 7. Write tests

Use `net/http/httptest` to mock the external API:

```go
func TestMyTool_Execute(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        json.NewEncoder(w).Encode(map[string]string{"result": "ok"})
    }))
    defer srv.Close()

    client := NewClientWithURL(srv.URL, "test-key")
    tool := NewMyTool(client)

    result := tool.Execute(context.Background(), map[string]any{"input": "test"})
    assert.False(t, result.IsError)
}
```

## Naming Conventions

- Tool names: `<service>_<action>` (e.g. `linear_create_issue`, `slack_send_message`)
- Package names: `internal/tools/<service>/`
- Env vars: `<SERVICE>_API_KEY` (e.g. `LINEAR_API_KEY`, `SLACK_BOT_TOKEN`)
- Deploy flags: `--<service>-api-key`

## Parameter Schema

Parameters use JSON Schema format. Common patterns:

```go
// Required string
"title": map[string]any{"type": "string", "description": "Issue title"}

// Optional integer with default
"limit": map[string]any{"type": "integer", "description": "Max results (default 10)"}

// Enum
"priority": map[string]any{"type": "integer", "description": "0=None, 1=Urgent, 2=High, 3=Medium, 4=Low"}
```

Mark required fields:
```go
"required": []string{"title", "description"}
```

## Configuration via .env

Users configure tool credentials in `.env`:

```bash
LINEAR_API_KEY=lin_api_...
LINEAR_TEAM_ID=<team-uuid>
SLACK_BOT_TOKEN=xoxb-...
MYSERVICE_API_KEY=...
```

The sidecar auto-loads these from the K8s Secret (injected as env vars). Tools only register when their env vars are present — no env var, no tool.

## Existing Tools

| Tool | Package | Env Vars |
|------|---------|----------|
| `linear_create_issue` | `internal/tools/linear` | `LINEAR_API_KEY`, `LINEAR_TEAM_ID` |
| `linear_search_issues` | `internal/tools/linear` | `LINEAR_API_KEY`, `LINEAR_TEAM_ID` |
| `linear_get_issue` | `internal/tools/linear` | `LINEAR_API_KEY` |
| `linear_update_issue` | `internal/tools/linear` | `LINEAR_API_KEY`, `LINEAR_TEAM_ID` |
| `slack_send_message` | `internal/tools/slack` | `SLACK_BOT_TOKEN` |
| `slack_list_channels` | `internal/tools/slack` | `SLACK_BOT_TOKEN` |

## Quick Reference: Adding a Tool via `/gsd:quick`

When using GSD to add a new tool, provide this context:

```
Add a new PicoClaw tool for <service>. Follow the pattern in docs/tool-development.md.
Files to create: internal/tools/<service>/client.go, internal/tools/<service>/tools.go
Files to modify: cmd/sidecar/main.go (registerTools), internal/cli/deploy.go, internal/k8s/resources.go
```
