package registration

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	spmv1alpha1 "github.com/dcm-project/service-provider-manager/api/v1alpha1"
	spmclient "github.com/dcm-project/service-provider-manager/pkg/client"

	"github.com/dcm-project/kubevirt-service-provider/internal/config"
)

// Registrar handles registration with the DCM Service Provider Manager
type Registrar struct {
	client      *spmclient.ClientWithResponses
	providerCfg *config.ProviderConfig
}

// NewRegistrar creates a new Registrar with the given configuration
func NewRegistrar(providerCfg *config.ProviderConfig, svcMgrCfg *config.ServiceProviderManagerConfig) (*Registrar, error) {
	httpClient := &http.Client{
		Timeout: providerCfg.HTTPTimeout,
	}

	client, err := spmclient.NewClientWithResponses(
		svcMgrCfg.Endpoint,
		spmclient.WithHTTPClient(httpClient),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create DCM client: %w", err)
	}

	return &Registrar{
		client:      client,
		providerCfg: providerCfg,
	}, nil
}

// Register registers this provider with the DCM Service Provider Manager.
// Registration is idempotent: if a provider with the same ID exists, it will be updated.
func (r *Registrar) Register(ctx context.Context) error {
	// Parse the provider UUID
	providerUUID, err := uuid.Parse(r.providerCfg.ID)
	if err != nil {
		return fmt.Errorf("invalid provider ID %q: %w", r.providerCfg.ID, err)
	}

	providerID := openapi_types.UUID(providerUUID)
	params := &spmv1alpha1.CreateProviderParams{Id: &providerID}

	provider := spmv1alpha1.Provider{
		Name:          r.providerCfg.Name,
		Endpoint:      r.providerCfg.Endpoint,
		ServiceType:   r.providerCfg.ServiceType,
		SchemaVersion: r.providerCfg.SchemaVersion,
	}

	resp, err := r.client.CreateProviderWithResponse(ctx, params, provider)
	if err != nil {
		return fmt.Errorf("failed to register provider: %w", err)
	}

	switch resp.StatusCode() {
	case http.StatusCreated:
		log.Printf("Registered new provider: %s (ID: %s)", r.providerCfg.Name, *resp.JSON201.Id)
	case http.StatusOK:
		log.Printf("Updated existing provider: %s (ID: %s)", r.providerCfg.Name, *resp.JSON200.Id)
	case http.StatusConflict:
		return fmt.Errorf("conflict registering provider: %s", resp.ApplicationproblemJSON409.Title)
	case http.StatusBadRequest:
		return fmt.Errorf("validation error: %s", resp.ApplicationproblemJSON400.Title)
	default:
		return fmt.Errorf("unexpected response status: %d", resp.StatusCode())
	}

	return nil
}
