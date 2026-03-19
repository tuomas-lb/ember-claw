package server_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tuomas-lb/ember-claw/internal/server"
)

func TestStartHealthServer(t *testing.T) {
	// Find a free port
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := lis.Addr().(*net.TCPAddr).Port
	lis.Close()

	ready := true
	srv := server.StartHealthServer(port, func() bool { return ready })
	require.NotNil(t, srv)
	defer srv.Stop(context.Background())

	// Give the goroutine a moment to start
	time.Sleep(100 * time.Millisecond)

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	// Health should return 200
	resp, err := http.Get(baseURL + "/health")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Ready should return 200 since readyFunc returns true
	resp, err = http.Get(baseURL + "/ready")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
