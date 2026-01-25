package apiserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/dcm-project/kubevirt-service-provider/api/v1alpha1"
	"github.com/dcm-project/kubevirt-service-provider/internal/api/server"
	"github.com/dcm-project/kubevirt-service-provider/internal/config"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

const gracefulShutdownTimeout = 5 * time.Second

type Server struct {
	cfg      *config.Config
	listener net.Listener
	handler  server.StrictServerInterface
}

func New(cfg *config.Config, listener net.Listener, handler server.StrictServerInterface) *Server {
	return &Server{
		cfg:      cfg,
		listener: listener,
		handler:  handler,
	}
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

	server.HandlerFromMuxWithBaseURL(
		server.NewStrictHandler(s.handler, nil),
		router,
		baseURL,
	)

	srv := http.Server{Handler: router}

	go func() {
		<-ctx.Done()
		ctxTimeout, cancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
		defer cancel()
		srv.SetKeepAlivesEnabled(false)
		_ = srv.Shutdown(ctxTimeout)
	}()

	if err := srv.Serve(s.listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
