// Package chat owns the live state of chat turns on the server side, so that
// "what is the agent doing right now" survives client disconnects and can be
// recovered by any browser (reload, second tab).
//
// Each (instance, session) has one sessionState. A turn runs in a detached
// goroutine driving one gRPC Chat stream to completion regardless of whether
// any client is connected; its processing steps are buffered and broadcast to
// all subscribers, and the user/thinking/answer rows are persisted. A client
// connecting mid-turn receives a snapshot (running flag + steps so far) and then
// live events — so it never loses track of an in-progress turn.
package chat

import (
	"context"
	"encoding/json"
	"log"
	"sync"

	pb "github.com/tuomas-lb/ember-claw/dashboard/gen/emberclaw/v1"
	"github.com/tuomas-lb/ember-claw/dashboard/internal/config"
	grpcclient "github.com/tuomas-lb/ember-claw/dashboard/internal/grpc"
	"github.com/tuomas-lb/ember-claw/dashboard/internal/store"
)

// Event is broadcast to subscribers and serialized to the browser WebSocket.
type Event struct {
	Type    string            `json:"type"`              // snapshot | status | step | done | error
	Running bool              `json:"running"`           // snapshot/status: is a turn in progress
	Message string            `json:"message,omitempty"` // snapshot: the running turn's user message
	Steps   []config.ChatStep `json:"steps,omitempty"`   // snapshot: steps accumulated so far
	Step    *config.ChatStep  `json:"step,omitempty"`    // step: one new processing step
	Text    string            `json:"text,omitempty"`    // done: the final answer
	Error   string            `json:"error,omitempty"`   // error: failure message
}

const subBuffer = 128

// Manager tracks live turn state per (instance, session).
type Manager struct {
	grpc  *grpcclient.Client
	store *store.Store // may be nil

	mu       sync.Mutex
	sessions map[string]*sessionState
}

type sessionState struct {
	mu      sync.Mutex
	running bool
	message string // current running turn's user message
	steps   []config.ChatStep
	queue   []string
	subs    map[chan Event]struct{}
}

// New creates a Manager. store may be nil (persistence disabled).
func New(grpcClient *grpcclient.Client, chatStore *store.Store) *Manager {
	return &Manager{grpc: grpcClient, store: chatStore, sessions: map[string]*sessionState{}}
}

func key(instance, session string) string { return instance + "|" + session }

func (m *Manager) get(instance, session string) *sessionState {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := key(instance, session)
	ss := m.sessions[k]
	if ss == nil {
		ss = &sessionState{subs: map[chan Event]struct{}{}}
		m.sessions[k] = ss
	}
	return ss
}

// Subscribe registers a subscriber and returns a snapshot of the current turn
// state plus a channel of future events and an unsubscribe func.
func (m *Manager) Subscribe(instance, session string) (Event, <-chan Event, func()) {
	ss := m.get(instance, session)
	ch := make(chan Event, subBuffer)

	ss.mu.Lock()
	snap := Event{Type: "snapshot", Running: ss.running}
	if ss.running {
		snap.Message = ss.message
		snap.Steps = append([]config.ChatStep(nil), ss.steps...)
	}
	ss.subs[ch] = struct{}{}
	ss.mu.Unlock()

	cancel := func() {
		ss.mu.Lock()
		delete(ss.subs, ch)
		ss.mu.Unlock()
	}
	return snap, ch, cancel
}

// broadcastLocked sends ev to all subscribers. Caller must hold ss.mu so that
// step accumulation and its broadcast are atomic with respect to Subscribe
// (preventing a new subscriber from getting a step both in its snapshot and as
// a live event).
func broadcastLocked(ss *sessionState, ev Event) {
	for ch := range ss.subs {
		select {
		case ch <- ev:
		default: // slow subscriber — it will recover via snapshot on reconnect
		}
	}
}

// Submit enqueues a user message for the session. Turns run one at a time per
// session (the agent processes a session serially); additional messages queue.
func (m *Manager) Submit(instance, session, message string) {
	ss := m.get(instance, session)
	ss.mu.Lock()
	ss.queue = append(ss.queue, message)
	start := !ss.running
	if start {
		ss.running = true
	}
	ss.mu.Unlock()
	if start {
		go m.worker(instance, session, ss)
	}
}

func (m *Manager) worker(instance, session string, ss *sessionState) {
	for {
		ss.mu.Lock()
		if len(ss.queue) == 0 {
			ss.running = false
			ss.message = ""
			ss.steps = nil
			broadcastLocked(ss, Event{Type: "status", Running: false})
			ss.mu.Unlock()
			return
		}
		msg := ss.queue[0]
		ss.queue = ss.queue[1:]
		ss.message = msg
		ss.steps = nil
		broadcastLocked(ss, Event{Type: "status", Running: true, Message: msg})
		ss.mu.Unlock()

		m.runTurn(instance, session, ss, msg)
	}
}

func (m *Manager) runTurn(instance, session string, ss *sessionState, message string) {
	// Detached context: the turn runs to completion (and persists) even if every
	// client disconnects. Cancelled when the turn finishes to close the stream.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if m.store != nil && message != "" {
		if _, err := m.store.AddMessage(ctx, instance, session, "user", message); err != nil {
			log.Printf("chat: persist user message (%s): %v", instance, err)
		}
	}

	stream, err := m.grpc.ChatStream(ctx, instance)
	if err != nil {
		m.emit(ss, Event{Type: "error", Error: err.Error()})
		return
	}
	if err := stream.Send(&pb.ChatRequest{Message: message, SessionKey: session}); err != nil {
		m.emit(ss, Event{Type: "error", Error: err.Error()})
		return
	}

	for {
		frame, err := stream.Recv()
		if err != nil {
			m.emit(ss, Event{Type: "error", Error: "chat stream: " + err.Error()})
			return
		}

		// Intermediate processing step.
		if !frame.GetDone() && frame.GetError() == "" && frame.GetText() != "" {
			var st config.ChatStep
			if json.Unmarshal([]byte(frame.GetText()), &st) == nil && st.Kind != "" {
				ss.mu.Lock()
				ss.steps = append(ss.steps, st)
				step := st
				broadcastLocked(ss, Event{Type: "step", Step: &step})
				ss.mu.Unlock()
			}
			continue
		}

		// Terminal frame: persist thinking + answer, then broadcast the result.
		steps := m.snapshotSteps(ss)
		if m.store != nil {
			if len(steps) > 0 {
				if b, mErr := json.Marshal(steps); mErr == nil {
					if _, err := m.store.AddMessage(ctx, instance, session, "thinking", string(b)); err != nil {
						log.Printf("chat: persist thinking (%s): %v", instance, err)
					}
				}
			}
			if frame.GetError() == "" && frame.GetText() != "" {
				if _, err := m.store.AddMessage(ctx, instance, session, "agent", frame.GetText()); err != nil {
					log.Printf("chat: persist agent message (%s): %v", instance, err)
				}
			}
		}
		if frame.GetError() != "" {
			m.emit(ss, Event{Type: "error", Error: frame.GetError()})
		} else {
			m.emit(ss, Event{Type: "done", Text: frame.GetText()})
		}
		return
	}
}

func (m *Manager) snapshotSteps(ss *sessionState) []config.ChatStep {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return append([]config.ChatStep(nil), ss.steps...)
}

func (m *Manager) emit(ss *sessionState, ev Event) {
	ss.mu.Lock()
	broadcastLocked(ss, ev)
	ss.mu.Unlock()
}
