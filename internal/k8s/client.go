package k8s

import (
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
// If kubeconfigPath is empty, it falls back to the KUBECONFIG environment
// variable, then to ~/.kube/config.
func NewClient(kubeconfigPath, namespace string) (*Client, error) {
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
