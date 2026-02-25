package monitor

import (
	"fmt"
	"log"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/dcm-project/kubevirt-service-provider/internal/constants"
)

// VMPhase represents the current phase/state of a VM
type VMPhase string

func (p VMPhase) String() string {
	return string(p)
}

const (
	// TODO: Common state for DCM. Must be published, so it can be shared with other providers.
	VMPhaseUnknown     VMPhase = "Unknown"
	VMPhasePending     VMPhase = "Pending"
	VMPhaseScheduling  VMPhase = "Scheduling"
	VMPhaseScheduled   VMPhase = "Scheduled"
	VMPhaseRunning     VMPhase = "Running"
	VMPhaseStopped     VMPhase = "Stopped"
	VMPhaseFailed      VMPhase = "Failed"
	VMPhaseSucceeded   VMPhase = "Succeeded"
	VMPhaseTerminating VMPhase = "Terminating"
)

// VMInfo contains extracted VM information for phase comparison
type VMInfo struct {
	VMID      string
	VMName    string
	Namespace string
	Phase     VMPhase
}

// PhaseChange represents a detected phase transition
type PhaseChange struct {
	VMInfo        VMInfo
	PreviousPhase VMPhase
	CurrentPhase  VMPhase
	IsSignificant bool // Whether this change should trigger an event
}

// ExtractVMInfo extracts phase and identifying information from a VM object
func ExtractVMInfo(obj *unstructured.Unstructured) (VMInfo, error) {
	if obj == nil {
		return VMInfo{}, fmt.Errorf("VM object is nil")
	}

	// Extract basic identifying information
	vmName := obj.GetName()
	namespace := obj.GetNamespace()

	// Extract DCM instance ID from labels
	vmID := ""
	labels := obj.GetLabels()
	if labels != nil {
		if id, found := labels[constants.DCMLabelInstanceID]; found {
			vmID = id
		}
	}

	// If no DCM instance ID found, use VM name as fallback
	if vmID == "" {
		vmID = vmName
		log.Printf("Warning: No DCM instance ID found for VM %s, using VM name as ID", vmName)
	}

	// Extract phase based on object kind
	phase, err := extractPhase(obj)
	if err != nil {
		return VMInfo{}, fmt.Errorf("failed to extract phase for VM %s: %w", vmName, err)
	}

	return VMInfo{
		VMID:      vmID,
		VMName:    vmName,
		Namespace: namespace,
		Phase:     phase,
	}, nil
}

// extractPhase determines the current phase of a VM from its status
func extractPhase(obj *unstructured.Unstructured) (VMPhase, error) {
	kind := obj.GetKind()

	switch kind {
	case "VirtualMachine":
		return extractVMPhase(obj)
	case "VirtualMachineInstance":
		return extractVMIPhase(obj)
	default:
		return VMPhaseUnknown, fmt.Errorf("unsupported object kind: %s", kind)
	}
}

// extractVMPhase extracts phase from VirtualMachine status
func extractVMPhase(vm *unstructured.Unstructured) (VMPhase, error) {
	// Check if VM is running (spec.running)
	running, found, err := unstructured.NestedBool(vm.Object, "spec", "running")
	if err != nil {
		return VMPhaseUnknown, fmt.Errorf("failed to get spec.running: %w", err)
	}
	if !found {
		running = true // Default to running if not specified
	}

	// If spec.running is false, VM is stopped
	if !running {
		return VMPhaseStopped, nil
	}

	// Check status.ready
	ready, found, err := unstructured.NestedBool(vm.Object, "status", "ready")
	if err != nil {
		log.Printf("Warning: failed to get status.ready: %v", err)
	}
	if found && ready {
		return VMPhaseRunning, nil
	}

	// Check status.created
	created, found, err := unstructured.NestedBool(vm.Object, "status", "created")
	if err != nil {
		log.Printf("Warning: failed to get status.created: %v", err)
	}
	if found && !created {
		return VMPhasePending, nil
	}

	// Check status.conditions for more detailed state
	conditions, found, err := unstructured.NestedSlice(vm.Object, "status", "conditions")
	if err != nil {
		log.Printf("Warning: failed to get status.conditions: %v", err)
	}
	if found {
		phase := analyzeVMConditions(conditions)
		if phase != VMPhaseUnknown {
			return phase, nil
		}
	}

	// If VM is running but not ready yet, it's likely pending/starting
	if running && (!found || !ready) {
		return VMPhasePending, nil
	}

	// Default case
	return VMPhaseUnknown, nil
}

// extractVMIPhase extracts phase from VirtualMachineInstance status
func extractVMIPhase(vmi *unstructured.Unstructured) (VMPhase, error) {
	// VMI has more detailed phase information in status.phase
	phase, found, err := unstructured.NestedString(vmi.Object, "status", "phase")
	if err != nil {
		return VMPhaseUnknown, fmt.Errorf("failed to get VMI status.phase: %w", err)
	}
	if !found {
		return VMPhasePending, nil
	}

	// Map VMI phases to our VMPhase constants
	switch phase {
	case "Pending":
		return VMPhasePending, nil
	case "Scheduling":
		return VMPhaseScheduling, nil
	case "Scheduled":
		return VMPhaseScheduled, nil
	case "Running":
		return VMPhaseRunning, nil
	case "Succeeded":
		return VMPhaseSucceeded, nil
	case "Failed":
		return VMPhaseFailed, nil
	case "Unknown":
		return VMPhaseUnknown, nil
	default:
		log.Printf("Warning: Unknown VMI phase '%s', mapping to Unknown", phase)
		return VMPhaseUnknown, nil
	}
}

// analyzeVMConditions analyzes VM conditions to determine phase
func analyzeVMConditions(conditions []interface{}) VMPhase {
	hasFailure := false
	hasReady := false

	for _, condInterface := range conditions {
		condition, ok := condInterface.(map[string]interface{})
		if !ok {
			continue
		}

		condType, found := condition["type"]
		if !found {
			continue
		}

		status, found := condition["status"]
		if !found {
			continue
		}

		typeStr, ok := condType.(string)
		if !ok {
			continue
		}

		statusStr, ok := status.(string)
		if !ok {
			continue
		}

		switch typeStr {
		case "Ready":
			if statusStr == "True" {
				hasReady = true
			}
		case "Failure", "Failed":
			if statusStr == "True" {
				hasFailure = true
			}
		case "Paused":
			if statusStr == "True" {
				return VMPhaseStopped // Treat paused as stopped
			}
		}
	}

	if hasFailure {
		return VMPhaseFailed
	}
	if hasReady {
		return VMPhaseRunning
	}

	return VMPhaseUnknown
}
