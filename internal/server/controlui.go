package server

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// RegisterControlUI mounts the web control interface on the given mux:
//
//	GET  /            — single-page control UI (status + chat)
//	GET  /api/status  — instance status JSON (auth required)
//	POST /api/chat    — send a message to the agent (auth required)
//
// API routes require `Authorization: Bearer <token>` matching controlToken.
// With an empty controlToken the API is disabled (503) — fail closed, since
// the instance may be exposed publicly and the agent has shell access.
func (s *Server) RegisterControlUI(mux *http.ServeMux, controlToken string) {
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(controlUIHTML))
	})
	mux.HandleFunc("/api/status", s.withAuth(controlToken, s.handleAPIStatus))
	mux.HandleFunc("/api/chat", s.withAuth(controlToken, s.handleAPIChat))
}

// withAuth wraps an API handler with bearer-token authentication.
func (s *Server) withAuth(token string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token == "" {
			writeJSONError(w, http.StatusServiceUnavailable, "control API disabled: CONTROL_TOKEN is not configured (set it with: eclaw set-secret <instance> CONTROL_TOKEN <token>)")
			return
		}
		auth := r.Header.Get("Authorization")
		presented, ok := strings.CutPrefix(auth, "Bearer ")
		if !ok || subtle.ConstantTimeCompare([]byte(presented), []byte(token)) != 1 {
			writeJSONError(w, http.StatusUnauthorized, "invalid or missing bearer token")
			return
		}
		next(w, r)
	}
}

// handleAPIStatus returns instance status as JSON.
func (s *Server) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ready":          s.ready,
		"model":          s.model,
		"provider":       s.provider,
		"uptime_seconds": int64(time.Since(s.startTime).Seconds()),
	})
}

// chatRequest is the POST /api/chat request body.
type chatRequest struct {
	Message   string `json:"message"`
	SessionID string `json:"session_id,omitempty"`
}

// handleAPIChat forwards a message to the agent and returns its response.
// Uses the same ProcessDirect path and session-key semantics as the gRPC API.
func (s *Server) handleAPIChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req chatRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeJSONError(w, http.StatusBadRequest, "message is required")
		return
	}

	sessionKey := assignSessionKey(req.SessionID, "web")
	response, err := s.agent.ProcessDirect(r.Context(), req.Message, sessionKey)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "agent error: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"response":   response,
		"session_id": sessionKey,
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
