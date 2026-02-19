package v1alpha1

import (
	catalogv1alpha1 "github.com/dcm-project/catalog-manager/api/v1alpha1/servicetypes/vm"
	"github.com/dcm-project/kubevirt-service-provider/internal/api/server"
)

// serverVMToVMSpec converts our API's server.VM type to the external catalogv1alpha1.VMSpec type
func serverVMToVMSpec(serverVM *server.VM) *catalogv1alpha1.VMSpec {
	if serverVM == nil {
		return nil
	}

	// Build external VMSpec
	vmSpec := &catalogv1alpha1.VMSpec{}

	// Convert VCPU
	vmSpec.Vcpu = catalogv1alpha1.Vcpu{
		Count: serverVM.Vcpu.Count,
	}

	// Convert Memory
	vmSpec.Memory = catalogv1alpha1.Memory{
		Size: serverVM.Memory.Size,
	}

	// Convert GuestOS
	vmSpec.GuestOS = catalogv1alpha1.GuestOS{
		Type: serverVM.GuestOS.Type,
	}

	// Convert Storage
	vmSpec.Storage = catalogv1alpha1.Storage{}
	for _, disk := range serverVM.Storage.Disks {
		vmSpec.Storage.Disks = append(vmSpec.Storage.Disks, catalogv1alpha1.Disk{
			Name:     disk.Name,
			Capacity: disk.Capacity,
		})
	}

	// Convert Access (optional)
	if serverVM.Access != nil {
		vmSpec.Access = &catalogv1alpha1.Access{}
		if serverVM.Access.SshPublicKey != nil {
			vmSpec.Access.SshPublicKey = serverVM.Access.SshPublicKey
		}
	}

	// Note: Metadata, ServiceType, and ProviderHints use external reference types
	// that we can't directly instantiate. For now, we'll leave them as default values
	// and let the mapper handle the minimal conversion needed for KubeVirt.
	// In a production environment, you'd need to properly map these fields.

	return vmSpec
}

// vmSpecToServerVM converts the external catalogv1alpha1.VMSpec type to our API's server.VM type.
// server.VM is a type alias for catalogv1alpha1.VMSpec, so we build using catalog types.
func vmSpecToServerVM(vmSpec *catalogv1alpha1.VMSpec, name string, path *string) *server.VM {
	if vmSpec == nil {
		return nil
	}

	// server.VM = catalogv1alpha1.VMSpec; use catalog types for all nested structs
	vm := &catalogv1alpha1.VMSpec{
		Vcpu: catalogv1alpha1.Vcpu{
			Count: vmSpec.Vcpu.Count,
		},
		Memory: catalogv1alpha1.Memory{
			Size: vmSpec.Memory.Size,
		},
		GuestOS: catalogv1alpha1.GuestOS{
			Type: vmSpec.GuestOS.Type,
		},
		Storage: catalogv1alpha1.Storage{
			Disks: vmSpec.Storage.Disks,
		},
	}
	if vmSpec.Access != nil {
		vm.Access = &catalogv1alpha1.Access{
			SshPublicKey: vmSpec.Access.SshPublicKey,
		}
	}
	// Metadata, ServiceType, ProviderHints: leave as zero value or set defaults if needed
	vm.ServiceType = "vm"

	return (*server.VM)(vm)
}
