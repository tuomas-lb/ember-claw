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

type Model struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

func httpClient() *http.Client {
	return &http.Client{Timeout: 15 * time.Second}
}

// ListModels queries the provider's API to list available models.
func ListModels(ctx context.Context, provider, apiKey string) ([]Model, error) {
	switch strings.ToLower(provider) {
	case "openai":
		return listOpenAICompatible(ctx, "https://api.openai.com/v1", apiKey)
	case "gemini", "google":
		return listGeminiModels(ctx, apiKey)
	case "anthropic":
		return listAnthropicModels(ctx, apiKey)
	case "groq":
		return listOpenAICompatible(ctx, "https://api.groq.com/openai/v1", apiKey)
	case "deepseek":
		return listOpenAICompatible(ctx, "https://api.deepseek.com/v1", apiKey)
	case "openrouter":
		return listOpenAICompatible(ctx, "https://openrouter.ai/api/v1", apiKey)
	case "copilot":
		return listOpenAICompatible(ctx, "https://api.githubcopilot.com", apiKey)
	case "mistral":
		return listMistralModels(ctx, apiKey)
	case "xai":
		return listOpenAICompatible(ctx, "https://api.x.ai/v1", apiKey)
	case "kimi", "moonshot":
		return listOpenAICompatible(ctx, "https://api.moonshot.cn/v1", apiKey)
	default:
		return nil, fmt.Errorf("unsupported provider %q", provider)
	}
}

func listOpenAICompatible(ctx context.Context, baseURL, apiKey string) ([]Model, error) {
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
		return nil, fmt.Errorf("authentication failed (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
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
		return nil, fmt.Errorf("authentication failed (HTTP %d)", resp.StatusCode)
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

func listMistralModels(ctx context.Context, apiKey string) ([]Model, error) {
	// Mistral standard API
	models, err := listOpenAICompatible(ctx, "https://api.mistral.ai/v1", apiKey)
	if err != nil {
		return nil, err
	}
	// Also try Codestral endpoint (may use same or different key)
	codestralModels, err := listOpenAICompatible(ctx, "https://codestral.mistral.ai/v1", apiKey)
	if err == nil {
		// Merge, dedup by ID
		seen := make(map[string]bool)
		for _, m := range models {
			seen[m.ID] = true
		}
		for _, m := range codestralModels {
			if !seen[m.ID] {
				models = append(models, m)
			}
		}
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}

func listAnthropicModels(_ context.Context, apiKey string) ([]Model, error) {
	// Anthropic doesn't have a models endpoint — validate key and return known models
	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages",
		strings.NewReader(`{"model":"claude-sonnet-4-20250514","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, fmt.Errorf("authentication failed (HTTP %d)", resp.StatusCode)
	}

	return []Model{
		{ID: "claude-opus-4-5-20250414", DisplayName: "Claude Opus 4.5"},
		{ID: "claude-sonnet-4-20250514", DisplayName: "Claude Sonnet 4"},
		{ID: "claude-sonnet-4-5-20250514", DisplayName: "Claude Sonnet 4.5"},
		{ID: "claude-haiku-3-5-20241022", DisplayName: "Claude Haiku 3.5"},
	}, nil
}
