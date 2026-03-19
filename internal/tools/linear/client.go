// Package linear provides a GraphQL client for Linear issue management.
package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const apiURL = "https://api.linear.app/graphql"

// Client wraps the Linear GraphQL API.
type Client struct {
	apiKey string
	http   *http.Client
}

// Issue represents a Linear issue.
type Issue struct {
	ID          string `json:"id"`
	Identifier  string `json:"identifier"` // e.g. "EMP-15"
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
	State       struct {
		Name string `json:"name"`
	} `json:"state"`
	Priority int `json:"priority"`
}

// NewClient creates a Linear API client.
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		http:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) graphql(ctx context.Context, query string, variables map[string]any) (json.RawMessage, error) {
	body, _ := json.Marshal(map[string]any{
		"query":     query,
		"variables": variables,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("linear request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("linear API error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("linear parse error: %w", err)
	}
	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("linear error: %s", result.Errors[0].Message)
	}
	return result.Data, nil
}

// CreateIssue creates a new issue and returns its URL.
func (c *Client) CreateIssue(ctx context.Context, teamID, title, description string) (string, error) {
	query := `mutation($input: IssueCreateInput!) {
		issueCreate(input: $input) {
			success
			issue { id url identifier }
		}
	}`
	vars := map[string]any{
		"input": map[string]any{
			"teamId":      teamID,
			"title":       title,
			"description": description,
		},
	}

	data, err := c.graphql(ctx, query, vars)
	if err != nil {
		return "", err
	}

	var resp struct {
		IssueCreate struct {
			Success bool
			Issue   struct {
				URL        string `json:"url"`
				Identifier string `json:"identifier"`
			}
		}
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}
	if !resp.IssueCreate.Success {
		return "", fmt.Errorf("issue creation failed")
	}
	return resp.IssueCreate.Issue.URL, nil
}

// SearchIssues returns recent issues from a team.
func (c *Client) SearchIssues(ctx context.Context, teamID string, limit int) ([]Issue, error) {
	query := `query($filter: IssueFilter, $first: Int!) {
		issues(filter: $filter, first: $first, orderBy: updatedAt) {
			nodes {
				id identifier title description url
				state { name }
				priority
			}
		}
	}`
	vars := map[string]any{
		"filter": map[string]any{
			"team": map[string]any{"id": map[string]any{"eq": teamID}},
		},
		"first": limit,
	}

	data, err := c.graphql(ctx, query, vars)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Issues struct {
			Nodes []Issue `json:"nodes"`
		}
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	return resp.Issues.Nodes, nil
}

// GetIssue retrieves a single issue by identifier (e.g. "EMP-15").
func (c *Client) GetIssue(ctx context.Context, identifier string) (*Issue, error) {
	query := `query($filter: IssueFilter) {
		issues(filter: $filter, first: 1) {
			nodes {
				id identifier title description url
				state { name }
				priority
			}
		}
	}`
	vars := map[string]any{
		"filter": map[string]any{
			"identifier": map[string]any{"eq": identifier},
		},
	}

	data, err := c.graphql(ctx, query, vars)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Issues struct {
			Nodes []Issue `json:"nodes"`
		}
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	if len(resp.Issues.Nodes) == 0 {
		return nil, fmt.Errorf("issue %q not found", identifier)
	}
	return &resp.Issues.Nodes[0], nil
}

// UpdateIssueInput holds fields to update on an issue.
type UpdateIssueInput struct {
	StateID    string
	Priority   *int
	AssigneeID string
}

// UpdateIssue modifies an existing issue and returns its URL.
func (c *Client) UpdateIssue(ctx context.Context, issueID string, input UpdateIssueInput) (string, error) {
	fields := map[string]any{}
	if input.StateID != "" {
		fields["stateId"] = input.StateID
	}
	if input.Priority != nil {
		fields["priority"] = *input.Priority
	}
	if input.AssigneeID != "" {
		fields["assigneeId"] = input.AssigneeID
	}

	query := `mutation($id: String!, $input: IssueUpdateInput!) {
		issueUpdate(id: $id, input: $input) {
			success
			issue { url }
		}
	}`
	vars := map[string]any{
		"id":    issueID,
		"input": fields,
	}

	data, err := c.graphql(ctx, query, vars)
	if err != nil {
		return "", err
	}

	var resp struct {
		IssueUpdate struct {
			Success bool
			Issue   struct{ URL string }
		}
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}
	return resp.IssueUpdate.Issue.URL, nil
}

// FindStateID resolves a workflow state name to its ID.
func (c *Client) FindStateID(ctx context.Context, teamID, stateName string) (string, error) {
	query := `query($teamID: ID!) {
		team(id: $teamID) {
			states { nodes { id name } }
		}
	}`
	data, err := c.graphql(ctx, query, map[string]any{"teamID": teamID})
	if err != nil {
		return "", err
	}

	var resp struct {
		Team struct {
			States struct {
				Nodes []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"nodes"`
			}
		}
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}
	for _, s := range resp.Team.States.Nodes {
		if s.Name == stateName {
			return s.ID, nil
		}
	}
	return "", fmt.Errorf("state %q not found in team", stateName)
}
