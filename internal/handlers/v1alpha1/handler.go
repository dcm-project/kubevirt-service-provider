package v1alpha1

import (
	"context"

	"github.com/dcm-project/kubevirt-service-provider/internal/api/server"
	"github.com/dcm-project/kubevirt-service-provider/internal/service"
	"go.uber.org/zap"
)

type ServiceHandler struct {
	vmService *service.VMService
}

func NewServiceHandler(providerService *service.VMService) *ServiceHandler {
	return &ServiceHandler{
		vmService: providerService,
	}
}

// ListHealth (GET /health)
func (s *ServiceHandler) ListHealth(ctx context.Context, request server.ListHealthRequestObject) (server.ListHealthResponseObject, error) {
	return server.ListHealth200TextResponse("OK"), nil
}

// GetVMHealth (GET /api/v1/vm/health)
func (s *ServiceHandler) ListVmHealth(ctx context.Context, request server.ListVmHealthRequestObject) (server.ListVmHealthResponseObject, error) {
	return server.ListVmHealth200TextResponse("OK"), nil
}

// CreateVM (POST /api/v1/vm)
func (s *ServiceHandler) CreateVm(ctx context.Context, request server.CreateVmRequestObject) (server.CreateVmResponseObject, error) {
	logger := zap.S().Named("handler:create-vm")
	logger.Info("Creating virtual machine...")
	vm, err := s.vmService.CreateVM(ctx, *request.Body)
	if err != nil {
		return nil, err
	}

	logger.Info("Successfully created VM application. ", "VM: ", vm.Id)
	return server.CreateVm201JSONResponse(vm), nil
}

// ListVM (GET /api/v1/vm)
func (s *ServiceHandler) ListVms(ctx context.Context, request server.ListVmsRequestObject) (server.ListVmsResponseObject, error) {
	logger := zap.S().Named("handler: list-vm")

	var vms []server.VMInstance
	var err error

	logger.Info("No request ID provided, listing all VMs from database")
	vms, err = s.vmService.ListVMsFromDatabase(ctx)

	if err != nil {
		logger.Errorw("Failed to list VMs", "error", err)
		return server.ListVms500JSONResponse{
			Error: err.Error(),
		}, nil
	}

	logger.Infow("Successfully retrieved VMs", "count", len(vms))
	return server.ListVms200JSONResponse{NextPageToken: nil, Vms: vms}, nil
}

// GetVM (GET /api/v1/vm/{id})
func (s *ServiceHandler) GetVm(ctx context.Context, request server.GetVmRequestObject) (server.GetVmResponseObject, error) {
	logger := zap.S().Named("handler:get-vm")

	vmId := request.VmId.String()
	logger.Infow("Request ID provided, fetching VM from cluster", "id", vmId)
	vmInstance, err := s.vmService.GetVMFromCluster(ctx, vmId)

	if err != nil {
		logger.Errorw("Failed to list VMs", "error", err)
		return server.GetVm500JSONResponse{
			Error: err.Error(),
		}, nil
	}

	logger.Infow("Successfully retrieved VM", "Id", vmId)
	return server.GetVm200JSONResponse(vmInstance), nil
}

// DeleteVM (DELETE /api/v1/vm/{id})
func (s *ServiceHandler) DeleteVm(ctx context.Context, request server.DeleteVmRequestObject) (server.DeleteVmResponseObject, error) {
	logger := zap.S().Named("handler:delete-vm")
	vmId := request.VmId.String()
	logger.Infow("Request ID provided, deleting VM from cluster", "id", vmId)
	err := s.vmService.DeleteVMApplication(ctx, &vmId)

	if err != nil {
		logger.Errorw("Failed to delete VM", "error", err)
		return server.DeleteVm500JSONResponse{
			Error: err.Error(),
		}, nil
	}
	return server.DeleteVm204Response{}, nil
}
