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
