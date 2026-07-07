package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	pb "github.com/tuomas-lb/ember-claw/dashboard/gen/emberclaw/v1"
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
//
// It opens one bidirectional gRPC Chat stream for the connection's lifetime and
// sends each user message on it. The sidecar replies with a series of Done=false
// frames carrying intermediate processing steps (reasoning, tool-call intents,
// JSON-encoded in the frame text) followed by one Done=true frame with the final
// answer. Steps are forwarded to the browser as {step:{...}} and are NOT
// persisted; only the final answer is stored.
func (h *Handler) HandleChat(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("chat ws upgrade for %s: %v", name, err)
		return
	}
	defer conn.Close()

	ctx := r.Context()

	grpcStream, err := h.grpc.ChatStream(ctx, name)
	if err != nil {
		sendChatError(conn, "chat stream error: "+err.Error())
		return
	}

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

		if err := grpcStream.Send(&pb.ChatRequest{Message: cm.Message, SessionKey: cm.SessionKey}); err != nil {
			sendChatError(conn, "send error: "+err.Error())
			return
		}

		// Read frames until the final (Done) frame for this turn.
		for {
			frame, err := grpcStream.Recv()
			if err != nil {
				sendChatError(conn, "chat recv error: "+err.Error())
				return
			}

			// Intermediate step frame: forward as {step:{...}}, do not persist.
			if !frame.GetDone() && frame.GetError() == "" && frame.GetText() != "" {
				var step config.ChatStep
				if json.Unmarshal([]byte(frame.GetText()), &step) == nil && step.Kind != "" {
					b, _ := json.Marshal(config.ChatResponse{Step: &step})
					if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
						return
					}
				}
				continue
			}

			// Final frame: persist the answer and forward it.
			if h.store != nil && frame.GetError() == "" && frame.GetText() != "" {
				if _, err := h.store.AddMessage(ctx, name, cm.SessionKey, "agent", frame.GetText()); err != nil {
					log.Printf("persist agent message for %s: %v", name, err)
				}
			}
			cr := config.ChatResponse{Text: frame.GetText(), Done: true, Error: frame.GetError()}
			b, _ := json.Marshal(cr)
			if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
				return
			}
			break
		}
	}
}

// sendChatError writes a terminal error ChatResponse to the WebSocket.
func sendChatError(conn *websocket.Conn, msg string) {
	cr := config.ChatResponse{Error: msg, Done: true}
	b, _ := json.Marshal(cr)
	_ = conn.WriteMessage(websocket.TextMessage, b)
}
