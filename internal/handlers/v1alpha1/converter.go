package v1alpha1

import (
	"encoding/json"
	"fmt"

	types "github.com/dcm-project/kubevirt-service-provider/api/v1alpha1"
	"github.com/dcm-project/kubevirt-service-provider/internal/api/server"
)

func vmSpecToServerVM(vmSpec *types.VMSpec, path *string, id string) (*server.VM, error) {
	if vmSpec == nil {
		return nil, fmt.Errorf("vmSpec is nil")
	}

	data, err := json.Marshal(vmSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal VMSpec: %w", err)
	}

	var serverVM server.VM
	if err := json.Unmarshal(data, &serverVM); err != nil {
		return nil, fmt.Errorf("failed to unmarshal to server.VM: %w", err)
	}

	serverVM.Path = path
	return &serverVM, nil
}

// createVMRequestToVMSpec converts CreateVMJSONRequestBody to VMSpec
func createVMRequestToVMSpec(createVM *server.CreateVMJSONRequestBody) (*types.VMSpec, error) {
	if createVM == nil {
		return nil, fmt.Errorf("createVM request body is nil")
	}

	data, err := json.Marshal(createVM)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal create VM request: %w", err)
	}

	var vmSpec types.VMSpec
	if err := json.Unmarshal(data, &vmSpec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal to VMSpec: %w", err)
	}

	return &vmSpec, nil
}
