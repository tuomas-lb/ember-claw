package grpc

import (
	"context"
	"fmt"

	pb "github.com/tuomas-lb/ember-claw/dashboard/gen/emberclaw/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client manages gRPC connections to PicoClaw instances via cluster-internal DNS.
type Client struct {
	namespace string
}

// New creates a gRPC client targeting instances in the given namespace.
func New(namespace string) *Client {
	return &Client{namespace: namespace}
}

// dial opens a gRPC connection to the named instance using its cluster DNS address.
func (c *Client) dial(instanceName string) (*grpc.ClientConn, error) {
	addr := fmt.Sprintf("picoclaw-%s.%s.svc.cluster.local:50051", instanceName, c.namespace)
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	return conn, nil
}

// Status calls the Status RPC on the named instance and returns the response.
func (c *Client) Status(ctx context.Context, name string) (*pb.StatusResponse, error) {
	conn, err := c.dial(name)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	client := pb.NewPicoClawServiceClient(conn)
	resp, err := client.Status(ctx, &pb.StatusRequest{})
	if err != nil {
		return nil, fmt.Errorf("status rpc for %s: %w", name, err)
	}
	return resp, nil
}

// Query sends a single message to the named instance and returns the full response.
func (c *Client) Query(ctx context.Context, name, message, sessionKey string) (*pb.QueryResponse, error) {
	conn, err := c.dial(name)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	client := pb.NewPicoClawServiceClient(conn)
	resp, err := client.Query(ctx, &pb.QueryRequest{
		Message:    message,
		SessionKey: sessionKey,
	})
	if err != nil {
		return nil, fmt.Errorf("query rpc for %s: %w", name, err)
	}
	return resp, nil
}

// ChatStream opens a bidirectional streaming Chat session to the named instance.
// The caller is responsible for closing the stream when done.
// The underlying gRPC connection is closed when the stream's Context is cancelled.
func (c *Client) ChatStream(ctx context.Context, name string) (pb.PicoClawService_ChatClient, error) {
	conn, err := c.dial(name)
	if err != nil {
		return nil, err
	}

	// Close the connection when the context is done so we don't leak.
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	client := pb.NewPicoClawServiceClient(conn)
	stream, err := client.Chat(ctx)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("open chat stream for %s: %w", name, err)
	}
	return stream, nil
}
