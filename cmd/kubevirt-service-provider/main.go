package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	apiserver "github.com/dcm-project/kubevirt-service-provider/internal/api_server"
	"github.com/dcm-project/kubevirt-service-provider/internal/config"
	handlers "github.com/dcm-project/kubevirt-service-provider/internal/handlers/v1alpha1"
	"github.com/dcm-project/kubevirt-service-provider/internal/registration"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	listener, err := net.Listen("tcp", cfg.ProviderConfig.ListenAddress)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// Register with DCM Service Provider Manager
	registrar, err := registration.NewRegistrar(cfg.ProviderConfig, cfg.ServiceProviderManagerConfig)
	if err != nil {
		log.Fatalf("Failed to create DCM registrar: %v", err)
	}

	regCtx, regCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer regCancel()

	if err := registrar.Register(regCtx); err != nil {
		log.Fatalf("Failed to register with DCM: %v", err)
	}

	srv := apiserver.New(cfg, listener, handlers.NewKubevirtHandler())

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	log.Printf("Starting server on %s", listener.Addr().String())
	if err := srv.Run(ctx); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
