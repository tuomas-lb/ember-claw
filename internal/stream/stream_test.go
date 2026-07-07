package stream

import (
	"testing"

	"github.com/sipeed/picoclaw/pkg/providers"
)

func TestRecorderBest(t *testing.T) {
	tests := []struct {
		name  string
		feed  []*providers.LLMResponse
		want  string
	}{
		{
			name: "answer only in Reasoning (DeepSeek empty-content case)",
			feed: []*providers.LLMResponse{{Content: "", Reasoning: "the real answer"}},
			want: "the real answer",
		},
		{
			name: "content preferred over reasoning",
			feed: []*providers.LLMResponse{{Content: "final", Reasoning: "thinking"}},
			want: "final",
		},
		{
			name: "falls back to ReasoningContent",
			feed: []*providers.LLMResponse{{ReasoningContent: "rc answer"}},
			want: "rc answer",
		},
		{
			name: "last non-empty content wins across iterations",
			feed: []*providers.LLMResponse{{Content: "first"}, {Content: "", Reasoning: "r"}},
			want: "first",
		},
		{
			name: "nothing recorded",
			feed: []*providers.LLMResponse{{}},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Recorder{}
			for _, resp := range tt.feed {
				r.record(resp)
			}
			if got := r.Best(); got != tt.want {
				t.Fatalf("Best() = %q, want %q", got, tt.want)
			}
		})
	}
}
