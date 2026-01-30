package v1alpha1

import (
	"context"
	"fmt"

	"github.com/dcm-project/kubevirt-service-provider/internal/api/server"
)

const (
	ApiPrefix = "/api/v1alpha1/"
)

type KubevirtHandler struct {
}

func NewKubevirtHandler() *KubevirtHandler {
	return &KubevirtHandler{}
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
	return server.ListVMs200JSONResponse{Vms: &[]server.VM{}}, nil
}

// (POST /vms)
func (s *KubevirtHandler) CreateVM(ctx context.Context, request server.CreateVMRequestObject) (server.CreateVMResponseObject, error) {
	status := 501
	title := "Not Implemented"
	typ := "about:blank"
	return &server.CreateVMdefaultApplicationProblemPlusJSONResponse{
		Body:       server.Error{Title: title, Type: typ, Status: &status},
		StatusCode: 501,
	}, nil
}

// (DELETE /vms/{vmId})
func (s *KubevirtHandler) DeleteVM(ctx context.Context, request server.DeleteVMRequestObject) (server.DeleteVMResponseObject, error) {
	return server.DeleteVM204Response{}, nil
}

// (GET /vms/{vmId})
func (s *KubevirtHandler) GetVM(ctx context.Context, request server.GetVMRequestObject) (server.GetVMResponseObject, error) {
	status := 404
	title := "Not Found"
	typ := "about:blank"
	return server.GetVM404ApplicationProblemPlusJSONResponse{
		Title: title,
		Type:  typ,
		Status: &status,
	}, nil
}

// (PUT /vms/{vmId})
func (s *KubevirtHandler) ApplyVM(ctx context.Context, request server.ApplyVMRequestObject) (server.ApplyVMResponseObject, error) {
	status := 501
	title := "Not Implemented"
	typ := "about:blank"
	return &server.ApplyVMdefaultApplicationProblemPlusJSONResponse{
		Body:       server.Error{Title: title, Type: typ, Status: &status},
		StatusCode: 501,
	}, nil
}
