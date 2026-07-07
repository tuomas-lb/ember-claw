package server

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	emberclaw "github.com/tuomas-lb/ember-claw/gen/emberclaw/v1"
	"github.com/tuomas-lb/ember-claw/internal/stream"
)

// noResponseFallbackMarker is a stable substring of PicoClaw's fallback message
// ("I've completed processing but have no response to give. Increase
// `max_tool_iterations`..."). PicoClaw returns it when the agent loop ends with
// empty final content — which also happens when the model puts its answer in
// the Reasoning field and leaves Content empty (PicoClaw only falls back to
// ReasoningContent, not Reasoning). We recover the model's actual output in
// that case rather than surfacing the dead-end message.
const noResponseFallbackMarker = "have no response to give"

// recoverEmptyResponse substitutes the recorded model output when PicoClaw
// returns its empty-response fallback.
func recoverEmptyResponse(text string, rec *stream.Recorder) string {
	if rec != nil && strings.Contains(text, noResponseFallbackMarker) {
		if alt := strings.TrimSpace(rec.Best()); alt != "" {
			return alt
		}
	}
	return text
}

// Server implements the PicoClawServiceServer gRPC interface.
// It wraps an AgentProcessor (production: *agent.AgentLoop; tests: mockProcessor).
type Server struct {
	emberclaw.UnimplementedPicoClawServiceServer
	agent     AgentProcessor
	startTime time.Time
	model     string
	provider  string
	ready     bool
}

// New creates a new Server with the given AgentProcessor.
func New(agent AgentProcessor) *Server {
	return &Server{
		agent:     agent,
		startTime: time.Now(),
	}
}

// SetModel sets the model name reported by the Status RPC.
func (s *Server) SetModel(model string) {
	s.model = model
}

// SetProvider sets the provider name reported by the Status RPC.
func (s *Server) SetProvider(provider string) {
	s.provider = provider
}

// SetReady sets the readiness state reported by the Status RPC.
func (s *Server) SetReady(ready bool) {
	s.ready = ready
}

// Chat implements the bidirectional streaming Chat RPC (GRPC-02, GRPC-05).
//
// A unique session key is assigned at stream start. If the first ChatRequest
// carries a non-empty SessionKey the server honors it (session resume).
// Each message is processed via ProcessDirect and a ChatResponse is streamed back.
func (s *Server) Chat(stream emberclaw.PicoClawService_ChatServer) error {
	// Assign a unique session key for this stream.
	// The client may override it on any message (see session resume logic below).
	sessionKey := assignSessionKey("", "grpc")

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			// Client finished sending; stream closes cleanly.
			return nil
		}
		if err != nil {
			return status.Errorf(codes.Canceled, "stream closed: %v", err)
		}

		// Honor client-provided session key (e.g. for session resume).
		if req.SessionKey != "" {
			sessionKey = req.SessionKey
		}

		if err := s.processStreaming(stream, req.Message, sessionKey); err != nil {
			return err
		}
	}
}

// processStreaming runs one chat turn, streaming intermediate processing steps
// (reasoning, tool-call intents) as ChatResponse frames with Done=false, then a
// final frame with Done=true carrying the answer (or Error).
//
// Steps are emitted by the stream-wrapped provider (see internal/stream) via a
// sink installed on the request context. The agent runs in a goroutine so we can
// forward buffered steps as they arrive; a full buffer drops steps rather than
// blocking the agent (they are best-effort UI hints, not the answer).
func (s *Server) processStreaming(srv emberclaw.PicoClawService_ChatServer, message, sessionKey string) error {
	steps := make(chan stream.Step, 64)
	rec := &stream.Recorder{}
	// Always use stream.Context() so cancellation propagates (Pitfall 4).
	ctx := stream.WithRecorder(stream.WithSink(srv.Context(), func(st stream.Step) {
		select {
		case steps <- st:
		default: // buffer full — drop this step
		}
	}), rec)

	type result struct {
		text string
		err  error
	}
	resCh := make(chan result, 1)
	go func() {
		text, err := s.agent.ProcessDirect(ctx, message, sessionKey)
		resCh <- result{text: text, err: err}
	}()

	sendStep := func(st stream.Step) error {
		b, err := json.Marshal(st)
		if err != nil {
			return nil // skip unencodable step
		}
		return srv.Send(&emberclaw.ChatResponse{Text: string(b), Done: false})
	}

	for {
		select {
		case st := <-steps:
			if err := sendStep(st); err != nil {
				return err
			}
		case res := <-resCh:
			// Flush any steps buffered before the result landed.
			for {
				select {
				case st := <-steps:
					if err := sendStep(st); err != nil {
						return err
					}
					continue
				default:
				}
				break
			}
			if res.err != nil {
				return status.Errorf(codes.Internal, "agent error: %v", res.err)
			}
			return srv.Send(&emberclaw.ChatResponse{Text: recoverEmptyResponse(res.text, rec), Done: true})
		}
	}
}

// Query implements the unary Query RPC (GRPC-03).
//
// A one-time session key is generated unless the request provides one.
// Errors from the agent are returned in QueryResponse.Error (not as gRPC errors)
// so the client always receives a structured response.
func (s *Server) Query(ctx context.Context, req *emberclaw.QueryRequest) (*emberclaw.QueryResponse, error) {
	sessionKey := assignSessionKey(req.SessionKey, "query")

	rec := &stream.Recorder{}
	response, err := s.agent.ProcessDirect(stream.WithRecorder(ctx, rec), req.Message, sessionKey)
	if err != nil {
		return &emberclaw.QueryResponse{Error: err.Error()}, nil
	}
	return &emberclaw.QueryResponse{Text: recoverEmptyResponse(response, rec)}, nil
}

// Status implements the unary Status RPC.
//
// Returns readiness, model/provider names, and uptime in seconds.
func (s *Server) Status(ctx context.Context, req *emberclaw.StatusRequest) (*emberclaw.StatusResponse, error) {
	return &emberclaw.StatusResponse{
		Ready:         s.ready,
		Model:         s.model,
		Provider:      s.provider,
		UptimeSeconds: int64(time.Since(s.startTime).Seconds()),
	}, nil
}
