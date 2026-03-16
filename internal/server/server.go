package server

import emberclaw "github.com/LastBotInc/ember-claw/gen/emberclaw/v1"

// Server is the gRPC service implementation.
// Stub -- full implementation in Plan 02.
// Embeds UnimplementedPicoClawServiceServer so the struct satisfies the interface
// and tests can compile. All methods return codes.Unimplemented until Plan 02.
type Server struct {
	emberclaw.UnimplementedPicoClawServiceServer
	agent AgentProcessor
}

// New creates a new Server with the given AgentProcessor.
func New(agent AgentProcessor) *Server {
	return &Server{agent: agent}
}
