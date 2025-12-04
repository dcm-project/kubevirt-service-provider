package apiserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	api "github.com/dcm-project/kubevirt-service-provider/api/v1alpha1"
	"github.com/dcm-project/kubevirt-service-provider/internal/api/server"
	"github.com/dcm-project/kubevirt-service-provider/internal/config"
	handlers "github.com/dcm-project/kubevirt-service-provider/internal/handlers/v1alpha1"
	"github.com/dcm-project/kubevirt-service-provider/internal/service"
	"github.com/dcm-project/kubevirt-service-provider/internal/store"
	dcm "github.com/dcm-project/service-provider-api/pkg/registration/client"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	oapimiddleware "github.com/oapi-codegen/nethttp-middleware"
	"github.com/spf13/pflag"
	httpSwagger "github.com/swaggo/http-swagger"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	kubecli "kubevirt.io/client-go/kubecli"
)

const (
	gracefulShutdownTimeout = 5 * time.Second
)

type Server struct {
	cfg        *config.Config
	store      store.Store
	listener   net.Listener
	virtClient kubecli.KubevirtClient
	//vmStatusSync *service.VMStatusSyncService
}

// New returns a new instance of a migration-planner server.
// It initializes the KubeVirt client once and shares it across all services that need it.
func New(
	cfg *config.Config, store store.Store, listener net.Listener,
) *Server {
	// Create KubeVirt client once - shared by VMService and VMStatusSyncService
	virtClient, err := kubecli.GetKubevirtClientFromClientConfig(
		kubecli.DefaultClientConfig(&pflag.FlagSet{}),
	)
	if err != nil {
		zap.S().Fatalw("cannot obtain KubeVirt client", "error", err)
	}

	//vmStatusSync := service.NewVMStatusSyncService(virtClient, store)
	return &Server{
		cfg:        cfg,
		store:      store,
		listener:   listener,
		virtClient: virtClient,
		//vmStatusSync: vmStatusSync,
	}
}

func oapiErrorHandler(w http.ResponseWriter, message string, statusCode int) {
	http.Error(w, fmt.Sprintf("API Error: %s", message), statusCode)
}

func (s *Server) Run(ctx context.Context) error {
	zap.S().Named("api_server").Info("Initializing API server")

	// Start VM status sync job in background
	//go s.vmStatusSync.StartSyncJob(ctx)

	swagger, err := api.GetSwagger()
	if err != nil {
		return fmt.Errorf("failed to load swagger spec: %w", err)
	}
	// Skip server name validation
	swagger.Servers = nil

	oapiOpts := oapimiddleware.Options{
		ErrorHandler: oapiErrorHandler,
	}
	router := chi.NewRouter()

	router.Use(
		middleware.RequestID,
		middleware.Recoverer,
	)

	// Add Swagger UI endpoints BEFORE OpenAPI validation middleware
	router.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger.json"),
	))
	router.Get("/swagger.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		swaggerJSON, err := json.Marshal(swagger)
		if err != nil {
			http.Error(w, "Failed to marshal swagger spec", http.StatusInternalServerError)
			return
		}
		_, err = w.Write(swaggerJSON)
		if err != nil {
			return
		}
	})

	// Create VMService with shared KubeVirt client
	// The client is initialized once at server startup and injected into services
	h := handlers.NewServiceHandler(
		service.NewVMService(s.virtClient, s.store),
	)

	// Apply OpenAPI validation middleware to API routes only
	router.Group(func(r chi.Router) {
		r.Use(oapimiddleware.OapiRequestValidatorWithOptions(swagger, &oapiOpts))
		server.HandlerFromMux(server.NewStrictHandler(h, nil), router)
	})

	srv := http.Server{Addr: s.cfg.Service.Address, Handler: router}

	go func() {
		<-ctx.Done()
		zap.S().Named("api_server").Infof("Shutdown signal received: %s", ctx.Err())
		ctxTimeout, cancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
		defer cancel()

		srv.SetKeepAlivesEnabled(false)
		_ = srv.Shutdown(ctxTimeout)
	}()

	// Start server in a goroutine
	serverErr := make(chan error, 1)
	serverReady := make(chan bool, 1)
	go func() {
		zap.S().Named("api_server").Infof("Listening on %s...", s.listener.Addr().String())
		// Signal that server is starting - do this BEFORE Serve() because Serve() blocks until shutdown
		serverReady <- true
		if err := srv.Serve(s.listener); err != nil && !errors.Is(err, net.ErrClosed) {
			serverErr <- err
		}
	}()

	// Wait for server to be ready, then register with service provider API
	go func() {
		<-serverReady // Wait for server to start
		// Small delay to ensure server is fully ready after srv.Serve() is called
		time.Sleep(100 * time.Millisecond)
		if err := s.registerWithDCMServiceProviderAPI(ctx); err != nil {
			zap.S().Named("api_server").Errorw("Failed to register with DCM ", "error", err)
			// Note: We log the error but don't fail the server startup
		}
	}()

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		return nil
	case err := <-serverErr:
		return err
	}
}

// registerWithDCMServiceProviderAPI registers this service with DCM service provider API
func (s *Server) registerWithDCMServiceProviderAPI(ctx context.Context) error {
	zap.S().Named("api_server").Info("Registering with DCM service provider API...")

	// Use actual listener address for apiHost (most accurate, especially for dynamic ports)
	// Fallback to BaseUrl only if listener address is unavailable
	var apiHost string
	listenerAddr := s.listener.Addr().String()

	if listenerAddr != "" {
		// Parse listener address (e.g., "127.0.0.1:8082" or "[::]:8082")
		host, port, err := net.SplitHostPort(listenerAddr)
		if err != nil {
			// If we can't parse the listener address, fallback to BaseUrl
			zap.S().Named("api_server").Warnw("Failed to parse listener address, using BaseUrl", "listener", listenerAddr, "error", err)
			apiHost = s.cfg.Service.BaseUrl
		} else {
			// Determine scheme from BaseUrl config, default to http
			scheme := "http"
			if baseURL := s.cfg.Service.BaseUrl; baseURL != "" {
				if parsedURL, err := url.Parse(baseURL); err == nil && parsedURL.Scheme != "" {
					scheme = parsedURL.Scheme
				}
			}
			// Normalize host (handle 0.0.0.0, ::, IPv6 brackets)
			host = strings.Trim(host, "[]")
			if host == "0.0.0.0" || host == "::" || host == "" {
				// Use localhost for external access if bound to all interfaces
				host = "localhost"
			}
			apiHost = fmt.Sprintf("%s://%s:%s", scheme, host, port)
		}
	} else {
		// Listener address is empty, use BaseUrl
		apiHost = s.cfg.Service.BaseUrl
	}

	// Validate that we have a valid apiHost
	if apiHost == "" {
		return fmt.Errorf("apiHost cannot be empty: listener address unavailable and BaseUrl not configured")
	}

	registrar := dcm.New(dcm.Config{BaseURL: s.cfg.Service.RegistryUrl})

	request := &dcm.RegistrationRequest{
		ServiceID: "123e4567-e89b-12d3-a456-426614174222",
		Endpoint:  fmt.Sprintf("%s/api/v1/vm", apiHost),
		Metadata: dcm.Metadata{
			Zone:   "us-east-1b",
			Region: "us-east-1",
		},
		Operations: []string{"CREATE", "DELETE", "READ"},
	}

	result, err := registrar.Register(ctx, "virtual_machine", request)

	if err != nil {
		return fmt.Errorf("failed to register with DCM service provider API: %w", err)
	}
	zap.S().Named("api_server").Info("Successfully registered with DCM ", "Service ID ", result.ServiceID)

	return nil
}

func (s *Server) getKubeClient() (*kubernetes.Clientset, error) {
	kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(restConfig)
}
