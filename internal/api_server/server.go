package apiserver

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/dcm-project/kubevirt-service-provider/api/v1alpha1"
	"github.com/dcm-project/kubevirt-service-provider/internal/api/server"
	"github.com/dcm-project/kubevirt-service-provider/internal/config"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"
)

const gracefulShutdownTimeout = 5 * time.Second

const readinessProbeTimeout = 5 * time.Second

const readinessProbeInterval = 50 * time.Millisecond

type Server struct {
	cfg      *config.Config
	listener net.Listener
	handler  server.StrictServerInterface
	onReady  func(context.Context)
}

func New(cfg *config.Config, listener net.Listener, handler server.StrictServerInterface) *Server {
	return &Server{
		cfg:      cfg,
		listener: listener,
		handler:  handler,
	}
}

// WithOnReady registers a callback invoked once the server is confirmed to be
// serving HTTP requests. The server verifies readiness by polling its own
// health endpoint before calling fn.
func (s *Server) WithOnReady(fn func(context.Context)) *Server {
	s.onReady = fn
	return s
}

func (s *Server) Run(ctx context.Context) error {
	router := chi.NewRouter()
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)

	swagger, err := v1alpha1.GetSwagger()
	if err != nil {
		return fmt.Errorf("failed to load swagger spec: %w", err)
	}

	baseURL := ""
	if len(swagger.Servers) > 0 {
		baseURL = swagger.Servers[0].URL
	}

	// Create a copy of the swagger spec for validation that preserves server context
	validationSwagger := *swagger

	// Add OpenAPI request validation middleware with server context
	router.Use(nethttpmiddleware.OapiRequestValidatorWithOptions(&validationSwagger, &nethttpmiddleware.Options{
		Options: openapi3filter.Options{
			AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
		},
		SilenceServersWarning: true,
		ErrorHandler: func(w http.ResponseWriter, message string, statusCode int) {
			log.Printf("OpenAPI validation error (status %d): %s", statusCode, message)
			http.Error(w, message, statusCode)
		},
	}))

	server.HandlerFromMuxWithBaseURL(
		server.NewStrictHandler(s.handler, nil),
		router,
		baseURL,
	)

	srv := http.Server{Handler: router}

	serveCh := make(chan error, 1)
	go func() {
		if err := srv.Serve(s.listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveCh <- err
		}
		close(serveCh)
	}()

	if s.onReady != nil {
		if err := s.waitForReady(ctx, s.listener.Addr().String()); err != nil {
			log.Printf("Readiness probe failed, skipping onReady callback: %v", err)
		} else {
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("onReady callback panicked: %v", r)
					}
				}()
				s.onReady(ctx)
			}()
		}
	}

	select {
	case <-ctx.Done():
	case err := <-serveCh:
		if err != nil {
			return err
		}
	}

	ctxTimeout, cancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
	defer cancel()
	srv.SetKeepAlivesEnabled(false)
	if err := srv.Shutdown(ctxTimeout); err != nil {
		log.Printf("Error during server shutdown: %v", err)
	}

	return nil
}

func (s *Server) waitForReady(ctx context.Context, addr string) error {
	url := fmt.Sprintf("http://%s/api/v1alpha1/vms/health", addr)
	client := &http.Client{Timeout: 1 * time.Second}

	deadline := time.NewTimer(readinessProbeTimeout)
	defer deadline.Stop()

	ticker := time.NewTicker(readinessProbeInterval)
	defer ticker.Stop()

	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
		if err != nil {
			return fmt.Errorf("creating readiness probe request: %w", err)
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("server readiness probe timed out after %s", readinessProbeTimeout)
		case <-ticker.C:
		}
	}
}
