package v1alpha1

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	types "github.com/dcm-project/kubevirt-service-provider/api/v1alpha1"
)

// mockVMClient implements VMClient for testing.
type mockVMClient struct {
	createFn      func(ctx context.Context, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error)
	getFn         func(ctx context.Context, vmID string) (*kubevirtv1.VirtualMachine, error)
	listFn        func(ctx context.Context, options metav1.ListOptions) ([]kubevirtv1.VirtualMachine, error)
	deleteFn      func(ctx context.Context, vmID string) error
	updateFn      func(ctx context.Context, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error)
	checkHealthFn func(ctx context.Context) error
}

func (m *mockVMClient) CreateVirtualMachine(ctx context.Context, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	if m.createFn != nil {
		return m.createFn(ctx, vm)
	}
	return nil, fmt.Errorf("createFn not set")
}

func (m *mockVMClient) GetVirtualMachine(ctx context.Context, vmID string) (*kubevirtv1.VirtualMachine, error) {
	if m.getFn != nil {
		return m.getFn(ctx, vmID)
	}
	return nil, fmt.Errorf("getFn not set")
}

func (m *mockVMClient) ListVirtualMachines(ctx context.Context, options metav1.ListOptions) ([]kubevirtv1.VirtualMachine, error) {
	if m.listFn != nil {
		return m.listFn(ctx, options)
	}
	return nil, fmt.Errorf("listFn not set")
}

func (m *mockVMClient) DeleteVirtualMachine(ctx context.Context, vmID string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, vmID)
	}
	return fmt.Errorf("deleteFn not set")
}

func (m *mockVMClient) UpdateVirtualMachine(ctx context.Context, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	if m.updateFn != nil {
		return m.updateFn(ctx, vm)
	}
	return nil, fmt.Errorf("updateFn not set")
}

func (m *mockVMClient) CheckHealth(ctx context.Context) error {
	if m.checkHealthFn != nil {
		return m.checkHealthFn(ctx)
	}
	return nil
}

// mockVMMapper implements VMMapper for testing.
type mockVMMapper struct {
	vmSpecToVMFn func(vmSpec *types.VMSpec, vmID string) (*kubevirtv1.VirtualMachine, error)
	vmToVMSpecFn func(vm *kubevirtv1.VirtualMachine) (*types.VMSpec, error)
}

func (m *mockVMMapper) VMSpecToVirtualMachine(vmSpec *types.VMSpec, vmID string) (*kubevirtv1.VirtualMachine, error) {
	if m.vmSpecToVMFn != nil {
		return m.vmSpecToVMFn(vmSpec, vmID)
	}
	return nil, fmt.Errorf("vmSpecToVMFn not set")
}

func (m *mockVMMapper) VirtualMachineToVMSpec(vm *kubevirtv1.VirtualMachine) (*types.VMSpec, error) {
	if m.vmToVMSpecFn != nil {
		return m.vmToVMSpecFn(vm)
	}
	return nil, fmt.Errorf("vmToVMSpecFn not set")
}
