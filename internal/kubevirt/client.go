package kubevirt

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/dcm-project/kubevirt-service-provider/internal/config"
)

// Client wraps the Kubernetes dynamic client for KubeVirt operations
type Client struct {
	dynamicClient   dynamic.Interface
	informerFactory dynamicinformer.DynamicSharedInformerFactory
	namespace       string
	timeout         time.Duration
	maxRetries      int
}

var (
	// KubeVirt VirtualMachine GroupVersionResource
	virtualMachineGVR = schema.GroupVersionResource{
		Group:    "kubevirt.io",
		Version:  "v1",
		Resource: "virtualmachines",
	}
)

// NewClient creates a new KubeVirt client using dynamic Kubernetes client
func NewClient(cfg *config.KubernetesConfig) (*Client, error) {
	var restConfig *rest.Config
	var err error

	if cfg.Kubeconfig != "" {
		// Load kubeconfig from file
		restConfig, err = clientcmd.BuildConfigFromFlags("", cfg.Kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("failed to build config from kubeconfig file %s: %w", cfg.Kubeconfig, err)
		}
	} else {
		// Use in-cluster config
		restConfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to build in-cluster config: %w", err)
		}
	}

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	// Create informer factory for monitoring service
	informerFactory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(
		dynamicClient,
		30*time.Minute, // Default resync period
		cfg.Namespace,
		nil, // No label selector filtering
	)

	return &Client{
		dynamicClient:   dynamicClient,
		informerFactory: informerFactory,
		namespace:       cfg.Namespace,
		timeout:         cfg.Timeout,
		maxRetries:      cfg.MaxRetries,
	}, nil
}

// CreateVirtualMachine creates a new VirtualMachine in the cluster
func (c *Client) CreateVirtualMachine(ctx context.Context, vm *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		createdVM, err := c.dynamicClient.Resource(virtualMachineGVR).Namespace(c.namespace).Create(timeoutCtx, vm, metav1.CreateOptions{})
		if err == nil {
			return createdVM, nil
		}
		lastErr = err

		if attempt < c.maxRetries {
			// Wait before retrying (exponential backoff)
			backoff := time.Duration(attempt+1) * time.Second
			select {
			case <-time.After(backoff):
				continue
			case <-timeoutCtx.Done():
				return nil, timeoutCtx.Err()
			}
		}
	}

	return nil, fmt.Errorf("failed to create VirtualMachine after %d retries: %w", c.maxRetries, lastErr)
}

// GetVirtualMachine retrieves a VirtualMachine by name
func (c *Client) GetVirtualMachine(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	return c.dynamicClient.Resource(virtualMachineGVR).Namespace(c.namespace).Get(timeoutCtx, name, metav1.GetOptions{})
}

// ListVirtualMachines lists all VirtualMachines in the namespace
func (c *Client) ListVirtualMachines(ctx context.Context, options metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	return c.dynamicClient.Resource(virtualMachineGVR).Namespace(c.namespace).List(timeoutCtx, options)
}

// DeleteVirtualMachine deletes a VirtualMachine by name
func (c *Client) DeleteVirtualMachine(ctx context.Context, name string) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	return c.dynamicClient.Resource(virtualMachineGVR).Namespace(c.namespace).Delete(timeoutCtx, name, metav1.DeleteOptions{})
}

// UpdateVirtualMachine updates an existing VirtualMachine
func (c *Client) UpdateVirtualMachine(ctx context.Context, vm *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	return c.dynamicClient.Resource(virtualMachineGVR).Namespace(c.namespace).Update(timeoutCtx, vm, metav1.UpdateOptions{})
}

// DynamicClient returns the underlying dynamic client
func (c *Client) DynamicClient() dynamic.Interface {
	return c.dynamicClient
}
