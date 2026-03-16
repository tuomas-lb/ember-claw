package k8s

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// PortForwardResult holds the OS-assigned local port and a stop channel for the
// active port-forward tunnel. Close stop channel to terminate the tunnel.
type PortForwardResult struct {
	LocalPort uint16
	StopChan  chan struct{}
}

// PortForwardPod establishes an in-process SPDY port-forward from an
// OS-assigned ephemeral local port to remotePort on the named pod.
//
// The tunnel stays open until pf.StopChan is closed (or the context is done).
// Callers should defer close(pf.StopChan) to clean up.
//
// NOTE: SPDY transport is not supported by fake clientsets; this method is
// intended for integration / live-cluster use only. Unit tests cover the gRPC
// layer separately via bufconn.
func (c *Client) PortForwardPod(ctx context.Context, podName string, remotePort int) (*PortForwardResult, error) {
	if c.restConfig == nil {
		return nil, fmt.Errorf("port-forward requires a real REST config (not available with fake clientset)")
	}

	// Build the portforward sub-resource URL for the pod.
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", c.namespace, podName)
	hostIP := strings.TrimLeft(c.restConfig.Host, "https://")

	transport, upgrader, err := spdy.RoundTripperFor(c.restConfig)
	if err != nil {
		return nil, fmt.Errorf("spdy.RoundTripperFor: %w", err)
	}

	dialer := spdy.NewDialer(
		upgrader,
		&http.Client{Transport: transport},
		http.MethodPost,
		&url.URL{Scheme: "https", Path: path, Host: hostIP},
	)

	stopChan := make(chan struct{}, 1)
	readyChan := make(chan struct{})

	// Port 0 lets the OS choose an ephemeral local port.
	pf, err := portforward.New(
		dialer,
		[]string{fmt.Sprintf("0:%d", remotePort)},
		stopChan,
		readyChan,
		io.Discard, // stdout: suppress banner lines
		io.Discard, // stderr: suppress error chatter
	)
	if err != nil {
		return nil, fmt.Errorf("portforward.New: %w", err)
	}

	errChan := make(chan error, 1)
	go func() { errChan <- pf.ForwardPorts() }()

	select {
	case <-readyChan:
		// Tunnel is ready; retrieve the assigned local port.
	case err := <-errChan:
		return nil, fmt.Errorf("port-forward failed: %w", err)
	case <-ctx.Done():
		close(stopChan)
		return nil, ctx.Err()
	}

	ports, err := pf.GetPorts()
	if err != nil {
		close(stopChan)
		return nil, fmt.Errorf("pf.GetPorts: %w", err)
	}
	if len(ports) == 0 {
		close(stopChan)
		return nil, fmt.Errorf("no ports returned by portforward")
	}

	return &PortForwardResult{
		LocalPort: ports[0].Local,
		StopChan:  stopChan,
	}, nil
}
