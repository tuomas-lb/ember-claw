package server

import "context"

// AgentProcessor abstracts PicoClaw's AgentLoop for testability.
// Production: *agent.AgentLoop satisfies this.
// Tests: inject a mock.
type AgentProcessor interface {
	ProcessDirect(ctx context.Context, content, sessionKey string) (string, error)
}
