package grpcclient

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	emberclaw "github.com/tuomas-lb/ember-claw/gen/emberclaw/v1"
)

// DialSidecar dials the eclaw sidecar gRPC server at localhost:{localPort} using
// insecure credentials (port-forwarded in-cluster traffic; TLS not required).
// It returns a PicoClawServiceClient, the underlying *grpc.ClientConn (for Close),
// and any error.
//
// NOTE: Uses grpc.NewClient (not deprecated grpc.Dial).
func DialSidecar(ctx context.Context, localPort uint16) (emberclaw.PicoClawServiceClient, *grpc.ClientConn, error) {
	target := fmt.Sprintf("localhost:%d", localPort)
	conn, err := grpc.NewClient(
		target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("grpc.NewClient(%q): %w", target, err)
	}
	return emberclaw.NewPicoClawServiceClient(conn), conn, nil
}
