package v1alpha1

import (
	"context"
	"fmt"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/dcm-project/kubevirt-service-provider/internal/api/server"
	"github.com/dcm-project/kubevirt-service-provider/internal/constants"
	"github.com/dcm-project/kubevirt-service-provider/internal/kubevirt"
)

const (
	ApiPrefix = "/api/v1alpha1/"
)

type KubevirtHandler struct {
	kubevirtClient *kubevirt.Client
	mapper         *kubevirt.Mapper
}

func NewKubevirtHandler(kubevirtClient *kubevirt.Client, mapper *kubevirt.Mapper) *KubevirtHandler {
	return &KubevirtHandler{
		kubevirtClient: kubevirtClient,
		mapper:         mapper,
	}
}

// vmIDToName converts a UUID to a Kubernetes-compatible VM name
func (s *KubevirtHandler) vmIDToName(vmID openapi_types.UUID) string {
	return fmt.Sprintf("vm-%s", strings.ReplaceAll(vmID.String(), "-", "")[:8])
}

// kubevirtVMToServerVM converts a typed KubeVirt VM to the API server.VM type.
// It extracts the DCM instance ID from spec.template.metadata.labels for the resource path.
func kubevirtVMToServerVM(s *KubevirtHandler, vm *kubevirtv1.VirtualMachine) (*server.VM, error) {
	if vm.Name == "" {
		return nil, fmt.Errorf("VM missing metadata.name")
	}
	vmSpec, err := s.mapper.VirtualMachineToVMSpec(vm)
	if err != nil {
		return nil, err
	}
	var path *string
	var vmID string
	if vm.Spec.Template != nil {
		if id, ok := vm.Spec.Template.ObjectMeta.Labels[constants.DCMLabelInstanceID]; ok && id != "" {
			vmID = id
			p := fmt.Sprintf("%svms/%s", ApiPrefix, vmID)
			path = &p
		}
	}
	return vmSpecToServerVM(vmSpec, path, vmID), nil
}

// (GET /health)
func (s *KubevirtHandler) GetHealth(ctx context.Context, request server.GetHealthRequestObject) (server.GetHealthResponseObject, error) {
	status := "ok"
	path := fmt.Sprintf("%shealth", ApiPrefix)
	return server.GetHealth200JSONResponse{
		Status: &status,
		Path:   &path,
	}, nil
}

// (GET /vms)
func (s *KubevirtHandler) ListVMs(ctx context.Context, request server.ListVMsRequestObject) (server.ListVMsResponseObject, error) {
	listOptions := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", constants.DCMLabelManagedBy, constants.DCMManagedByValue),
	}
	list, err := s.kubevirtClient.ListVirtualMachines(ctx, listOptions)
	if err != nil {
		return kubevirt.MapKubernetesErrorForList(err), nil
	}
	vms := make([]server.VM, 0, len(list))
	for i := range list {
		serverVM, err := kubevirtVMToServerVM(s, &list[i])
		if err != nil {
			// Skip VMs that fail to convert (e.g. missing required data)
			continue
		}
		vms = append(vms, *serverVM)
	}
	return server.ListVMs200JSONResponse{Vms: &vms}, nil
}

// (POST /vms)
func (s *KubevirtHandler) CreateVM(ctx context.Context, request server.CreateVMRequestObject) (server.CreateVMResponseObject, error) {
	vmSpec := request.Body
	vmID := request.Params.Id.String()
	path := fmt.Sprintf("%svms/%s", ApiPrefix, vmID)

	// Check for existing VM (idempotency support)
	existingVM, err := s.kubevirtClient.GetVirtualMachine(ctx, vmID)
	if err == nil && existingVM != nil {
		// VM already exists: derive ID from labels and ensure response reflects stored state
		existingVMID := s.extractVMIDFromVM(existingVM)
		if existingVMID != "" {
			vmID = existingVMID
		}

		// Handle potential name collision when client did not provide explicit ID
		if request.Params.Id == nil && existingVMID != vmID {
			// Name collision with different VM, return conflict
			status := 409
			title := "Conflict"
			typ := "about:blank"
			detail := fmt.Sprintf("Virtual machine name collision detected for %s", vmID)
			return &server.CreateVMdefaultApplicationProblemPlusJSONResponse{
				Body: server.Error{
					Title:  title,
					Type:   typ,
					Status: &status,
					Detail: &detail,
				},
				StatusCode: status,
			}, nil
		}

		// Convert existing VM back to VMSpec and return 200 OK for idempotent create
		vmSpec, err := s.mapper.VirtualMachineToVMSpec(existingVM)
		if err != nil {
			body, statusCode := kubevirt.InternalServerError(fmt.Sprintf("Failed to convert existing VM: %v", err))
			return &server.CreateVMdefaultApplicationProblemPlusJSONResponse{
				Body:       body,
				StatusCode: statusCode,
			}, nil
		}

		// Ensure the spec has the effective ID and path
		serverVM := vmSpecToServerVM(vmSpec, &path, vmID)

		// Return 201 Created for existing VM (idempotent create)
		// Note: OpenAPI spec only defines 201 response, ideally would be 200 for existing resources
		return server.CreateVM201JSONResponse(*serverVM), nil
	}
	// If error is not "not found", handle it
	if err != nil && !kubevirt.IsNotFoundError(err) {
		return kubevirt.MapKubernetesError(err), nil
	}

	// Convert VMSpec to KubeVirt VirtualMachine
	catalogVMSpec := createVMRequestToVMSpec(vmSpec)
	virtualMachine, err := s.mapper.VMSpecToVirtualMachine(catalogVMSpec, vmID)
	if err != nil {
		status := 422
		title := "Validation Error"
		typ := "about:blank"
		detail := fmt.Sprintf("Failed to convert VMSpec to VirtualMachine: %v", err)
		return &server.CreateVMdefaultApplicationProblemPlusJSONResponse{
			Body: server.Error{
				Title:  title,
				Type:   typ,
				Status: &status,
				Detail: &detail,
			},
			StatusCode: status,
		}, nil
	}

	// Create the VirtualMachine in Kubernetes cluster
	createdVM, err := s.kubevirtClient.CreateVirtualMachine(ctx, virtualMachine)
	if err != nil {
		return kubevirt.MapKubernetesError(err), nil
	}

	// Successfully created VM
	if createdVM != nil {
		serverVM, err := kubevirtVMToServerVM(s, createdVM)
		if err != nil {
			return kubevirt.MapKubernetesError(err), nil
		}
		serverVM.Path = &path
		return server.CreateVM201JSONResponse(*serverVM), nil
	}

	// Fallback error
	body, statusCode := kubevirt.InternalServerError("Failed to create virtual machine")
	return &server.CreateVMdefaultApplicationProblemPlusJSONResponse{
		Body:       body,
		StatusCode: statusCode,
	}, nil
}

// (DELETE /vms/{vmId})
func (s *KubevirtHandler) DeleteVM(ctx context.Context, request server.DeleteVMRequestObject) (server.DeleteVMResponseObject, error) {
	// Check if VM exists
	_, err := s.kubevirtClient.GetVirtualMachine(ctx, request.VmId.String())
	if err != nil {
		if kubevirt.IsNotFoundError(err) {
			// VM not found, return 404
			status := 404
			title := "Not Found"
			typ := "about:blank"
			detail := fmt.Sprintf("Virtual machine with ID %s not found", request.VmId.String())
			return server.DeleteVM404ApplicationProblemPlusJSONResponse{
				Title:  title,
				Type:   typ,
				Status: &status,
				Detail: &detail,
			}, nil
		}
		// Other errors
		return kubevirt.MapKubernetesErrorForDelete(err), nil
	}

	// Delete the VM
	err = s.kubevirtClient.DeleteVirtualMachine(ctx, request.VmId.String())
	if err != nil {
		return kubevirt.MapKubernetesErrorForDelete(err), nil
	}

	// Successfully deleted
	return server.DeleteVM204Response{}, nil
}

// (GET /vms/{vmId})
func (s *KubevirtHandler) GetVM(ctx context.Context, request server.GetVMRequestObject) (server.GetVMResponseObject, error) {
	// Convert VM ID to name
	vmName := s.vmIDToName(request.VmId)

	// Get the VM from Kubernetes
	vm, err := s.kubevirtClient.GetVirtualMachine(ctx, vmName)
	if err != nil {
		if kubevirt.IsNotFoundError(err) {
			// VM not found, return 404
			status := 404
			title := "Not Found"
			typ := "about:blank"
			detail := fmt.Sprintf("Virtual machine with ID %s not found", request.VmId.String())
			return server.GetVM404ApplicationProblemPlusJSONResponse{
				Title:  title,
				Type:   typ,
				Status: &status,
				Detail: &detail,
			}, nil
		}
		// Other errors
		return kubevirt.MapKubernetesErrorForGet(err), nil
	}

	// Convert KubeVirt VirtualMachine back to VMSpec
	vmSpec, err := s.mapper.VirtualMachineToVMSpec(vm)
	if err != nil {
		body, statusCode := kubevirt.InternalServerError(fmt.Sprintf("Failed to convert VirtualMachine to VMSpec: %v", err))
		return server.GetVMdefaultApplicationProblemPlusJSONResponse{
			Body:       body,
			StatusCode: statusCode,
		}, nil
	}

	// Convert VMSpec back to server VM and return
	vmID := request.VmId.String()
	path := fmt.Sprintf("%svms/%s", ApiPrefix, vmID)
	serverVM := vmSpecToServerVM(vmSpec, &path, vmID)
	return server.GetVM200JSONResponse(*serverVM), nil
}

// (PUT /vms/{vmId})
func (s *KubevirtHandler) ApplyVM(ctx context.Context, request server.ApplyVMRequestObject) (server.ApplyVMResponseObject, error) {
	// Return not implemented
	status := 501
	title := "Not Implemented"
	typ := "about:blank"
	detail := "Applying VMs is not implemented"
	return server.ApplyVM400ApplicationProblemPlusJSONResponse{
		Title:  title,
		Type:   typ,
		Status: &status,
		Detail: &detail,
	}, nil
}

// extractVMIDFromVM extracts the DCM instance ID from a KubeVirt VM object
func (s *KubevirtHandler) extractVMIDFromVM(vm *kubevirtv1.VirtualMachine) string {
	// First check main metadata labels
	if vmID, found := vm.Labels[constants.DCMLabelInstanceID]; found && vmID != "" {
		return vmID
	}

	// Then check template metadata labels (for VMs created before label propagation fix)
	if vm.Spec.Template != nil {
		if vmID, found := vm.Spec.Template.ObjectMeta.Labels[constants.DCMLabelInstanceID]; found && vmID != "" {
			return vmID
		}
	}

	return ""
}
