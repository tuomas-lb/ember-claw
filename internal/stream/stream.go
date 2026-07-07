// Package stream adds live "processing" visibility to the agent's chat path.
//
// PicoClaw's AgentLoop.ProcessDirect is a single blocking call that only
// returns the final answer; v0.2.3 exposes no per-step callback for the
// gRPC/web session (reasoning is only routed to configured messaging
// "reasoning channels" via the shared outbound bus). To surface intermediate
// steps without patching PicoClaw, we wrap the LLM provider: the wrapper sees
// every LLMResponse the agent gets — including the model's reasoning and the
// tool calls it is about to make — and emits them as Steps through a per-request
// sink stashed in the context. The agent calls Provider.Chat with the same
// context that flows from ProcessDirect, so the sink correlates to exactly one
// chat turn even when sessions run concurrently.
package stream

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/sipeed/picoclaw/pkg/providers"
)

// Step is one intermediate processing event surfaced to the chat UI.
type Step struct {
	Kind    string `json:"kind"`              // "reasoning" | "tool"
	Tool    string `json:"tool,omitempty"`    // tool name (Kind == "tool")
	Content string `json:"content,omitempty"` // reasoning text or tool arguments
}

// Sink receives Steps as the agent produces them. Implementations must not
// block (the agent's request path calls this synchronously).
type Sink func(Step)

type ctxKey struct{}
type recorderKey struct{}

// WithSink returns a context carrying sink; the wrapped provider emits Steps to
// it for the duration of the call chain rooted at this context.
func WithSink(ctx context.Context, sink Sink) context.Context {
	if sink == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, sink)
}

// sinkFrom returns the Sink installed by WithSink, or nil.
func sinkFrom(ctx context.Context) Sink {
	s, _ := ctx.Value(ctxKey{}).(Sink)
	return s
}

// Recorder captures the model's most recent content and reasoning across a
// request's iterations. It exists to recover an answer when PicoClaw discards
// it: its final-response handling falls back to ReasoningContent but not
// Reasoning, so a turn where the model returns empty Content with the answer in
// Reasoning (common with DeepSeek/OpenRouter) yields the "no response to give"
// fallback even though the model did respond. Best() returns the recovered text.
type Recorder struct {
	mu            sync.Mutex
	lastContent   string
	lastReasoning string
}

func (r *Recorder) record(resp *providers.LLMResponse) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if resp.Content != "" {
		r.lastContent = resp.Content
	}
	if resp.Reasoning != "" {
		r.lastReasoning = resp.Reasoning
	} else if resp.ReasoningContent != "" {
		r.lastReasoning = resp.ReasoningContent
	}
}

// Best returns the best recovered text: the last non-empty content, else the
// last non-empty reasoning, else "".
func (r *Recorder) Best() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lastContent != "" {
		return r.lastContent
	}
	return r.lastReasoning
}

// WithRecorder returns a context carrying rec; the wrapped provider records each
// response into it.
func WithRecorder(ctx context.Context, rec *Recorder) context.Context {
	if rec == nil {
		return ctx
	}
	return context.WithValue(ctx, recorderKey{}, rec)
}

func recorderFrom(ctx context.Context) *Recorder {
	r, _ := ctx.Value(recorderKey{}).(*Recorder)
	return r
}

const maxArgPreview = 400

// WrapProvider decorates an LLMProvider so that, whenever a context carries a
// Sink, each LLM response's reasoning and tool-call intents are emitted as
// Steps. Providers without a Sink in context are unaffected.
func WrapProvider(p providers.LLMProvider) providers.LLMProvider {
	if p == nil {
		return nil
	}
	return &provider{inner: p}
}

type provider struct {
	inner providers.LLMProvider
}

func (p *provider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	options map[string]any,
) (*providers.LLMResponse, error) {
	resp, err := p.inner.Chat(ctx, messages, tools, model, options)
	if err == nil && resp != nil {
		if sink := sinkFrom(ctx); sink != nil {
			emit(sink, resp)
		}
		if rec := recorderFrom(ctx); rec != nil {
			rec.record(resp)
		}
	}
	return resp, err
}

func emit(sink Sink, resp *providers.LLMResponse) {
	if r := resp.Reasoning; r != "" {
		sink(Step{Kind: "reasoning", Content: r})
	} else if r := resp.ReasoningContent; r != "" {
		sink(Step{Kind: "reasoning", Content: r})
	}
	for _, tc := range resp.ToolCalls {
		// Tool calls arrive in one of two shapes depending on the provider:
		// the nested Function (raw OpenAI shape) or the flat Name/Arguments
		// that openai-compat providers (DeepSeek via OpenRouter) populate
		// directly. Handle both — the agent normalizes them only after the
		// provider returns, so at this layer either may be set.
		name := tc.Name
		var args string
		if tc.Function != nil {
			if name == "" {
				name = tc.Function.Name
			}
			args = tc.Function.Arguments
		}
		if args == "" && len(tc.Arguments) > 0 {
			if b, err := json.Marshal(tc.Arguments); err == nil {
				args = string(b)
			}
		}
		if name == "" {
			continue
		}
		if len(args) > maxArgPreview {
			args = args[:maxArgPreview] + "…"
		}
		sink(Step{Kind: "tool", Tool: name, Content: args})
	}
}

func (p *provider) GetDefaultModel() string { return p.inner.GetDefaultModel() }

// SupportsThinking forwards the optional ThinkingCapable behavior so the agent
// loop's `provider.(providers.ThinkingCapable)` assertion keeps working.
func (p *provider) SupportsThinking() bool {
	if tc, ok := p.inner.(providers.ThinkingCapable); ok {
		return tc.SupportsThinking()
	}
	return false
}

// Close forwards the optional StatefulProvider behavior (agentLoop.Close()).
func (p *provider) Close() {
	if c, ok := p.inner.(interface{ Close() }); ok {
		c.Close()
	}
}
