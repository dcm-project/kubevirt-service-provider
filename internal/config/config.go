package config

import (
	"time"

	"github.com/kelseyhightower/envconfig"
)

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

// KubernetesConfig holds configuration for connecting to Kubernetes/KubeVirt
type KubernetesConfig struct {
	// Kubeconfig path for connecting to Kubernetes cluster (optional, defaults to in-cluster)
	Kubeconfig string `envconfig:"KUBERNETES_KUBECONFIG"`
	// Namespace for creating VMs
	Namespace string `envconfig:"KUBERNETES_NAMESPACE" default:"default"`
	// Timeout for Kubernetes API requests
	Timeout time.Duration `envconfig:"KUBERNETES_TIMEOUT" default:"60s"`
	// MaxRetries for failed operations
	MaxRetries int `envconfig:"KUBERNETES_MAX_RETRIES" default:"3"`
}

// NATSConfig holds configuration for NATS connection
type NATSConfig struct {
	// URL is the NATS server URL
	URL string `envconfig:"NATS_URL" default:"nats://localhost:4222"`
	// Timeout for NATS operations
	Timeout time.Duration `envconfig:"NATS_TIMEOUT" default:"10s"`
	// MaxReconnect attempts (-1 for unlimited)
	MaxReconnect int `envconfig:"NATS_MAX_RECONNECT" default:"-1"`
	// ReconnectWait time between reconnect attempts
	ReconnectWait time.Duration `envconfig:"NATS_RECONNECT_WAIT" default:"2s"`
}

// EventConfig holds configuration for event monitoring
type EventConfig struct {
	// Enabled controls whether event monitoring is active
	Enabled bool `envconfig:"EVENTS_ENABLED" default:"true"`
	// ResyncPeriod for Kubernetes informers
	ResyncPeriod time.Duration `envconfig:"EVENTS_RESYNC_PERIOD" default:"30m"`
}

type Config struct {
	ProviderConfig               *ProviderConfig
	ServiceProviderManagerConfig *ServiceProviderManagerConfig
	KubernetesConfig            *KubernetesConfig
	NATSConfig                  *NATSConfig
	EventConfig                 *EventConfig
}

func Load() (*Config, error) {
	cfg := &Config{}
	if err := envconfig.Process("", cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
