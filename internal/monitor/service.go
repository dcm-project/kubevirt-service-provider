package monitor

import (
	"context"
	"fmt"
	"log"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	kubevirtv1 "kubevirt.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/dcm-project/kubevirt-service-provider/internal/constants"
	"github.com/dcm-project/kubevirt-service-provider/internal/events"
)

// Service monitors VM status changes and publishes events
type Service struct {
	dynamicClient   dynamic.Interface
	namespace       string
	publisher       *events.Publisher
	informerFactory dynamicinformer.DynamicSharedInformerFactory
	vmiInformer     cache.SharedIndexInformer
	resyncPeriod    time.Duration
}

var (
	virtualMachineInstanceGVR = schema.GroupVersionResource{
		Group:    "kubevirt.io",
		Version:  "v1",
		Resource: "virtualmachineinstances",
	}
)

// MonitorConfig contains configuration for the monitoring service
type MonitorConfig struct {
	Namespace    string
	ResyncPeriod time.Duration
}

// NewMonitorService creates a new VM monitoring service
func NewMonitorService(dynamicClient dynamic.Interface, publisher *events.Publisher, config MonitorConfig) *Service {
	service := &Service{
		dynamicClient: dynamicClient,
		namespace:     config.Namespace,
		publisher:     publisher,
		resyncPeriod:  config.ResyncPeriod,
	}

	// Create informer factory
	service.informerFactory = dynamicinformer.NewFilteredDynamicSharedInformerFactory(
		dynamicClient,
		config.ResyncPeriod,
		config.Namespace,
		func(options *metav1.ListOptions) {
			options.LabelSelector = fmt.Sprintf("%s=%s", constants.DCMLabelManagedBy, constants.DCMManagedByValue)
		},
	)

	// Setup informers
	service.setupInformers()

	return service
}

// setupInformers configures the VM and VMI informers
func (s *Service) setupInformers() {
	// Setup VirtualMachineInstance informer
	s.vmiInformer = s.informerFactory.ForResource(virtualMachineInstanceGVR).Informer()
	s.vmiInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			s.handleVMEvent(obj, "created")
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			s.handleVMEvent(newObj, "updated")
		},
	})
}

// Run starts the monitoring service
func (s *Service) Run(ctx context.Context) error {
	log.Printf("Starting KubeVirt VM monitoring service in namespace %s", s.namespace)

	// Start informers
	s.informerFactory.Start(ctx.Done())

	// Wait for cache sync
	log.Printf("Waiting for informer caches to sync...")
	if !cache.WaitForCacheSync(ctx.Done(), s.vmiInformer.HasSynced) {
		return fmt.Errorf("failed to sync informer caches")
	}

	log.Printf("Informer caches synced successfully")
	log.Printf("KubeVirt VM monitoring service is running")

	// Wait for context cancellation
	<-ctx.Done()
	log.Printf("Stopping KubeVirt VM monitoring service")
	return nil
}

// handleVMEvent handles any VM/VMI event by publishing current state
func (s *Service) handleVMEvent(obj interface{}, eventType string) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		log.Printf("Warning: handleVMEvent received non-unstructured object")
		return
	}

	// Convert unstructured to typed VMI at the informer boundary
	vmi := &kubevirtv1.VirtualMachineInstance{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, vmi); err != nil {
		log.Printf("Error converting unstructured to VirtualMachineInstance: %v", err)
		return
	}

	// If the VMI don't contain ID skip the VM event
	if vmi.Labels[constants.DCMLabelInstanceID] == "" {
		log.Printf("Warning: VMI %s does not contain DCM instance ID", vmi.Name)
		return
	}

	// Extract VM information
	vmInfo, err := ExtractVMInfo(vmi)
	if err != nil {
		log.Printf("Error extracting VM info: %v", err)
		return
	}

	log.Printf("VM %s: %s (ID: %s) with phase %s", eventType, vmInfo.VMName, vmInfo.VMID, vmInfo.Phase)

	// Publish current VM state
	s.publishVMEvent(vmInfo)
}

// publishVMEvent publishes the current VM state
func (s *Service) publishVMEvent(vmInfo VMInfo) {
	vmEvent := events.VMEvent{
		VMID:      vmInfo.VMID,
		VMName:    vmInfo.VMName,
		Namespace: vmInfo.Namespace,
		Phase:     vmInfo.Phase.String(),
		Timestamp: time.Now(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.publisher.PublishVMEvent(ctx, vmEvent); err != nil {
		log.Printf("Error publishing VM event for %s: %v", vmInfo.VMID, err)
	}
}

// GetStats returns monitoring service statistics
func (s *Service) GetStats() map[string]interface{} {
	stats := make(map[string]interface{})
	stats["vmi_informer_synced"] = s.vmiInformer.HasSynced()
	stats["publisher_connected"] = s.publisher.IsConnected()

	return stats
}
