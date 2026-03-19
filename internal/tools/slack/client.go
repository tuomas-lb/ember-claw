// Package slack provides an HTTP client for Slack messaging.
package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const apiBase = "https://slack.com/api"

// Client wraps the Slack Web API.
type Client struct {
	botToken string
	http     *http.Client
}

// Channel represents a Slack channel.
type Channel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// NewClient creates a Slack API client.
func NewClient(botToken string) *Client {
	return &Client{
		botToken: botToken,
		http:     &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) post(ctx context.Context, method string, payload any) (json.RawMessage, error) {
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", apiBase+"/"+method, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+c.botToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("slack request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result struct {
		OK    bool            `json:"ok"`
		Error string          `json:"error"`
		Data  json.RawMessage `json:"-"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("slack parse error: %w", err)
	}
	if !result.OK {
		return nil, fmt.Errorf("slack error: %s", result.Error)
	}
	return respBody, nil
}

// SendMessage sends a message to a Slack channel. If threadTS is non-empty, replies in that thread.
func (c *Client) SendMessage(ctx context.Context, channel, text, threadTS string) error {
	payload := map[string]any{
		"channel": channel,
		"text":    text,
	}
	if threadTS != "" {
		payload["thread_ts"] = threadTS
	}

	_, err := c.post(ctx, "chat.postMessage", payload)
	return err
}

// ListChannels returns public channels the bot is a member of.
func (c *Client) ListChannels(ctx context.Context) ([]Channel, error) {
	payload := map[string]any{
		"types":            "public_channel",
		"exclude_archived": true,
		"limit":            200,
	}

	body, err := c.post(ctx, "conversations.list", payload)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Channels []Channel `json:"channels"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return resp.Channels, nil
}
