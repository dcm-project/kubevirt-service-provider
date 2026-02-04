package config

import (
	"time"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	ProviderConfig               *ProviderConfig
	ServiceProviderManagerConfig *ServiceProviderManagerConfig
}

type ProviderConfig struct {
	ListenAddress string `envconfig:"PROVIDER_LISTEN_ADDRESS" default:"0.0.0.0:8081"`
	// Name is the name to register this provider as
	Name string `envconfig:"PROVIDER_NAME" default:"kubevirt-provider"`
	// Endpoint is the external endpoint where this provider can be reached
	Endpoint string `envconfig:"PROVIDER_ENDPOINT" default:"http://localhost:8081/api/v1alpha1"`
	// ServiceType is the type of service this provider offers
	ServiceType string `envconfig:"PROVIDER_SERVICE_TYPE" default:"vm"`
	// SchemaVersion is the API schema version
	SchemaVersion string `envconfig:"PROVIDER_SCHEMA_VERSION" default:"v1alpha1"`
	// ID is the ID of this provider
	ID string `envconfig:"PROVIDER_ID" default:"c9243c71-5ae0-4ee2-8a28-a83b3cb38d98"`
	// HTTPTimeout is the timeout for HTTP client requests
	HTTPTimeout time.Duration `envconfig:"PROVIDER_HTTP_TIMEOUT" default:"30s"`
}

// ServiceProviderManagerConfig holds configuration for registering with Service Provider Manager
type ServiceProviderManagerConfig struct {
	// Endpoint is the URL of the Service Manager API
	Endpoint string `envconfig:"SERVICE_MANAGER_ENDPOINT" default:"http://localhost:8080/api/v1alpha1"`
}

func Load() (*Config, error) {
	cfg := &Config{}
	if err := envconfig.Process("", cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
