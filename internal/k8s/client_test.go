package k8s

import (
	"encoding/base64"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient_KUBECONFIG_BASE64(t *testing.T) {
	// Create a minimal valid kubeconfig
	kubeconfig := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://localhost:6443
  name: test
contexts:
- context:
    cluster: test
    user: test
  name: test
current-context: test
users:
- name: test
  user:
    token: fake-token
`
	encoded := base64.StdEncoding.EncodeToString([]byte(kubeconfig))

	os.Setenv("KUBECONFIG_BASE64", encoded)
	t.Cleanup(func() { os.Unsetenv("KUBECONFIG_BASE64") })

	client, err := NewClient("", "picoclaw")
	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.NotNil(t, client.restConfig)
	assert.Equal(t, "https://localhost:6443", client.restConfig.Host)
}

func TestNewClient_KUBECONFIG_BASE64_InvalidBase64(t *testing.T) {
	os.Setenv("KUBECONFIG_BASE64", "not-valid-base64!!!")
	t.Cleanup(func() { os.Unsetenv("KUBECONFIG_BASE64") })

	_, err := NewClient("", "picoclaw")
	assert.Error(t, err)
}

func TestNewClient_ExplicitPathOverridesBase64(t *testing.T) {
	// Even if KUBECONFIG_BASE64 is set, explicit path should take precedence.
	// We set KUBECONFIG_BASE64 to something valid but use a non-existent explicit path.
	os.Setenv("KUBECONFIG_BASE64", base64.StdEncoding.EncodeToString([]byte("apiVersion: v1")))
	t.Cleanup(func() { os.Unsetenv("KUBECONFIG_BASE64") })

	_, err := NewClient("/nonexistent/kubeconfig", "picoclaw")
	// Should fail trying to read the explicit path, not the base64 env var
	assert.Error(t, err)
}
