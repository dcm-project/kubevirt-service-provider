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

	logger.Info("Successfully created VM application. ", "VM: ", vm.ID)
	return server.CreateVM201JSONResponse{Id: &vm.ID, Name: &vm.RequestInfo.VMName, Namespace: &vm.RequestInfo.Namespace}, nil
}

// GetVM (GET /api/v1/vm/{id})
func (s *ServiceHandler) GetVM(ctx context.Context, request server.GetVMRequestObject) (server.GetVMResponseObject, error) {
	logger := zap.S().Named("handler")
	logger.Info("Retrieving provider: ", "ID: ", request)

	return server.GetVM200JSONResponse{}, nil
}

// GetVM (GET /api/v1/vm)
func (s *ServiceHandler) ListVM(ctx context.Context, request server.ListVMRequestObject) (server.ListVMResponseObject, error) {
	logger := zap.S().Named("handler")
	logger.Info("Retrieving provider: ", "ID: ", request)

	return server.ListVM200JSONResponse{}, nil
}

// ApplyVM (PUT /api/v1/vm)
func (s *ServiceHandler) ApplyVM(ctx context.Context, request server.ApplyVMRequestObject) (server.ApplyVMResponseObject, error) {
	logger := zap.S().Named("handler")
	logger.Info("Retrieving provider: ", "ID: ", request)

	return server.ApplyVM201JSONResponse{}, nil
}

// DeleteVM (DELETE /v1/vm)
func (s *ServiceHandler) DeleteVM(ctx context.Context, request server.DeleteVMRequestObject) (server.DeleteVMResponseObject, error) {
	logger := zap.S().Named("service-provider")
	logger.Info("Deleting Application. ", "VM: ", request)

	appID := &request.Id
	declaredVM, err := s.vmService.DeleteVMApplication(ctx, appID)
	if err != nil {
		logger.Error("Failed to Delete VM application")
		return nil, err
	}
	logger.Info("Successfully deleted VM application. ", "VM: ", appID)
	return server.DeleteVM204JSONResponse{
		Id:        appID,
		Name:      &declaredVM.RequestInfo.VMName,
		Namespace: &declaredVM.RequestInfo.Namespace}, nil
}
