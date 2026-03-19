package server

import (
	"context"
	"io"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	emberclaw "github.com/tuomas-lb/ember-claw/gen/emberclaw/v1"
)

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

		// Always use stream.Context() so cancellation propagates (Pitfall 4).
		response, err := s.agent.ProcessDirect(stream.Context(), req.Message, sessionKey)
		if err != nil {
			return status.Errorf(codes.Internal, "agent error: %v", err)
		}

		if err := stream.Send(&emberclaw.ChatResponse{Text: response, Done: true}); err != nil {
			return err
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

	response, err := s.agent.ProcessDirect(ctx, req.Message, sessionKey)
	if err != nil {
		return &emberclaw.QueryResponse{Error: err.Error()}, nil
	}
	return &emberclaw.QueryResponse{Text: response}, nil
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
