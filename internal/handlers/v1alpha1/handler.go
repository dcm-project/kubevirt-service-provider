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
	return server.ListHealth200Response{}, nil
}

// GetVMHealth (GET /api/v1/vm/health)
func (s *ServiceHandler) GetVMHealth(ctx context.Context, request server.GetVMHealthRequestObject) (server.GetVMHealthResponseObject, error) {
	return server.GetVMHealth200Response{}, nil
}

// CreateVM (POST /api/v1/vm)
func (s *ServiceHandler) CreateVM(ctx context.Context, request server.CreateVMRequestObject) (server.CreateVMResponseObject, error) {
	logger := zap.S().Named("handler:create-vm")
	logger.Info("Creating virtual machine...")
	vm, err := s.vmService.CreateVM(ctx, *request.Body)
	if err != nil {
		return nil, err
	}

	logger.Info("Successfully created VM application. ", "VM: ", vm.Id)
	return server.CreateVM201JSONResponse(vm), nil
}

// ListVM (GET /api/v1/vm)
func (s *ServiceHandler) ListVM(ctx context.Context, request server.ListVMRequestObject) (server.ListVMResponseObject, error) {
	logger := zap.S().Named("handler: list-vm")

	var vms []server.VMInstance
	var err error

	// Check if request ID is provided in the body
	if request.Params.Id != nil && request.Params.Id.String() != "" {
		logger.Infow("Request ID provided, fetching VM from cluster", "id", *request.Params.Id)
		vms, err = s.vmService.GetVMFromCluster(ctx, request.Params.Id.String())
	} else {
		logger.Info("No request ID provided, listing all VMs from database")
		vms, err = s.vmService.ListVMsFromDatabase(ctx)
	}

	if err != nil {
		logger.Errorw("Failed to list VMs", "error", err)
		return server.ListVM500JSONResponse{
			Error: err.Error(),
		}, nil
	}

	logger.Infow("Successfully retrieved VMs", "count", len(vms))
	return server.ListVM200JSONResponse(vms), nil
}
