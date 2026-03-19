package slack

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sipeed/picoclaw/pkg/tools"
)

// NewSendMessageTool returns a PicoClaw tool for sending Slack messages.
func NewSendMessageTool(client *Client) tools.Tool {
	return &sendMessageTool{client: client}
}

type sendMessageTool struct {
	client *Client
}

func (t *sendMessageTool) Name() string { return "slack_send_message" }

func (t *sendMessageTool) Description() string {
	return "Send a message to a Slack channel. Can reply in a thread if thread_ts is provided."
}

func (t *sendMessageTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"channel":   map[string]any{"type": "string", "description": "Channel name (e.g. #general) or channel ID"},
			"text":      map[string]any{"type": "string", "description": "Message text (supports Slack markdown: *bold*, _italic_, `code`)"},
			"thread_ts": map[string]any{"type": "string", "description": "Thread timestamp to reply in (optional)"},
		},
		"required": []string{"channel", "text"},
	}
}

func (t *sendMessageTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	channel, _ := args["channel"].(string)
	text, _ := args["text"].(string)
	threadTS, _ := args["thread_ts"].(string)

	if err := t.client.SendMessage(ctx, channel, text, threadTS); err != nil {
		return tools.ErrorResult(fmt.Sprintf("send message failed: %v", err))
	}
	return tools.NewToolResult(fmt.Sprintf("Message sent to %s", channel))
}

// NewListChannelsTool returns a PicoClaw tool for listing Slack channels.
func NewListChannelsTool(client *Client) tools.Tool {
	return &listChannelsTool{client: client}
}

type listChannelsTool struct {
	client *Client
}

func (t *listChannelsTool) Name() string { return "slack_list_channels" }

func (t *listChannelsTool) Description() string {
	return "List Slack channels the bot has access to. Returns channel names and IDs."
}

func (t *listChannelsTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *listChannelsTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	channels, err := t.client.ListChannels(ctx)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("list channels failed: %v", err))
	}

	if len(channels) == 0 {
		return tools.NewToolResult("No channels found. The bot may need to be invited to channels.")
	}

	data, _ := json.MarshalIndent(channels, "", "  ")
	return tools.NewToolResult(string(data))
}
