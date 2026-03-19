package linear

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sipeed/picoclaw/pkg/tools"
)

// NewCreateIssueTool returns a PicoClaw tool for creating Linear issues.
func NewCreateIssueTool(client *Client, teamID string) tools.Tool {
	return &createIssueTool{client: client, teamID: teamID}
}

type createIssueTool struct {
	client *Client
	teamID string
}

func (t *createIssueTool) Name() string { return "linear_create_issue" }

func (t *createIssueTool) Description() string {
	return "Create a new Linear issue/ticket. Returns the issue URL."
}

func (t *createIssueTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title":       map[string]any{"type": "string", "description": "Issue title"},
			"description": map[string]any{"type": "string", "description": "Issue description (supports markdown)"},
		},
		"required": []string{"title", "description"},
	}
}

func (t *createIssueTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	title, _ := args["title"].(string)
	description, _ := args["description"].(string)

	url, err := t.client.CreateIssue(ctx, t.teamID, title, description)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to create issue: %v", err))
	}
	return tools.NewToolResult(fmt.Sprintf("Issue created: %s", url))
}

// NewSearchIssuesTool returns a PicoClaw tool for searching Linear issues.
func NewSearchIssuesTool(client *Client, teamID string) tools.Tool {
	return &searchIssuesTool{client: client, teamID: teamID}
}

type searchIssuesTool struct {
	client *Client
	teamID string
}

func (t *searchIssuesTool) Name() string { return "linear_search_issues" }

func (t *searchIssuesTool) Description() string {
	return "Search for existing Linear issues in the team. Returns recent issues with their identifiers, titles, and status."
}

func (t *searchIssuesTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"limit": map[string]any{"type": "integer", "description": "Maximum number of issues to return (default 10, max 50)"},
		},
	}
}

func (t *searchIssuesTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	limit := 10
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
		if limit > 50 {
			limit = 50
		}
	}

	issues, err := t.client.SearchIssues(ctx, t.teamID, limit)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("search failed: %v", err))
	}

	if len(issues) == 0 {
		return tools.NewToolResult("No issues found.")
	}

	data, _ := json.MarshalIndent(issues, "", "  ")
	return tools.NewToolResult(string(data))
}

// NewGetIssueTool returns a PicoClaw tool for getting a single Linear issue.
func NewGetIssueTool(client *Client) tools.Tool {
	return &getIssueTool{client: client}
}

type getIssueTool struct {
	client *Client
}

func (t *getIssueTool) Name() string { return "linear_get_issue" }

func (t *getIssueTool) Description() string {
	return "Get a specific Linear issue by its identifier (e.g. EMP-15). Returns full issue details."
}

func (t *getIssueTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"identifier": map[string]any{"type": "string", "description": "Issue identifier (e.g. EMP-15)"},
		},
		"required": []string{"identifier"},
	}
}

func (t *getIssueTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	identifier, _ := args["identifier"].(string)

	issue, err := t.client.GetIssue(ctx, identifier)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("get issue failed: %v", err))
	}

	data, _ := json.MarshalIndent(issue, "", "  ")
	return tools.NewToolResult(string(data))
}

// NewUpdateIssueTool returns a PicoClaw tool for updating Linear issues.
func NewUpdateIssueTool(client *Client, teamID string) tools.Tool {
	return &updateIssueTool{client: client, teamID: teamID}
}

type updateIssueTool struct {
	client *Client
	teamID string
}

func (t *updateIssueTool) Name() string { return "linear_update_issue" }

func (t *updateIssueTool) Description() string {
	return "Update an existing Linear issue. Can change state (e.g. 'In Progress', 'Done'), priority (0-4), or assignee."
}

func (t *updateIssueTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"identifier": map[string]any{"type": "string", "description": "Issue identifier (e.g. EMP-15)"},
			"state":      map[string]any{"type": "string", "description": "New state name (e.g. 'In Progress', 'Done')"},
			"priority":   map[string]any{"type": "integer", "description": "Priority: 0=None, 1=Urgent, 2=High, 3=Medium, 4=Low"},
		},
		"required": []string{"identifier"},
	}
}

func (t *updateIssueTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	identifier, _ := args["identifier"].(string)

	// First get the issue to find its ID
	issue, err := t.client.GetIssue(ctx, identifier)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("issue not found: %v", err))
	}

	input := UpdateIssueInput{}

	if state, ok := args["state"].(string); ok && state != "" {
		stateID, err := t.client.FindStateID(ctx, t.teamID, state)
		if err != nil {
			return tools.ErrorResult(fmt.Sprintf("state not found: %v", err))
		}
		input.StateID = stateID
	}

	if priority, ok := args["priority"].(float64); ok {
		p := int(priority)
		input.Priority = &p
	}

	url, err := t.client.UpdateIssue(ctx, issue.ID, input)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("update failed: %v", err))
	}
	return tools.NewToolResult(fmt.Sprintf("Issue updated: %s", url))
}
