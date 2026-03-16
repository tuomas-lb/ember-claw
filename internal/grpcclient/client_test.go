package grpcclient_test

import (
	"context"
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	emberclaw "github.com/LastBotInc/ember-claw/gen/emberclaw/v1"
	"github.com/LastBotInc/ember-claw/internal/grpcclient"
	"github.com/LastBotInc/ember-claw/internal/server"
)

const bufSize = 1024 * 1024

// mockProcessor echoes content back with a prefix.
type mockProcessor struct{}

func (m *mockProcessor) ProcessDirect(_ context.Context, content, _ string) (string, error) {
	return "echo: " + content, nil
}

// startTestServer starts a gRPC server using an in-memory bufconn listener.
// Returns the local port (0 sentinel), the bufconn dialer, and a cleanup function.
func startBufconnServer(t *testing.T) (*grpc.ClientConn, func()) {
	t.Helper()

	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer()
	emberclaw.RegisterPicoClawServiceServer(srv, server.New(&mockProcessor{}))

	go func() {
		if err := srv.Serve(lis); err != nil && err != grpc.ErrServerStopped {
			t.Logf("bufconn server error: %v", err)
		}
	}()

	conn, err := grpc.NewClient(
		"passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	cleanup := func() {
		conn.Close()
		srv.Stop()
		lis.Close()
	}
	return conn, cleanup
}

// TestDialSidecar verifies that DialSidecar returns a valid PicoClawServiceClient.
// We can't easily test the real localhost dial without a live server, so we
// focus on the bufconn-backed path below. This test just confirms DialSidecar
// returns without error on a free ephemeral port (connection is lazy).
func TestDialSidecar_ReturnsClient(t *testing.T) {
	ctx := context.Background()
	client, conn, err := grpcclient.DialSidecar(ctx, 50051)
	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.NotNil(t, conn)
	conn.Close()
}

// TestQueryRPC dials a bufconn-backed server, sends a QueryRequest, and
// verifies the response text matches the mock processor's echo output.
func TestQueryRPC(t *testing.T) {
	conn, cleanup := startBufconnServer(t)
	defer cleanup()

	client := emberclaw.NewPicoClawServiceClient(conn)
	ctx := context.Background()

	resp, err := client.Query(ctx, &emberclaw.QueryRequest{
		Message:    "hello",
		SessionKey: "test-session",
	})
	require.NoError(t, err)
	assert.Equal(t, "echo: hello", resp.Text)
	assert.Empty(t, resp.Error)
}

// TestChatStream opens a bidi streaming Chat RPC, sends 3 messages, and verifies
// 3 corresponding echo responses are received.
func TestChatStream(t *testing.T) {
	conn, cleanup := startBufconnServer(t)
	defer cleanup()

	client := emberclaw.NewPicoClawServiceClient(conn)
	ctx := context.Background()

	stream, err := client.Chat(ctx)
	require.NoError(t, err)

	messages := []string{"msg1", "msg2", "msg3"}
	for _, msg := range messages {
		err := stream.Send(&emberclaw.ChatRequest{
			Message:    msg,
			SessionKey: "test-session",
		})
		require.NoError(t, err)

		resp, err := stream.Recv()
		require.NoError(t, err)
		assert.Equal(t, "echo: "+msg, resp.Text)
	}

	err = stream.CloseSend()
	require.NoError(t, err)

	// Drain remaining responses
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
	}
}
