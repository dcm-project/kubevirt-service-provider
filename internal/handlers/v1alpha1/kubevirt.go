package v1alpha1

import (
	"context"
	"fmt"
	"log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/dcm-project/kubevirt-service-provider/internal/api/server"
	"github.com/dcm-project/kubevirt-service-provider/internal/constants"
	"github.com/dcm-project/kubevirt-service-provider/internal/kubevirt"
)

const (
	APIPrefix = "/api/v1alpha1/"
)

type KubevirtHandler struct {
	kubevirtClient VMClient
	mapper         VMMapper
}

func NewKubevirtHandler(kubevirtClient VMClient, mapper VMMapper) *KubevirtHandler {
	return &KubevirtHandler{
		kubevirtClient: kubevirtClient,
		mapper:         mapper,
	}
}

// kubevirtVMToServerVM converts a typed KubeVirt VM to the API server.VM type.
// It extracts the DCM instance ID from spec.template.metadata.labels for the resource path.
func (s *KubevirtHandler) kubevirtVMToServerVM(vm *kubevirtv1.VirtualMachine) (*server.VM, error) {
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
			p := fmt.Sprintf("%svms/%s", APIPrefix, vmID)
			path = &p
		}
	}
	serverVM, err := vmSpecToServerVM(vmSpec, path, vmID)
	if err != nil {
		return nil, fmt.Errorf("failed to convert VMSpec to server VM: %w", err)
	}
	return serverVM, nil
}

// (GET /health)
func (s *KubevirtHandler) GetHealth(ctx context.Context, request server.GetHealthRequestObject) (server.GetHealthResponseObject, error) {
	status := "ok"
	path := fmt.Sprintf("%shealth", APIPrefix)
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
		serverVM, err := s.kubevirtVMToServerVM(&list[i])
		if err != nil {
			log.Printf("Warning: skipping VM %s: failed to convert: %v", list[i].Name, err)
			continue
		}
		vms = append(vms, *serverVM)
	}
	return server.ListVMs200JSONResponse{Vms: &vms}, nil
}

// (POST /vms)
func (s *KubevirtHandler) CreateVM(ctx context.Context, request server.CreateVMRequestObject) (server.CreateVMResponseObject, error) {
	vmSpec := request.Body
	vmID := *request.Params.Id
	path := fmt.Sprintf("%svms/%s", APIPrefix, vmID)

	log.Printf("CreateVM called: vmID=%s, body=%+v", vmID, vmSpec)

	// Convert VMSpec to KubeVirt VirtualMachine
	catalogVMSpec, err := createVMRequestToVMSpec(vmSpec)
	if err != nil {
		body, statusCode := kubevirt.InternalServerError(fmt.Sprintf("Failed to convert request: %v", err))
		return &server.CreateVMdefaultApplicationProblemPlusJSONResponse{
			Body:       body,
			StatusCode: statusCode,
		}, nil
	}

	virtualMachine, err := s.mapper.VMSpecToVirtualMachine(catalogVMSpec, vmID)
	if err != nil {
		body, statusCode := kubevirt.ValidationError(fmt.Sprintf("Failed to convert VMSpec to VirtualMachine: %v", err))
		return &server.CreateVMdefaultApplicationProblemPlusJSONResponse{
			Body:       body,
			StatusCode: statusCode,
		}, nil
	}

	// Create the VirtualMachine in Kubernetes cluster
	createdVM, err := s.kubevirtClient.CreateVirtualMachine(ctx, virtualMachine)
	if err != nil {
		return kubevirt.MapKubernetesError(err), nil
	}

	// Convert created VM back to response resource
	createdVMSpec, err := s.mapper.VirtualMachineToVMSpec(createdVM)
	if err != nil {
		body, statusCode := kubevirt.InternalServerError(fmt.Sprintf("Failed to convert created VM: %v", err))
		return &server.CreateVMdefaultApplicationProblemPlusJSONResponse{
			Body:       body,
			StatusCode: statusCode,
		}, nil
	}
	serverVM, err := vmSpecToServerVM(createdVMSpec, &path, vmID)
	if err != nil {
		body, statusCode := kubevirt.InternalServerError(fmt.Sprintf("Failed to convert VM spec: %v", err))
		return &server.CreateVMdefaultApplicationProblemPlusJSONResponse{
			Body:       body,
			StatusCode: statusCode,
		}, nil
	}
	return server.CreateVM201JSONResponse(*serverVM), nil
}

// (DELETE /vms/{vmId})
func (s *KubevirtHandler) DeleteVM(ctx context.Context, request server.DeleteVMRequestObject) (server.DeleteVMResponseObject, error) {
	// Delete the VM
	err := s.kubevirtClient.DeleteVirtualMachine(ctx, request.VmId.String())
	if err != nil {
		return kubevirt.MapKubernetesErrorForDelete(err), nil
	}

	return server.DeleteVM204Response{}, nil
}

// (GET /vms/{vmId})
func (s *KubevirtHandler) GetVM(ctx context.Context, request server.GetVMRequestObject) (server.GetVMResponseObject, error) {
	vmID := request.VmId.String()

	vm, err := s.kubevirtClient.GetVirtualMachine(ctx, vmID)
	if err != nil {
		if kubevirt.IsNotFoundError(err) {
			status := 404
			title := "Not Found"
			typ := "about:blank"
			detail := fmt.Sprintf("Virtual machine with ID %s not found", vmID)
			return server.GetVM404ApplicationProblemPlusJSONResponse{
				Title:  title,
				Type:   typ,
				Status: &status,
				Detail: &detail,
			}, nil
		}
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

	path := fmt.Sprintf("%svms/%s", APIPrefix, vmID)
	serverVM, err := vmSpecToServerVM(vmSpec, &path, vmID)
	if err != nil {
		body, statusCode := kubevirt.InternalServerError(fmt.Sprintf("Failed to convert VM spec: %v", err))
		return server.GetVMdefaultApplicationProblemPlusJSONResponse{
			Body:       body,
			StatusCode: statusCode,
		}, nil
	}
	return server.GetVM200JSONResponse(*serverVM), nil
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
