package cli

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderEnvKey(t *testing.T) {
	tests := []struct {
		provider string
		want     string
	}{
		{"openai", "OPENAI_API_KEY"},
		{"gemini", "GEMINI_API_KEY"},
		{"anthropic", "ANTHROPIC_API_KEY"},
		{"groq", "GROQ_API_KEY"},
		{"deepseek", "DEEPSEEK_API_KEY"},
		{"OpenAI", "OPENAI_API_KEY"},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			assert.Equal(t, tt.want, providerEnvKey(tt.provider))
		})
	}
}

func TestResolveAPIKey_FlagTakesPrecedence(t *testing.T) {
	os.Setenv("OPENAI_API_KEY", "from-env")
	t.Cleanup(func() { os.Unsetenv("OPENAI_API_KEY") })

	key, err := resolveAPIKey("from-flag", "openai")
	require.NoError(t, err)
	assert.Equal(t, "from-flag", key)
}

func TestResolveAPIKey_FallsBackToEnv(t *testing.T) {
	os.Setenv("GEMINI_API_KEY", "AIza-test")
	t.Cleanup(func() { os.Unsetenv("GEMINI_API_KEY") })

	key, err := resolveAPIKey("", "gemini")
	require.NoError(t, err)
	assert.Equal(t, "AIza-test", key)
}

func TestResolveAPIKey_ErrorWhenBothEmpty(t *testing.T) {
	os.Unsetenv("ANTHROPIC_API_KEY")

	_, err := resolveAPIKey("", "anthropic")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ANTHROPIC_API_KEY")
	assert.Contains(t, err.Error(), "--api-key")
}
