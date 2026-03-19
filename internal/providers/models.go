package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Model represents a model available from a provider.
type Model struct {
	ID          string
	DisplayName string
}

// ListModels queries the provider's API to list available models.
// This also serves as an API key validation — an invalid key returns an error.
func ListModels(ctx context.Context, provider, apiKey string) ([]Model, error) {
	switch strings.ToLower(provider) {
	case "openai":
		return listOpenAIModels(ctx, "https://api.openai.com/v1", apiKey)
	case "gemini", "google":
		return listGeminiModels(ctx, apiKey)
	case "anthropic":
		return listAnthropicModels(ctx, apiKey)
	case "groq":
		return listOpenAIModels(ctx, "https://api.groq.com/openai/v1", apiKey)
	case "deepseek":
		return listOpenAIModels(ctx, "https://api.deepseek.com/v1", apiKey)
	case "openrouter":
		return listOpenAIModels(ctx, "https://openrouter.ai/api/v1", apiKey)
	default:
		return nil, fmt.Errorf("model listing not supported for provider %q (supported: openai, gemini, anthropic, groq, deepseek, openrouter)", provider)
	}
}

// listOpenAIModels hits the OpenAI-compatible /v1/models endpoint.
func listOpenAIModels(ctx context.Context, baseURL, apiKey string) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, fmt.Errorf("authentication failed (HTTP %d): check your API key", resp.StatusCode)
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []struct {
			ID      string `json:"id"`
			Created int64  `json:"created"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	models := make([]Model, 0, len(result.Data))
	for _, m := range result.Data {
		models = append(models, Model{ID: m.ID, DisplayName: m.ID})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}

// listGeminiModels hits the Google Generative AI models endpoint.
func listGeminiModels(ctx context.Context, apiKey string) ([]Model, error) {
	url := "https://generativelanguage.googleapis.com/v1beta/models?key=" + apiKey
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 400 || resp.StatusCode == 403 {
		return nil, fmt.Errorf("authentication failed (HTTP %d): check your API key", resp.StatusCode)
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Models []struct {
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	models := make([]Model, 0, len(result.Models))
	for _, m := range result.Models {
		// Gemini returns "models/gemini-2.5-flash" — strip "models/" prefix
		id := strings.TrimPrefix(m.Name, "models/")
		models = append(models, Model{ID: id, DisplayName: m.DisplayName})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}

// listAnthropicModels validates the API key and returns known Claude models.
// Anthropic does not have a public model listing API.
func listAnthropicModels(ctx context.Context, apiKey string) ([]Model, error) {
	// Validate the API key by making a minimal request
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-20250514","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, fmt.Errorf("authentication failed (HTTP %d): check your API key", resp.StatusCode)
	}

	// Key is valid — return known models
	return []Model{
		{ID: "claude-opus-4-5-20250414", DisplayName: "Claude Opus 4.5"},
		{ID: "claude-sonnet-4-20250514", DisplayName: "Claude Sonnet 4"},
		{ID: "claude-sonnet-4-5-20250514", DisplayName: "Claude Sonnet 4.5"},
		{ID: "claude-haiku-3-5-20241022", DisplayName: "Claude Haiku 3.5"},
	}, nil
}

// Test hooks for URL injection.
var (
	listGeminiModelsFunc    = listGeminiModels
	listAnthropicModelsFunc = listAnthropicModels
)

// listGeminiModelsWithURL is the testable version that accepts a full URL.
func listGeminiModelsWithURL(ctx context.Context, url string) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 400 || resp.StatusCode == 403 {
		return nil, fmt.Errorf("authentication failed (HTTP %d): check your API key", resp.StatusCode)
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Models []struct {
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	models := make([]Model, 0, len(result.Models))
	for _, m := range result.Models {
		id := strings.TrimPrefix(m.Name, "models/")
		models = append(models, Model{ID: id, DisplayName: m.DisplayName})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}

// listAnthropicModelsWithURL is the testable version that accepts a full URL.
func listAnthropicModelsWithURL(ctx context.Context, url, apiKey string) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(`{"model":"claude-sonnet-4-20250514","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, fmt.Errorf("authentication failed (HTTP %d): check your API key", resp.StatusCode)
	}

	return []Model{
		{ID: "claude-opus-4-5-20250414", DisplayName: "Claude Opus 4.5"},
		{ID: "claude-sonnet-4-20250514", DisplayName: "Claude Sonnet 4"},
		{ID: "claude-sonnet-4-5-20250514", DisplayName: "Claude Sonnet 4.5"},
		{ID: "claude-haiku-3-5-20241022", DisplayName: "Claude Haiku 3.5"},
	}, nil
}

func httpClient() *http.Client {
	return &http.Client{Timeout: 15 * time.Second}
}
