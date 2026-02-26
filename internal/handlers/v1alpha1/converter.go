package v1alpha1

import (
	"encoding/json"

	types "github.com/dcm-project/kubevirt-service-provider/api/v1alpha1"
	"github.com/dcm-project/kubevirt-service-provider/internal/api/server"
)

// serverVMToVMSpec converts our API's server.VM type to the types.VMSpec type (spec only, no path)
func serverVMToVMSpec(serverVM *server.VM) *types.VMSpec {
	if serverVM == nil {
		return nil
	}

	// Convert between the two identical structures via JSON marshaling
	data, err := json.Marshal(serverVM)
	if err != nil {
		return nil
	}

	var vmSpec types.VMSpec
	if err := json.Unmarshal(data, &vmSpec); err != nil {
		return nil
	}

	return &vmSpec
}

// serverVMToVM converts our API's server.VM type to the types.VM type (full resource)
func serverVMToVM(serverVM *server.VM) *types.VM {
	if serverVM == nil {
		return nil
	}

	// Convert between the two identical structures via JSON marshaling
	data, err := json.Marshal(serverVM)
	if err != nil {
		return nil
	}

	var vm types.VM
	if err := json.Unmarshal(data, &vm); err != nil {
		return nil
	}

	return &vm
}

func vmSpecToServerVM(vmSpec *types.VMSpec, path *string) *server.VM {
	if vmSpec == nil {
		return nil
	}

	// Convert between the two structures via JSON marshaling
	data, err := json.Marshal(vmSpec)
	if err != nil {
		return nil
	}

	var serverVM server.VM
	if err := json.Unmarshal(data, &serverVM); err != nil {
		return nil
	}

	serverVM.Path = path
	return &serverVM
}

func vmToServerVM(vm *types.VM, path *string) *server.VM {
	if vm == nil {
		return nil
	}

	// Convert between the two identical structures via JSON marshaling
	data, err := json.Marshal(vm)
	if err != nil {
		return nil
	}

	var serverVM server.VM
	if err := json.Unmarshal(data, &serverVM); err != nil {
		return nil
	}

	// Path should already be set in the VM, but allow override
	if path != nil {
		serverVM.Path = path
	}
	return &serverVM
}

// createVMRequestToVMSpec converts CreateVMJSONRequestBody to VMSpec
func createVMRequestToVMSpec(createVM *server.CreateVMJSONRequestBody) *types.VMSpec {
	if createVM == nil {
		return nil
	}

	// Convert via JSON marshaling to ensure compatibility
	data, err := json.Marshal(createVM)
	if err != nil {
		return nil
	}

	var vmSpec types.VMSpec
	if err := json.Unmarshal(data, &vmSpec); err != nil {
		return nil
	}

	return &vmSpec
}
