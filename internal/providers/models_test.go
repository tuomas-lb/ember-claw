package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListModels_UnsupportedProvider(t *testing.T) {
	_, err := ListModels(context.Background(), "unknown", "key123")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}

func TestListModels_ProviderRouting(t *testing.T) {
	// Verify the switch statement routes correctly by checking error for bad URLs
	// (we can't connect to real APIs in unit tests, but can verify routing)
	providers := []string{"openai", "gemini", "google", "anthropic", "groq", "deepseek", "openrouter"}
	for _, p := range providers {
		t.Run(p, func(t *testing.T) {
			// With a cancelled context, the HTTP call fails quickly
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			_, err := ListModels(ctx, p, "test-key")
			// Should get a context error, not "unsupported provider"
			assert.Error(t, err)
			assert.NotContains(t, err.Error(), "not supported")
		})
	}
}

func TestListOpenAIModels_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/models", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"id": "gpt-4o", "created": 1234},
				{"id": "gpt-3.5-turbo", "created": 1234},
				{"id": "gpt-4o-mini", "created": 1234},
			},
		})
	}))
	defer srv.Close()

	models, err := listOpenAIModels(context.Background(), srv.URL, "test-key")
	require.NoError(t, err)
	assert.Len(t, models, 3)

	// Should be sorted
	assert.Equal(t, "gpt-3.5-turbo", models[0].ID)
	assert.Equal(t, "gpt-4o", models[1].ID)
	assert.Equal(t, "gpt-4o-mini", models[2].ID)
}

func TestListOpenAIModels_AuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "invalid key"}`))
	}))
	defer srv.Close()

	_, err := listOpenAIModels(context.Background(), srv.URL, "bad-key")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}

func TestListOpenAIModels_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`server error`))
	}))
	defer srv.Close()

	_, err := listOpenAIModels(context.Background(), srv.URL, "key")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API error")
}

func TestListGeminiModels_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-key", r.URL.Query().Get("key"))

		json.NewEncoder(w).Encode(map[string]interface{}{
			"models": []map[string]interface{}{
				{"name": "models/gemini-2.5-flash", "displayName": "Gemini 2.5 Flash"},
				{"name": "models/gemini-2.0-flash", "displayName": "Gemini 2.0 Flash"},
			},
		})
	}))
	defer srv.Close()

	// Override the Gemini URL for testing
	origFunc := listGeminiModelsFunc
	listGeminiModelsFunc = func(ctx context.Context, apiKey string) ([]Model, error) {
		return listGeminiModelsWithURL(ctx, srv.URL+"/v1beta/models?key="+apiKey)
	}
	defer func() { listGeminiModelsFunc = origFunc }()

	models, err := listGeminiModelsFunc(context.Background(), "test-key")
	require.NoError(t, err)
	assert.Len(t, models, 2)
	// Should strip "models/" prefix
	assert.Equal(t, "gemini-2.0-flash", models[0].ID)
	assert.Equal(t, "gemini-2.5-flash", models[1].ID)
}

func TestListGeminiModels_AuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	origFunc := listGeminiModelsFunc
	listGeminiModelsFunc = func(ctx context.Context, apiKey string) ([]Model, error) {
		return listGeminiModelsWithURL(ctx, srv.URL+"/v1beta/models?key="+apiKey)
	}
	defer func() { listGeminiModelsFunc = origFunc }()

	_, err := listGeminiModelsFunc(context.Background(), "bad-key")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}

func TestListAnthropicModels_ReturnsHardcodedList(t *testing.T) {
	// Use a server that returns 200 (valid key)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-key", r.Header.Get("x-api-key"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"content":[{"text":"hi"}]}`))
	}))
	defer srv.Close()

	origFunc := listAnthropicModelsFunc
	listAnthropicModelsFunc = func(ctx context.Context, apiKey string) ([]Model, error) {
		return listAnthropicModelsWithURL(ctx, srv.URL+"/v1/messages", apiKey)
	}
	defer func() { listAnthropicModelsFunc = origFunc }()

	models, err := listAnthropicModelsFunc(context.Background(), "test-key")
	require.NoError(t, err)
	assert.True(t, len(models) >= 3, "should return at least 3 known Claude models")
	// Verify known models are present
	ids := make([]string, len(models))
	for i, m := range models {
		ids[i] = m.ID
	}
	assert.Contains(t, ids, "claude-sonnet-4-20250514")
}

func TestListAnthropicModels_AuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	origFunc := listAnthropicModelsFunc
	listAnthropicModelsFunc = func(ctx context.Context, apiKey string) ([]Model, error) {
		return listAnthropicModelsWithURL(ctx, srv.URL+"/v1/messages", apiKey)
	}
	defer func() { listAnthropicModelsFunc = origFunc }()

	_, err := listAnthropicModelsFunc(context.Background(), "bad-key")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}
