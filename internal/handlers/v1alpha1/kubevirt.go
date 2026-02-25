package v1alpha1

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

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

// unstructuredVMToServerVM converts an unstructured KubeVirt VM to the API server.VM type.
// It extracts the DCM instance ID from spec.template.metadata.labels for the resource path.
func unstructuredVMToServerVM(s *KubevirtHandler, vm *unstructured.Unstructured) (*server.VM, error) {
	vmName, found, err := unstructured.NestedString(vm.Object, "metadata", "name")
	if err != nil || !found || vmName == "" {
		return nil, fmt.Errorf("VM missing metadata.name")
	}
	vmSpec, err := s.mapper.VirtualMachineToVMSpec(vm)
	if err != nil {
		return nil, err
	}
	var path *string
	labels, _, _ := unstructured.NestedStringMap(vm.Object, "spec", "template", "metadata", "labels")
	if vmID, ok := labels[constants.DCMLabelInstanceID]; ok && vmID != "" {
		p := fmt.Sprintf("%svms/%s", ApiPrefix, vmID)
		path = &p
	}
	return vmSpecToServerVM(vmSpec, vmName, path), nil
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
	vms := make([]server.VM, 0, len(list.Items))
	for i := range list.Items {
		vm := &list.Items[i]
		serverVM, err := unstructuredVMToServerVM(s, vm)
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
	// Validate request body
	if request.Body == nil {
		status := 400
		title := "Bad Request"
		typ := "about:blank"
		detail := "Request body is required"
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

	vmSpec := request.Body

	// Generate VM name and ID
	var vmID string
	var vmName string

	// Use provided ID if available for idempotent creation
	if request.Params.Id != nil {
		vmID = request.Params.Id.String()
		vmName = fmt.Sprintf("vm-%s", strings.ReplaceAll(vmID, "-", "")[:8])
	} else {
		// Generate new UUID
		generatedID := uuid.New()
		vmID = generatedID.String()
		vmName = fmt.Sprintf("vm-%s", strings.ReplaceAll(vmID, "-", "")[:8])
	}

	// Check for existing VM (idempotency support)
	existingVM, err := s.kubevirtClient.GetVirtualMachine(ctx, vmName)
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
			detail := fmt.Sprintf("Virtual machine name collision detected for %s", vmName)
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
			status := 500
			title := "Internal Server Error"
			typ := "about:blank"
			detail := fmt.Sprintf("Failed to convert existing VM: %v", err)
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

		// Ensure the spec has the effective ID and path
		path := fmt.Sprintf("%svms/%s", ApiPrefix, vmID)
		serverVM := vmSpecToServerVM(vmSpec, vmName, &path)

		// Return 201 Created for existing VM (idempotent create)
		// Note: OpenAPI spec only defines 201 response, ideally would be 200 for existing resources
		return server.CreateVM201JSONResponse(*serverVM), nil
	}
	// If error is not "not found", handle it
	if err != nil && !kubevirt.IsNotFoundError(err) {
		return kubevirt.MapKubernetesError(err), nil
	}

	// Convert VMSpec to KubeVirt VirtualMachine
	catalogVMSpec := serverVMToVMSpec(vmSpec)
	virtualMachine, err := s.mapper.VMSpecToVirtualMachine(catalogVMSpec, vmName, vmID)
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
		return server.CreateVM201JSONResponse(*vmSpec), nil
	}

	// Fallback error
	status := 500
	title := "Internal Server Error"
	typ := "about:blank"
	detail := "Failed to create virtual machine"
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

// (DELETE /vms/{vmId})
func (s *KubevirtHandler) DeleteVM(ctx context.Context, request server.DeleteVMRequestObject) (server.DeleteVMResponseObject, error) {
	// Convert VM ID to name
	vmName := s.vmIDToName(request.VmId)

	// Check if VM exists
	_, err := s.kubevirtClient.GetVirtualMachine(ctx, vmName)
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
	err = s.kubevirtClient.DeleteVirtualMachine(ctx, vmName)
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
		status := 500
		title := "Internal Server Error"
		typ := "about:blank"
		detail := fmt.Sprintf("Failed to convert VirtualMachine to VMSpec: %v", err)
		return server.GetVMdefaultApplicationProblemPlusJSONResponse{
			Body: server.Error{
				Title:  title,
				Type:   typ,
				Status: &status,
				Detail: &detail,
			},
			StatusCode: status,
		}, nil
	}

	// Convert VMSpec back to server VM and return
	vmID := request.VmId.String()
	path := fmt.Sprintf("%svms/%s", ApiPrefix, vmID)
	serverVM := vmSpecToServerVM(vmSpec, vmName, &path)
	return server.GetVM200JSONResponse(*serverVM), nil
}

// (PUT /vms/{vmId})
func (s *KubevirtHandler) ApplyVM(ctx context.Context, request server.ApplyVMRequestObject) (server.ApplyVMResponseObject, error) {
	// Validate request body
	if request.Body == nil {
		status := 400
		title := "Bad Request"
		typ := "about:blank"
		detail := "Request body is required"
		return &server.ApplyVMdefaultApplicationProblemPlusJSONResponse{
			Body: server.Error{
				Title:  title,
				Type:   typ,
				Status: &status,
				Detail: &detail,
			},
			StatusCode: status,
		}, nil
	}

	vmSpec := request.Body

	// Convert VM ID to name
	vmName := s.vmIDToName(request.VmId)

	// Check if VM exists
	existingVM, err := s.kubevirtClient.GetVirtualMachine(ctx, vmName)
	if err != nil && !kubevirt.IsNotFoundError(err) {
		// Error other than "not found"
		return kubevirt.MapKubernetesErrorForApply(err), nil
	}

	// Convert VMSpec to KubeVirt VirtualMachine
	catalogVMSpec := serverVMToVMSpec(vmSpec)
	virtualMachine, err := s.mapper.VMSpecToVirtualMachine(catalogVMSpec, vmName, request.VmId.String())
	if err != nil {
		status := 422
		title := "Validation Error"
		typ := "about:blank"
		detail := fmt.Sprintf("Failed to convert VMSpec to VirtualMachine: %v", err)
		return &server.ApplyVMdefaultApplicationProblemPlusJSONResponse{
			Body: server.Error{
				Title:  title,
				Type:   typ,
				Status: &status,
				Detail: &detail,
			},
			StatusCode: status,
		}, nil
	}

	if existingVM != nil {
		// VM exists, update it
		updatedVM, err := s.kubevirtClient.UpdateVirtualMachine(ctx, virtualMachine)
		if err != nil {
			return kubevirt.MapKubernetesErrorForApply(err), nil
		}

		// Successfully updated VM, return 200
		if updatedVM != nil {
			return server.ApplyVM200JSONResponse(*vmSpec), nil
		}
	} else {
		// VM doesn't exist, create it
		createdVM, err := s.kubevirtClient.CreateVirtualMachine(ctx, virtualMachine)
		if err != nil {
			return kubevirt.MapKubernetesErrorForApply(err), nil
		}

		// Successfully created VM, return 200
		if createdVM != nil {
			return server.ApplyVM200JSONResponse(*vmSpec), nil
		}
	}

	// Fallback error
	status := 500
	title := "Internal Server Error"
	typ := "about:blank"
	detail := "Failed to apply virtual machine"
	return &server.ApplyVMdefaultApplicationProblemPlusJSONResponse{
		Body: server.Error{
			Title:  title,
			Type:   typ,
			Status: &status,
			Detail: &detail,
		},
		StatusCode: status,
	}, nil
}

// extractVMIDFromVM extracts the DCM instance ID from a KubeVirt VM object
func (s *KubevirtHandler) extractVMIDFromVM(vm *unstructured.Unstructured) string {
	// First check main metadata labels
	if labels := vm.GetLabels(); labels != nil {
		if vmID, found := labels[constants.DCMLabelInstanceID]; found && vmID != "" {
			return vmID
		}
	}

	// Then check template metadata labels (for VMs created before label propagation fix)
	if templateLabels, found, err := unstructured.NestedStringMap(vm.Object, "spec", "template", "metadata", "labels"); err == nil && found {
		if vmID, exists := templateLabels[constants.DCMLabelInstanceID]; exists && vmID != "" {
			return vmID
		}
	}

	return ""
}
