package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tuomas-lb/ember-claw/internal/server"
)

func newControlUIServer(t *testing.T, token string) (*httptest.Server, *mockProcessor) {
	t.Helper()
	mock := newMockProcessor("hello from agent")
	s := server.New(mock)
	s.SetModel("test-model")
	s.SetProvider("test-provider")
	s.SetReady(true)

	mux := http.NewServeMux()
	s.RegisterControlUI(mux, token)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts, mock
}

func TestControlUI_ServesHTML(t *testing.T) {
	ts, _ := newControlUIServer(t, "secret")

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("content-type = %q, want text/html", ct)
	}
}

func TestControlUI_UnknownPathIs404(t *testing.T) {
	ts, _ := newControlUIServer(t, "secret")

	resp, err := http.Get(ts.URL + "/nope")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /nope status = %d, want 404", resp.StatusCode)
	}
}

func TestControlUI_APIRequiresToken(t *testing.T) {
	ts, _ := newControlUIServer(t, "secret")

	// No token.
	resp, err := http.Get(ts.URL + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no-token status = %d, want 401", resp.StatusCode)
	}

	// Wrong token.
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/status", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong-token status = %d, want 401", resp.StatusCode)
	}
}

func TestControlUI_APIDisabledWithoutConfiguredToken(t *testing.T) {
	ts, _ := newControlUIServer(t, "")

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/status", nil)
	req.Header.Set("Authorization", "Bearer anything")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 when CONTROL_TOKEN unset", resp.StatusCode)
	}
}

func TestControlUI_Status(t *testing.T) {
	ts, _ := newControlUIServer(t, "secret")

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/status", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body struct {
		Ready    bool   `json:"ready"`
		Model    string `json:"model"`
		Provider string `json:"provider"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.Ready || body.Model != "test-model" || body.Provider != "test-provider" {
		t.Fatalf("unexpected status body: %+v", body)
	}
}

func TestControlUI_Chat(t *testing.T) {
	ts, mock := newControlUIServer(t, "secret")

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/chat",
		strings.NewReader(`{"message":"hi there","session_id":"web:fixed"}`))
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("chat status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Response  string `json:"response"`
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Response != "hello from agent" {
		t.Fatalf("response = %q", body.Response)
	}
	if body.SessionID != "web:fixed" {
		t.Fatalf("session_id = %q, want web:fixed (client-provided keys are honored)", body.SessionID)
	}
	calls := mock.getCalls()
	if len(calls) != 1 || calls[0].content != "hi there" {
		t.Fatalf("agent calls = %+v", calls)
	}
}

func TestControlUI_ChatRejectsEmptyMessage(t *testing.T) {
	ts, _ := newControlUIServer(t, "secret")

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/chat", strings.NewReader(`{"message":"  "}`))
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}
