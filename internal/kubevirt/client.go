package kubevirt

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/dcm-project/kubevirt-service-provider/internal/config"
	"github.com/dcm-project/kubevirt-service-provider/internal/constants"
)

// Client wraps a typed REST client for KubeVirt VM operations
type Client struct {
	restClient      *rest.RESTClient
	dynamicClient   dynamic.Interface
	informerFactory dynamicinformer.DynamicSharedInformerFactory
	namespace       string
	timeout         time.Duration
	maxRetries      int
}

var (
	kubevirtScheme         = runtime.NewScheme()
	kubevirtCodecs         serializer.CodecFactory
	kubevirtParameterCodec runtime.ParameterCodec
)

func init() {
	// Register KubeVirt types so the REST client can serialize/deserialize them
	schemeBuilder := runtime.NewSchemeBuilder(func(s *runtime.Scheme) error {
		s.AddKnownTypes(
			schema.GroupVersion{Group: "kubevirt.io", Version: "v1"},
			&kubevirtv1.VirtualMachine{},
			&kubevirtv1.VirtualMachineList{},
			&kubevirtv1.VirtualMachineInstance{},
			&kubevirtv1.VirtualMachineInstanceList{},
		)
		metav1.AddToGroupVersion(s, schema.GroupVersion{Group: "kubevirt.io", Version: "v1"})
		return nil
	})
	_ = schemeBuilder.AddToScheme(kubevirtScheme)
	kubevirtCodecs = serializer.NewCodecFactory(kubevirtScheme)
	kubevirtParameterCodec = runtime.NewParameterCodec(kubevirtScheme)
}

// NewClient creates a new KubeVirt client with a typed REST client for VM operations
// and a dynamic client for informers
func NewClient(cfg *config.KubernetesConfig) (*Client, error) {
	var restConfig *rest.Config
	var err error

	if cfg.Kubeconfig != "" {
		restConfig, err = clientcmd.BuildConfigFromFlags("", cfg.Kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("failed to build config from kubeconfig file %s: %w", cfg.Kubeconfig, err)
		}
	} else {
		restConfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to build in-cluster config: %w", err)
		}
	}

	// Create typed REST client for KubeVirt API
	kubevirtConfig := *restConfig
	kubevirtConfig.GroupVersion = &schema.GroupVersion{Group: "kubevirt.io", Version: "v1"}
	kubevirtConfig.APIPath = "/apis"
	kubevirtConfig.NegotiatedSerializer = serializer.WithoutConversionCodecFactory{CodecFactory: kubevirtCodecs}
	if kubevirtConfig.UserAgent == "" {
		kubevirtConfig.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	restClient, err := rest.RESTClientFor(&kubevirtConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create KubeVirt REST client: %w", err)
	}

	// Create dynamic client for informers
	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	// Create informer factory for monitoring service
	informerFactory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(
		dynamicClient,
		30*time.Minute,
		cfg.Namespace,
		func(options *metav1.ListOptions) {
			options.LabelSelector = fmt.Sprintf("%s=%s", constants.DCMLabelManagedBy, constants.DCMManagedByValue)
		},
	)

	return &Client{
		restClient:      restClient,
		dynamicClient:   dynamicClient,
		informerFactory: informerFactory,
		namespace:       cfg.Namespace,
		timeout:         cfg.Timeout,
		maxRetries:      cfg.MaxRetries,
	}, nil
}

// CreateVirtualMachine creates a new VirtualMachine in the cluster
func (c *Client) CreateVirtualMachine(ctx context.Context, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	result := &kubevirtv1.VirtualMachine{}
	err := c.restClient.Post().
		Resource("virtualmachines").
		Namespace(c.namespace).
		Body(vm).
		Do(timeoutCtx).
		Into(result)
	if err != nil {
		return nil, fmt.Errorf("failed to create VirtualMachine: %w", err)
	}
	result.SetGroupVersionKind(kubevirtv1.VirtualMachineGroupVersionKind)
	return result, nil
}

// GetVirtualMachine retrieves a VirtualMachine by DCM instance ID
func (c *Client) GetVirtualMachine(ctx context.Context, vmID string) (*kubevirtv1.VirtualMachine, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	vmList := &kubevirtv1.VirtualMachineList{}
	err := c.restClient.Get().
		Resource("virtualmachines").
		Namespace(c.namespace).
		VersionedParams(&metav1.ListOptions{
			LabelSelector: fmt.Sprintf("%s=%s", constants.DCMLabelInstanceID, vmID),
		}, kubevirtParameterCodec).
		Do(timeoutCtx).
		Into(vmList)
	if err != nil {
		return nil, fmt.Errorf("failed to get VirtualMachine by dcmlabelinstanceid: %w", err)
	}
	if len(vmList.Items) == 0 {
		return nil, fmt.Errorf("VirtualMachine with dcmlabelinstanceid %q not found", vmID)
	}
	vmList.Items[0].SetGroupVersionKind(kubevirtv1.VirtualMachineGroupVersionKind)
	return &vmList.Items[0], nil
}

// ListVirtualMachines lists all VirtualMachines in the namespace
func (c *Client) ListVirtualMachines(ctx context.Context, options metav1.ListOptions) ([]kubevirtv1.VirtualMachine, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	vmList := &kubevirtv1.VirtualMachineList{}
	err := c.restClient.Get().
		Resource("virtualmachines").
		Namespace(c.namespace).
		VersionedParams(&options, kubevirtParameterCodec).
		Do(timeoutCtx).
		Into(vmList)
	if err != nil {
		return nil, err
	}
	for i := range vmList.Items {
		vmList.Items[i].SetGroupVersionKind(kubevirtv1.VirtualMachineGroupVersionKind)
	}
	return vmList.Items, nil
}

// DeleteVirtualMachine deletes a VirtualMachine by DCM instance ID
func (c *Client) DeleteVirtualMachine(ctx context.Context, vmId string) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	item, err := c.GetVirtualMachine(ctx, vmId)
	if err != nil {
		return fmt.Errorf("failed to get VirtualMachine by dcmlabelinstanceid: %w", err)
	}
	if item == nil {
		return fmt.Errorf("VirtualMachine with dcmlabelinstanceid %q not found", vmId)
	}
	return c.restClient.Delete().
		Resource("virtualmachines").
		Namespace(c.namespace).
		Name(item.Name).
		Body(&metav1.DeleteOptions{}).
		Do(timeoutCtx).
		Error()
}

// UpdateVirtualMachine updates an existing VirtualMachine
func (c *Client) UpdateVirtualMachine(ctx context.Context, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	result := &kubevirtv1.VirtualMachine{}
	err := c.restClient.Put().
		Resource("virtualmachines").
		Namespace(c.namespace).
		Name(vm.Name).
		Body(vm).
		Do(timeoutCtx).
		Into(result)
	if err != nil {
		return nil, err
	}
	result.SetGroupVersionKind(kubevirtv1.VirtualMachineGroupVersionKind)
	return result, nil
}

// DynamicClient returns the underlying dynamic client
func (c *Client) DynamicClient() dynamic.Interface {
	return c.dynamicClient
}
