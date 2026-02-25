package monitor

import (
	"context"
	"fmt"
	"log"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"

	"github.com/dcm-project/kubevirt-service-provider/internal/events"
)

// Service monitors VM status changes and publishes events
type Service struct {
	dynamicClient   dynamic.Interface
	namespace       string
	publisher       *events.Publisher
	informerFactory dynamicinformer.DynamicSharedInformerFactory
	vmInformer      cache.SharedIndexInformer
	vmiInformer     cache.SharedIndexInformer
	resyncPeriod    time.Duration
}

var (
	// KubeVirt resource definitions
	virtualMachineGVR = schema.GroupVersionResource{
		Group:    "kubevirt.io",
		Version:  "v1",
		Resource: "virtualmachines",
	}

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
		nil, // No label selector filtering
	)

	// Setup informers
	service.setupInformers()

	return service
}

// setupInformers configures the VM and VMI informers
func (s *Service) setupInformers() {
	// Setup VirtualMachine informer
	s.vmInformer = s.informerFactory.ForResource(virtualMachineGVR).Informer()
	s.vmInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			s.handleVMEvent(obj, "created")
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			s.handleVMEvent(newObj, "updated")
		},
		DeleteFunc: func(obj interface{}) {
			s.handleVMEvent(obj, "deleted")
		},
	})

	// Setup VirtualMachineInstance informer
	s.vmiInformer = s.informerFactory.ForResource(virtualMachineInstanceGVR).Informer()
	s.vmiInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			s.handleVMEvent(obj, "created")
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			s.handleVMEvent(newObj, "updated")
		},
		DeleteFunc: func(obj interface{}) {
			s.handleVMEvent(obj, "deleted")
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
	if !cache.WaitForCacheSync(ctx.Done(), s.vmInformer.HasSynced, s.vmiInformer.HasSynced) {
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
	var vm *unstructured.Unstructured
	var ok bool

	// Handle potential DeletedFinalStateUnknown for delete events
	if eventType == "deleted" {
		if deletedState, isDeleted := obj.(cache.DeletedFinalStateUnknown); isDeleted {
			vm, ok = deletedState.Obj.(*unstructured.Unstructured)
			if !ok {
				log.Printf("Warning: handleVMEvent received unknown deleted object type")
				return
			}
		} else {
			vm, ok = obj.(*unstructured.Unstructured)
			if !ok {
				log.Printf("Warning: handleVMEvent received non-unstructured object")
				return
			}
		}
	} else {
		vm, ok = obj.(*unstructured.Unstructured)
		if !ok {
			log.Printf("Warning: handleVMEvent received non-unstructured object")
			return
		}
	}

	// Only process VMs managed by DCM
	if !s.isDCMManagedVM(vm) {
		return
	}

	// Extract VM information
	vmInfo, err := ExtractVMInfo(vm)
	if err != nil {
		log.Printf("Error extracting VM info: %v", err)
		return
	}

	log.Printf("VM %s: %s (ID: %s) with phase %s", eventType, vmInfo.VMName, vmInfo.VMID, vmInfo.Phase)

	// For delete events, set phase to stopped
	if eventType == "deleted" {
		vmInfo.Phase = VMPhaseStopped
	}

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

// isDCMManagedVM checks if a VM is managed by DCM
func (s *Service) isDCMManagedVM(obj *unstructured.Unstructured) bool {
	labels := obj.GetLabels()
	if labels == nil {
		return false
	}

	managedBy, found := labels["dcm.project/managed-by"]
	return found && managedBy == "dcm"
}

// GetStats returns monitoring service statistics
func (s *Service) GetStats() map[string]interface{} {
	stats := make(map[string]interface{})
	stats["vm_informer_synced"] = s.vmInformer.HasSynced()
	stats["vmi_informer_synced"] = s.vmiInformer.HasSynced()
	stats["publisher_connected"] = s.publisher.IsConnected()

	return stats
}
