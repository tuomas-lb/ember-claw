package k8s

import (
	"encoding/base64"
	"os"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client wraps a Kubernetes clientset with namespace scoping and rest config.
// The rest config is retained for operations that need it (e.g., port-forward).
type Client struct {
	cs         kubernetes.Interface
	restConfig *rest.Config
	namespace  string
}

// NewClient builds a Client from a kubeconfig path and namespace.
// Resolution order:
//  1. Explicit kubeconfigPath argument
//  2. KUBECONFIG_BASE64 env var (base64-encoded kubeconfig, for CI/automation)
//  3. KUBECONFIG env var (standard kubectl behavior)
//  4. ~/.kube/config
func NewClient(kubeconfigPath, namespace string) (*Client, error) {
	if kubeconfigPath == "" {
		if b64 := os.Getenv("KUBECONFIG_BASE64"); b64 != "" {
			decoded, err := base64.StdEncoding.DecodeString(b64)
			if err != nil {
				return nil, err
			}
			f, err := os.CreateTemp("", "eclaw-kubeconfig-*.yaml")
			if err != nil {
				return nil, err
			}
			if _, err := f.Write(decoded); err != nil {
				f.Close()
				return nil, err
			}
			f.Close()
			kubeconfigPath = f.Name()
		}
	}

	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, err
	}
	cs, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}
	return &Client{cs: cs, restConfig: restConfig, namespace: namespace}, nil
}

// NewClientFromClientset creates a Client from an existing Kubernetes Interface.
// This is the primary constructor for tests using fake.NewSimpleClientset().
func NewClientFromClientset(cs kubernetes.Interface, namespace string) *Client {
	return &Client{cs: cs, namespace: namespace}
}
