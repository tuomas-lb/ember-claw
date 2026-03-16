package server

import (
	"github.com/sipeed/picoclaw/pkg/health"
)

// StartHealthServer starts the PicoClaw HTTP health server on addr (e.g. "0.0.0.0:8080").
// It exposes /health (liveness) and /ready (readiness) endpoints for K8s probes (K8S-04).
//
// readyFunc is polled by the /ready handler: 200 when true, 503 when false.
// The returned *http.Server can be used to shut down the server gracefully.
//
// The function starts the server in a goroutine and returns immediately.
// If addr is empty, "0.0.0.0:8080" is used.
func StartHealthServer(port int, readyFunc func() bool) *health.Server {
	srv := health.NewServer("0.0.0.0", port)
	// Sync initial state from readyFunc
	srv.SetReady(readyFunc())
	go srv.Start()
	return srv
}
