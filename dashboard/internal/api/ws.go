package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/tuomas-lb/ember-claw/dashboard/internal/chat"
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

// HandleChat handles GET /api/instances/{name}/chat?session=<key> as a WebSocket.
//
// Turn state lives server-side in the chat.Manager, not in this connection: the
// client subscribes to its session and immediately receives a snapshot (whether
// a turn is running and the steps it has produced so far), then live events
// (step / done / error / status). User messages read off the socket are handed
// to the Manager, which runs each turn to completion and persists it regardless
// of whether any client stays connected. So a reload or second tab recovers the
// in-flight turn and its thinking, and always knows if the agent is working.
func (h *Handler) HandleChat(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	session := r.URL.Query().Get("session")

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("chat ws upgrade for %s: %v", name, err)
		return
	}
	defer conn.Close()

	if session == "" {
		writeEvent(conn, chat.Event{Type: "error", Error: "missing session"})
		return
	}

	snapshot, events, cancel := h.chat.Subscribe(name, session)
	defer cancel()

	// Single writer goroutine: snapshot first, then live events. Only this
	// goroutine writes to the connection.
	stop := make(chan struct{})
	go func() {
		if writeEvent(conn, snapshot) != nil {
			return
		}
		for {
			select {
			case <-stop:
				return
			case ev, ok := <-events:
				if !ok {
					return
				}
				if writeEvent(conn, ev) != nil {
					return
				}
			}
		}
	}()

	// Reader loop: user messages → Manager. Closing the socket only detaches
	// this viewer; the turn keeps running server-side.
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var cm config.ChatMessage
		if json.Unmarshal(msg, &cm) == nil && cm.Message != "" {
			h.chat.Submit(name, session, cm.Message)
		}
	}
	close(stop)
}

func writeEvent(conn *websocket.Conn, ev chat.Event) error {
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, b)
}
