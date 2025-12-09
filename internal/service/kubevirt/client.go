package kubevirt

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"kubevirt.io/client-go/kubecli"
)

// Client wraps KubeVirt client operations
type Client struct {
	client kubecli.KubevirtClient
}

// NewClient creates a new KubeVirt client wrapper
func NewClient(client kubecli.KubevirtClient) *Client {
	return &Client{client: client}
}

// NamespaceExists checks if a Kubernetes namespace exists
func (k *Client) NamespaceExists(ctx context.Context, namespace string) (bool, error) {
	if namespace == "" {
		return false, fmt.Errorf("namespace name cannot be empty")
	}

	_, err := k.client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, fmt.Errorf("namespace %q does not exist", namespace)
		}
		return false, fmt.Errorf("failed to check namespace %q: %w", namespace, err)
	}
	return true, nil
}
