package v1alpha1

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	types "github.com/dcm-project/kubevirt-service-provider/api/v1alpha1"
)

// VMClient defines the operations the handler needs from a KubeVirt client.
type VMClient interface {
	CreateVirtualMachine(ctx context.Context, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error)
	GetVirtualMachine(ctx context.Context, vmID string) (*kubevirtv1.VirtualMachine, error)
	ListVirtualMachines(ctx context.Context, options metav1.ListOptions) ([]kubevirtv1.VirtualMachine, error)
	DeleteVirtualMachine(ctx context.Context, vmID string) error
	UpdateVirtualMachine(ctx context.Context, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error)
}

// VMMapper defines the operations the handler needs for VM spec conversion.
type VMMapper interface {
	VMSpecToVirtualMachine(vmSpec *types.VMSpec, vmID string) (*kubevirtv1.VirtualMachine, error)
	VirtualMachineToVMSpec(vm *kubevirtv1.VirtualMachine) (*types.VMSpec, error)
}
