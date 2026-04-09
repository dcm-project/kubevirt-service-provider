package registration

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	spmv1alpha1 "github.com/dcm-project/service-provider-manager/api/v1alpha1/provider"
	spmclient "github.com/dcm-project/service-provider-manager/pkg/client/provider"
	"github.com/google/uuid"

	"github.com/dcm-project/kubevirt-service-provider/internal/config"
)

var errNonRetryable = errors.New("non-retryable")

// Option configures a Registrar.
type Option func(*Registrar)

// SetInitialBackoff sets the initial retry backoff interval.
func SetInitialBackoff(d time.Duration) Option {
	return func(r *Registrar) {
		r.initialBackoff = d
	}
}

// SetMaxBackoff sets the maximum retry backoff interval.
func SetMaxBackoff(d time.Duration) Option {
	return func(r *Registrar) {
		r.maxBackoff = d
	}
}

// Registrar handles registration with the DCM Service Provider Manager
type Registrar struct {
	client         *spmclient.ClientWithResponses
	providerCfg    *config.ProviderConfig
	initialBackoff time.Duration
	maxBackoff     time.Duration
	startOnce      sync.Once
	done           chan struct{}
}

// NewRegistrar creates a new Registrar with the given configuration
func NewRegistrar(providerCfg *config.ProviderConfig, svcMgrCfg *config.ServiceProviderManagerConfig, opts ...Option) (*Registrar, error) {
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

	r := &Registrar{
		client:         client,
		providerCfg:    providerCfg,
		initialBackoff: 1 * time.Second,
		maxBackoff:     60 * time.Second,
		done:           make(chan struct{}),
	}
	for _, opt := range opts {
		opt(r)
	}

	return r, nil
}

// Start begins the registration process in the background.
// Multiple calls are safe; only the first launches a goroutine.
func (r *Registrar) Start(ctx context.Context) {
	r.startOnce.Do(func() {
		go func() {
			defer close(r.done)
			r.run(ctx)
		}()
	})
}

// Done returns a channel that is closed when the registration goroutine
// has completed (either after successful registration or context cancellation).
func (r *Registrar) Done() <-chan struct{} {
	return r.done
}

func (r *Registrar) run(ctx context.Context) {
	backoff := r.initialBackoff

	for {
		if err := r.register(ctx); err == nil {
			return
		} else if errors.Is(err, errNonRetryable) {
			log.Printf("Registration failed with non-retryable error, giving up: %v", err)
			return
		} else {
			log.Printf("Registration failed, will retry: %v", err)
		}

		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}

		backoff *= 2
		if backoff > r.maxBackoff {
			backoff = r.maxBackoff
		}
	}
}

func (r *Registrar) register(ctx context.Context) error {
	providerUUID, err := uuid.Parse(r.providerCfg.ID)
	if err != nil {
		return fmt.Errorf("invalid provider ID %q: %v: %w", r.providerCfg.ID, err, errNonRetryable)
	}

	providerID := providerUUID.String()
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
		return fmt.Errorf("conflict registering provider: %s: %w", resp.ApplicationproblemJSON409.Title, errNonRetryable)
	case http.StatusBadRequest:
		return fmt.Errorf("validation error: %s: %w", resp.ApplicationproblemJSON400.Title, errNonRetryable)
	default:
		sc := resp.StatusCode()
		if sc >= 400 && sc < 500 {
			return fmt.Errorf("registration returned non-retryable status %d: %w", sc, errNonRetryable)
		}
		return fmt.Errorf("unexpected response status: %d", sc)
	}

	return nil
}
