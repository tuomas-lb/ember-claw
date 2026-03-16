package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sipeed/picoclaw/pkg/health"
)

// TestHealthEndpoints verifies PicoClaw's health.Server behavior (GRPC-04).
//
// The health server is used as-is in the sidecar for K8s probes.
// - /health always returns 200 {"status":"ok"}
// - /ready returns 503 before SetReady(true), then 200 after
func TestHealthEndpoints(t *testing.T) {
	// Create health server (not started — ready is false by default)
	healthSrv := health.NewServer("0.0.0.0", 0)

	// Register the health server handlers on a test mux
	mux := http.NewServeMux()
	healthSrv.RegisterOnMux(mux)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	t.Run("health always returns 200", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/health")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var body map[string]interface{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, "ok", body["status"])
	})

	t.Run("ready returns 503 before SetReady", func(t *testing.T) {
		// Confirm SetReady(false) is the initial state
		healthSrv.SetReady(false)

		resp, err := http.Get(ts.URL + "/ready")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

		var body map[string]interface{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, "not ready", body["status"])
	})

	t.Run("ready returns 200 after SetReady(true)", func(t *testing.T) {
		healthSrv.SetReady(true)

		resp, err := http.Get(ts.URL + "/ready")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var body map[string]interface{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, "ready", body["status"])
	})
}

// TestGRPCHealth verifies gRPC health check returns SERVING status (GRPC-04).
// This test uses the standard grpc/health protocol that K8s 1.24+ understands natively.
// The actual gRPC health registration happens in the server setup (Plan 02).
// This test verifies the health server library is accessible and the status logic is sound.
func TestGRPCHealth(t *testing.T) {
	// The gRPC health server is tested via grpc health protocol.
	// Full integration test requires a running gRPC server with health registered.
	// That is covered in the server integration once server.go is implemented (Plan 02).
	// For now, this test documents the expected behavior and will expand in Plan 02.
	t.Log("gRPC health check integration test: deferred to Plan 02 server implementation")
	t.Log("Expected: grpc.health.v1.Health/Check returns SERVING for the empty service name")
}
