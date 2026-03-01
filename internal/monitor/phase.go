package monitor

import (
	"fmt"
	"log"

	kubevirtv1 "kubevirt.io/api/core/v1"

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

// ExtractVMInfo extracts phase and identifying information from a VMI object
func ExtractVMInfo(vmi *kubevirtv1.VirtualMachineInstance) (VMInfo, error) {
	if vmi == nil {
		return VMInfo{}, fmt.Errorf("VMI object is nil")
	}

	return VMInfo{
		VMID:      vmi.Labels[constants.DCMLabelInstanceID],
		VMName:    vmi.Name,
		Namespace: vmi.Namespace,
		Phase:     mapVMIPhase(vmi.Status.Phase),
	}, nil
}

// mapVMIPhase maps KubeVirt VMI phase to our VMPhase constants
func mapVMIPhase(phase kubevirtv1.VirtualMachineInstancePhase) VMPhase {
	switch phase {
	case kubevirtv1.Pending:
		return VMPhasePending
	case kubevirtv1.Scheduling:
		return VMPhaseScheduling
	case kubevirtv1.Scheduled:
		return VMPhaseScheduled
	case kubevirtv1.Running:
		return VMPhaseRunning
	case kubevirtv1.Succeeded:
		return VMPhaseSucceeded
	case kubevirtv1.Failed:
		return VMPhaseFailed
	case kubevirtv1.Unknown:
		return VMPhaseUnknown
	default:
		log.Printf("Warning: Unknown VMI phase '%s', mapping to Unknown", phase)
		return VMPhaseUnknown
	}
}
