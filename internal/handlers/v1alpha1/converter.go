package v1alpha1

import (
	"encoding/json"

	types "github.com/dcm-project/kubevirt-service-provider/api/v1alpha1"
	"github.com/dcm-project/kubevirt-service-provider/internal/api/server"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func vmSpecToServerVM(vmSpec *types.VMSpec, path *string, id string) *server.VM {
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
	if parsed, err := uuid.Parse(id); err == nil {
		serverVM.Id = openapi_types.UUID(parsed)
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
