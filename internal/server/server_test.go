package server_test

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	emberclaw "github.com/LastBotInc/ember-claw/gen/emberclaw/v1"
	"github.com/LastBotInc/ember-claw/internal/server"
)

const bufSize = 1024 * 1024

// mockProcessor implements AgentProcessor for testing.
// It records all calls and returns configurable responses.
type mockProcessor struct {
	mu          sync.Mutex
	calls       []mockCall
	response    string
	responseErr error
}

type mockCall struct {
	content    string
	sessionKey string
}

func newMockProcessor(response string) *mockProcessor {
	return &mockProcessor{response: response}
}

func (m *mockProcessor) ProcessDirect(ctx context.Context, content, sessionKey string) (string, error) {
	m.mu.Lock()
	m.calls = append(m.calls, mockCall{content: content, sessionKey: sessionKey})
	m.mu.Unlock()
	return m.response, m.responseErr
}

func (m *mockProcessor) getCalls() []mockCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]mockCall, len(m.calls))
	copy(result, m.calls)
	return result
}

// startTestServer starts a gRPC server using an in-memory bufconn listener.
// Returns the client connection and a cleanup function.
func startTestServer(t *testing.T, mock *mockProcessor) (*grpc.ClientConn, func()) {
	t.Helper()

	lis := bufconn.Listen(bufSize)
	grpcSrv := grpc.NewServer()

	svc := server.New(mock)
	emberclaw.RegisterPicoClawServiceServer(grpcSrv, svc)

	go func() {
		if err := grpcSrv.Serve(lis); err != nil {
			// Ignore error on test shutdown
			_ = err
		}
	}()

	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	cleanup := func() {
		conn.Close()
		grpcSrv.GracefulStop()
		lis.Close()
	}

	return conn, cleanup
}

// TestProcessDirect verifies the mock AgentProcessor returns canned response (GRPC-01).
func TestProcessDirect(t *testing.T) {
	mock := newMockProcessor("hello from agent")
	resp, err := mock.ProcessDirect(context.Background(), "hi", "session:test")
	require.NoError(t, err)
	assert.Equal(t, "hello from agent", resp)

	calls := mock.getCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "hi", calls[0].content)
	assert.Equal(t, "session:test", calls[0].sessionKey)
}

// TestChatBidiStream verifies client can send one message and receive a response (GRPC-02).
func TestChatBidiStream(t *testing.T) {
	mock := newMockProcessor("pong")
	conn, cleanup := startTestServer(t, mock)
	defer cleanup()

	client := emberclaw.NewPicoClawServiceClient(conn)
	stream, err := client.Chat(context.Background())
	require.NoError(t, err)

	err = stream.Send(&emberclaw.ChatRequest{Message: "ping"})
	require.NoError(t, err)

	resp, err := stream.Recv()
	require.NoError(t, err)
	assert.Equal(t, "pong", resp.Text)
	assert.True(t, resp.Done)

	err = stream.CloseSend()
	require.NoError(t, err)

	// After CloseSend, Recv should return EOF
	_, err = stream.Recv()
	assert.Equal(t, io.EOF, err)
}

// TestChatMultipleMessages verifies client can send 3 messages and receive 3 responses (GRPC-02).
func TestChatMultipleMessages(t *testing.T) {
	mock := newMockProcessor("response")
	conn, cleanup := startTestServer(t, mock)
	defer cleanup()

	client := emberclaw.NewPicoClawServiceClient(conn)
	stream, err := client.Chat(context.Background())
	require.NoError(t, err)

	messages := []string{"msg1", "msg2", "msg3"}
	for _, msg := range messages {
		err = stream.Send(&emberclaw.ChatRequest{Message: msg})
		require.NoError(t, err)

		resp, err := stream.Recv()
		require.NoError(t, err)
		assert.Equal(t, "response", resp.Text)
		assert.True(t, resp.Done)
	}

	err = stream.CloseSend()
	require.NoError(t, err)

	_, err = stream.Recv()
	assert.Equal(t, io.EOF, err)

	// Verify mock received all 3 messages
	calls := mock.getCalls()
	require.Len(t, calls, 3)
	for i, msg := range messages {
		assert.Equal(t, msg, calls[i].content)
	}
}

// TestQuery verifies the unary Query RPC returns a response (GRPC-03).
func TestQuery(t *testing.T) {
	mock := newMockProcessor("query response")
	conn, cleanup := startTestServer(t, mock)
	defer cleanup()

	client := emberclaw.NewPicoClawServiceClient(conn)
	resp, err := client.Query(context.Background(), &emberclaw.QueryRequest{Message: "what is 2+2"})
	require.NoError(t, err)
	assert.Equal(t, "query response", resp.Text)
	assert.Empty(t, resp.Error)
}

// TestSessionIsolation verifies two concurrent streams use different session keys (GRPC-05).
func TestSessionIsolation(t *testing.T) {
	mock := newMockProcessor("ok")
	conn, cleanup := startTestServer(t, mock)
	defer cleanup()

	client := emberclaw.NewPicoClawServiceClient(conn)

	// Start two concurrent streams
	stream1, err := client.Chat(context.Background())
	require.NoError(t, err)

	stream2, err := client.Chat(context.Background())
	require.NoError(t, err)

	// Send one message on each stream
	require.NoError(t, stream1.Send(&emberclaw.ChatRequest{Message: "from stream1"}))
	_, err = stream1.Recv()
	require.NoError(t, err)

	require.NoError(t, stream2.Send(&emberclaw.ChatRequest{Message: "from stream2"}))
	_, err = stream2.Recv()
	require.NoError(t, err)

	stream1.CloseSend()
	stream2.CloseSend()

	// Drain EOF
	stream1.Recv()
	stream2.Recv()

	calls := mock.getCalls()
	require.Len(t, calls, 2)

	// Each stream must have been assigned a unique session key
	key1 := calls[0].sessionKey
	key2 := calls[1].sessionKey
	assert.NotEmpty(t, key1, "stream1 session key must not be empty")
	assert.NotEmpty(t, key2, "stream2 session key must not be empty")
	assert.NotEqual(t, key1, key2, "concurrent streams must have different session keys")
}
