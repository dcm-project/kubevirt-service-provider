package apiserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	api "github.com/dcm-project/service-provider-api/api/v1alpha1"
	"github.com/dcm-project/service-provider-api/internal/api/server"
	"github.com/dcm-project/service-provider-api/internal/config"
	handlers "github.com/dcm-project/service-provider-api/internal/handlers/v1alpha1"
	"github.com/dcm-project/service-provider-api/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-resty/resty/v2"
	oapimiddleware "github.com/oapi-codegen/nethttp-middleware"
	httpSwagger "github.com/swaggo/http-swagger"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	gracefulShutdownTimeout = 5 * time.Second
)

type Server struct {
	cfg      *config.Config
	listener net.Listener
}

// New returns a new instance of a migration-planner server.
func New(
	cfg *config.Config,

	listener net.Listener,
) *Server {
	return &Server{
		cfg:      cfg,
		listener: listener,
	}
}

func oapiErrorHandler(w http.ResponseWriter, message string, statusCode int) {
	http.Error(w, fmt.Sprintf("API Error: %s", message), statusCode)
}

func (s *Server) Run(ctx context.Context) error {
	zap.S().Named("api_server").Info("Initializing API server")
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

	h := handlers.NewServiceHandler(
		service.NewVMService(),
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
	go func() {
		zap.S().Named("api_server").Infof("Listening on %s...", s.listener.Addr().String())
		if err := srv.Serve(s.listener); err != nil && !errors.Is(err, net.ErrClosed) {
			serverErr <- err
		}
	}()

	// Wait a moment for the server to start listening, then register with service provider API
	go func() {
		time.Sleep(100 * time.Millisecond)
		if err := s.registerWithDCMServiceProviderAPI(ctx); err != nil {
			zap.S().Named("api_server").Errorw("Failed to register with service provider API", "error", err)
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

	payload := map[string]interface{}{
		"apiHost":     s.cfg.Service.BaseUrl,
		"description": "KubeVirt Virtual Machine Service Provider",
		"endpoint":    "/v1/vm",
		"id":          "123e4567-e89b-12d3-a456-426614174220",
		"name":        "kubevirt-service-provider",
		"operations":  []string{"GET", "PUT", "POST", "DELETE"},
		"type":        "virtual_machine",
	}
	restyClient := resty.New()
	result, err := restyClient.R().
		SetBody(payload).
		Post(fmt.Sprintf("%s%s", s.cfg.Service.RegistryUrl, s.cfg.Service.RegistryEndpoint))

	if err != nil {
		return fmt.Errorf("failed to register with DCM service provider API: %w", err)
	}

	if result.IsError() {
		return fmt.Errorf("external service returned error status: %d, response: %s", result.StatusCode(), result.String())
	}
	if result.StatusCode() == http.StatusCreated {
		zap.S().Named("api_server").Info("Successfully registered with DCM service provider API")
	}

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
