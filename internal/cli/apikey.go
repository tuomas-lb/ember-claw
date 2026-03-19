package cli

import (
	"fmt"
	"os"
	"strings"
)

// providerEnvKey returns the environment variable name for a provider's API key.
// e.g. "openai" -> "OPENAI_API_KEY", "gemini" -> "GEMINI_API_KEY"
func providerEnvKey(provider string) string {
	return strings.ToUpper(provider) + "_API_KEY"
}

// resolveAPIKey returns the API key from the flag value, or falls back to the
// provider-specific env var (e.g. OPENAI_API_KEY, GEMINI_API_KEY, ANTHROPIC_API_KEY).
// Returns an error if neither is set.
func resolveAPIKey(flagValue, provider string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}

	envName := providerEnvKey(provider)
	if v := os.Getenv(envName); v != "" {
		return v, nil
	}

	return "", fmt.Errorf("API key required: use --api-key flag or set %s in environment/.env", envName)
}
