package api

import (
	"context"
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
// Each user message runs as an independent turn on its own gRPC Chat stream.
// The sidecar replies with Done=false frames carrying intermediate processing
// steps (reasoning, tool-call intents, JSON-encoded in the frame text) followed
// by one Done=true frame with the final answer. Steps are forwarded to the
// browser as {step:{...}} and are NOT persisted; only the final answer is.
//
// Crucially, both the turn and its persistence use a context DETACHED from the
// WebSocket (context.WithoutCancel): a turn completes and its messages are saved
// even if the user refreshes or navigates away mid-generation. Writes to the
// (possibly closed) browser connection are best-effort and never abort a turn.
func (h *Handler) HandleChat(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("chat ws upgrade for %s: %v", name, err)
		return
	}
	defer conn.Close()

	// Detached from the request: persistence and in-flight turns must survive
	// the WebSocket closing (a refresh or tab switch mid-turn).
	base := context.WithoutCancel(r.Context())

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return // Client disconnected; any in-flight turn finishes + persists below.
		}

		var cm config.ChatMessage
		if err := json.Unmarshal(msg, &cm); err != nil {
			sendChatError(conn, "invalid message format: "+err.Error())
			continue
		}

		h.runChatTurn(base, conn, name, cm)
	}
}

// runChatTurn processes one user message: persists it, opens a per-turn gRPC
// Chat stream, forwards processing steps + the final answer to the browser
// (best-effort), and persists the final answer. All persistence uses the
// detached base context so it succeeds regardless of the WebSocket's state.
func (h *Handler) runChatTurn(base context.Context, conn *websocket.Conn, name string, cm config.ChatMessage) {
	// Persist the user message first, so it survives even if the turn errors.
	if h.store != nil && cm.Message != "" {
		if _, err := h.store.AddMessage(base, name, cm.SessionKey, "user", cm.Message); err != nil {
			log.Printf("persist user message for %s: %v", name, err)
		}
	}

	turnCtx, cancel := context.WithCancel(base)
	defer cancel() // closes this turn's gRPC stream/connection

	grpcStream, err := h.grpc.ChatStream(turnCtx, name)
	if err != nil {
		sendChatError(conn, "chat stream error: "+err.Error())
		return
	}
	if err := grpcStream.Send(&pb.ChatRequest{Message: cm.Message, SessionKey: cm.SessionKey}); err != nil {
		sendChatError(conn, "send error: "+err.Error())
		return
	}

	// Accumulate the turn's processing steps so they can be persisted alongside
	// the answer and re-rendered on reload/back-track.
	var steps []config.ChatStep

	for {
		frame, err := grpcStream.Recv()
		if err != nil {
			sendChatError(conn, "chat recv error: "+err.Error())
			return
		}

		// Intermediate step frame: forward as {step:{...}}, do not persist.
		// Write failures are ignored — the browser may have gone away, but the
		// turn must still run to completion and persist.
		if !frame.GetDone() && frame.GetError() == "" && frame.GetText() != "" {
			var step config.ChatStep
			if json.Unmarshal([]byte(frame.GetText()), &step) == nil && step.Kind != "" {
				steps = append(steps, step)
				b, _ := json.Marshal(config.ChatResponse{Step: &step})
				_ = conn.WriteMessage(websocket.TextMessage, b)
			}
			continue
		}

		// Final frame (detached ctx for all writes). Persist the thinking steps
		// first (so they sort before the answer), then the answer.
		if h.store != nil {
			if len(steps) > 0 {
				if sb, err := json.Marshal(steps); err == nil {
					if _, err := h.store.AddMessage(base, name, cm.SessionKey, "thinking", string(sb)); err != nil {
						log.Printf("persist thinking for %s: %v", name, err)
					}
				}
			}
			if frame.GetError() == "" && frame.GetText() != "" {
				if _, err := h.store.AddMessage(base, name, cm.SessionKey, "agent", frame.GetText()); err != nil {
					log.Printf("persist agent message for %s: %v", name, err)
				}
			}
		}
		cr := config.ChatResponse{Text: frame.GetText(), Done: true, Error: frame.GetError()}
		b, _ := json.Marshal(cr)
		_ = conn.WriteMessage(websocket.TextMessage, b)
		return
	}
}

// sendChatError writes a terminal error ChatResponse to the WebSocket.
func sendChatError(conn *websocket.Conn, msg string) {
	cr := config.ChatResponse{Error: msg, Done: true}
	b, _ := json.Marshal(cr)
	_ = conn.WriteMessage(websocket.TextMessage, b)
}
