package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/tuomas-lb/ember-claw/dashboard/internal/config"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins; tighten for production if needed.
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
}

// HandleLogs handles GET /api/instances/{name}/logs as a WebSocket.
// Query parameters:
//   - follow=true  (stream continuously, default false)
//   - tail=N       (number of historical lines to send first, default 100)
func (h *Handler) HandleLogs(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	follow := r.URL.Query().Get("follow") == "true"
	tailLines := int64(100)
	if t := r.URL.Query().Get("tail"); t != "" {
		if n, err := strconv.ParseInt(t, 10, 64); err == nil && n >= 0 {
			tailLines = n
		}
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("logs ws upgrade for %s: %v", name, err)
		return
	}
	defer conn.Close()

	// Use the request context so that closing the WS cancels log streaming.
	ctx := r.Context()

	scanner, closer, err := h.k8s.StreamLogsScanner(ctx, name, follow, tailLines)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("error: "+err.Error()))
		return
	}
	defer closer.Close()

	// Pump log lines to the WebSocket in a goroutine; stop when context is done
	// or the connection is closed.
	done := make(chan struct{})

	// Read loop — detect client disconnect
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	for scanner.Scan() {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		default:
		}

		line := scanner.Text()
		if err := conn.WriteMessage(websocket.TextMessage, []byte(line)); err != nil {
			return
		}
	}
}

// HandleChat handles GET /api/instances/{name}/chat as a WebSocket.
// Uses the simpler Query RPC (one message → one response) per user message,
// which matches how eclaw chat works in single-shot mode.
func (h *Handler) HandleChat(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("chat ws upgrade for %s: %v", name, err)
		return
	}
	defer conn.Close()

	ctx := r.Context()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return // Client disconnected
		}

		var cm config.ChatMessage
		if err := json.Unmarshal(msg, &cm); err != nil {
			sendChatError(conn, "invalid message format: "+err.Error())
			continue
		}

		// Persist the user message before calling the agent, so history is
		// captured even if the agent errors or the connection drops.
		if h.store != nil && cm.Message != "" {
			if _, err := h.store.AddMessage(ctx, name, cm.SessionKey, "user", cm.Message); err != nil {
				log.Printf("persist user message for %s: %v", name, err)
			}
		}

		// Use Query RPC — one request, one response (like eclaw chat -m)
		resp, err := h.grpc.Query(ctx, name, cm.Message, cm.SessionKey)
		if err != nil {
			sendChatError(conn, "query error: "+err.Error())
			continue
		}

		// Persist the agent response.
		if h.store != nil && resp.GetError() == "" && resp.GetText() != "" {
			if _, err := h.store.AddMessage(ctx, name, cm.SessionKey, "agent", resp.GetText()); err != nil {
				log.Printf("persist agent message for %s: %v", name, err)
			}
		}

		cr := config.ChatResponse{
			Text:  resp.GetText(),
			Done:  true,
			Error: resp.GetError(),
		}
		b, _ := json.Marshal(cr)
		if writeErr := conn.WriteMessage(websocket.TextMessage, b); writeErr != nil {
			return
		}
	}
}

// sendChatError writes a terminal error ChatResponse to the WebSocket.
func sendChatError(conn *websocket.Conn, msg string) {
	cr := config.ChatResponse{Error: msg, Done: true}
	b, _ := json.Marshal(cr)
	_ = conn.WriteMessage(websocket.TextMessage, b)
}
