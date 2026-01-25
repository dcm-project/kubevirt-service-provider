package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	apiserver "github.com/dcm-project/kubevirt-service-provider/internal/api_server"
	"github.com/dcm-project/kubevirt-service-provider/internal/config"
	handlers "github.com/dcm-project/kubevirt-service-provider/internal/handlers/v1alpha1"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	listener, err := net.Listen("tcp", cfg.ProviderConfig.Address)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	srv := apiserver.New(cfg, listener, handlers.NewKubevirtHandler())

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	log.Printf("Starting server on %s", listener.Addr().String())
	if err := srv.Run(ctx); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
