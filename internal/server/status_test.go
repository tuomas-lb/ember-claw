package server_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	emberclaw "github.com/tuomas-lb/ember-claw/gen/emberclaw/v1"
	"github.com/tuomas-lb/ember-claw/internal/server"
)

// TestStatusRPC verifies the Status RPC returns model, provider, ready, and uptime.
func TestStatusRPC(t *testing.T) {
	mock := newMockProcessor("ok")
	conn, cleanup := startTestServer(t, mock)
	defer cleanup()

	client := emberclaw.NewPicoClawServiceClient(conn)
	resp, err := client.Status(context.Background(), &emberclaw.StatusRequest{})
	require.NoError(t, err)

	// Default server is not ready (SetReady not called in startTestServer)
	assert.False(t, resp.Ready)
	assert.Empty(t, resp.Model)
	assert.Empty(t, resp.Provider)
	assert.True(t, resp.UptimeSeconds >= 0)
}

// TestStatusRPC_WithConfig verifies Status returns configured model/provider.
func TestStatusRPC_WithConfig(t *testing.T) {
	mock := newMockProcessor("ok")

	svc := server.New(mock)
	svc.SetModel("gemini-2.5-flash")
	svc.SetProvider("gemini")
	svc.SetReady(true)

	resp, err := svc.Status(context.Background(), &emberclaw.StatusRequest{})
	require.NoError(t, err)

	assert.True(t, resp.Ready)
	assert.Equal(t, "gemini-2.5-flash", resp.Model)
	assert.Equal(t, "gemini", resp.Provider)
	assert.True(t, resp.UptimeSeconds >= 0)
}

// TestQueryErrorPropagation verifies agent errors are returned in QueryResponse.Error.
func TestQueryErrorPropagation(t *testing.T) {
	mock := newMockProcessor("")
	mock.responseErr = fmt.Errorf("provider timeout")

	conn, cleanup := startTestServer(t, mock)
	defer cleanup()

	client := emberclaw.NewPicoClawServiceClient(conn)
	resp, err := client.Query(context.Background(), &emberclaw.QueryRequest{Message: "hello"})
	require.NoError(t, err, "Query should not return gRPC error for agent errors")
	assert.Equal(t, "provider timeout", resp.Error)
	assert.Empty(t, resp.Text)
}

// TestChatWithClientSessionKey verifies client-provided session key is honored.
func TestChatWithClientSessionKey(t *testing.T) {
	mock := newMockProcessor("response")
	conn, cleanup := startTestServer(t, mock)
	defer cleanup()

	client := emberclaw.NewPicoClawServiceClient(conn)
	stream, err := client.Chat(context.Background())
	require.NoError(t, err)

	// Send with explicit session key
	err = stream.Send(&emberclaw.ChatRequest{Message: "hi", SessionKey: "my-custom-session"})
	require.NoError(t, err)

	_, err = stream.Recv()
	require.NoError(t, err)

	stream.CloseSend()
	stream.Recv() // drain EOF

	calls := mock.getCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "my-custom-session", calls[0].sessionKey)
}

// TestQueryWithSessionKey verifies client-provided session key in Query.
func TestQueryWithSessionKey(t *testing.T) {
	mock := newMockProcessor("result")
	conn, cleanup := startTestServer(t, mock)
	defer cleanup()

	client := emberclaw.NewPicoClawServiceClient(conn)
	resp, err := client.Query(context.Background(), &emberclaw.QueryRequest{
		Message:    "what is 1+1",
		SessionKey: "custom-query-session",
	})
	require.NoError(t, err)
	assert.Equal(t, "result", resp.Text)

	calls := mock.getCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "custom-query-session", calls[0].sessionKey)
}
