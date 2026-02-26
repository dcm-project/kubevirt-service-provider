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
	case "VirtualMachineInstance":
		return extractVMIPhase(obj)
	default:
		return VMPhaseUnknown, fmt.Errorf("unsupported object kind: %s", kind)
	}
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
