package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	apiserver "github.com/dcm-project/kubevirt-service-provider/internal/api_server"
	"github.com/dcm-project/kubevirt-service-provider/internal/config"
	"github.com/dcm-project/kubevirt-service-provider/internal/events"
	handlers "github.com/dcm-project/kubevirt-service-provider/internal/handlers/v1alpha1"
	"github.com/dcm-project/kubevirt-service-provider/internal/kubevirt"
	"github.com/dcm-project/kubevirt-service-provider/internal/monitor"
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

	// Initialize KubeVirt client
	kubevirtClient, err := kubevirt.NewClient(cfg.KubernetesConfig)
	if err != nil {
		log.Fatalf("Failed to create KubeVirt client: %v", err)
	}

	// Initialize mapper
	mapper := kubevirt.NewMapper(cfg.KubernetesConfig.Namespace)

	// Initialize event monitoring if enabled
	var monitorService *monitor.Service
	if cfg.EventConfig.Enabled {
		log.Printf("Initializing event monitoring service")

		// Initialize NATS publisher
		publisherConfig := events.PublisherConfig{
			NATSURL:      cfg.NATSConfig.URL,
			Timeout:      cfg.NATSConfig.Timeout,
			MaxReconnect: cfg.NATSConfig.MaxReconnect,
		}
		publisher, err := events.NewPublisher(publisherConfig)
		if err != nil {
			log.Fatalf("Failed to create event publisher: %v", err)
		}

		// Initialize monitoring service
		monitorConfig := monitor.MonitorConfig{
			Namespace:    cfg.KubernetesConfig.Namespace,
			ResyncPeriod: cfg.EventConfig.ResyncPeriod,
		}
		monitorService = monitor.NewMonitorService(kubevirtClient.DynamicClient(), publisher, monitorConfig)

		log.Printf("Event monitoring service initialized")
	}

	// Create handler with dependencies
	handler := handlers.NewKubevirtHandler(kubevirtClient, mapper)

	srv := apiserver.New(cfg, listener, handler)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Start monitoring service if enabled
	var wg sync.WaitGroup
	if monitorService != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Printf("Starting VM monitoring service")
			if err := monitorService.Run(ctx); err != nil {
				log.Printf("Monitoring service error: %v", err)
			}
		}()
	}

	log.Printf("Starting server on %s", listener.Addr().String())

	// Start server
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := srv.Run(ctx); err != nil {
			log.Printf("Server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-ctx.Done()
	log.Printf("Shutdown signal received, waiting for services to stop...")

	// Wait for all services to stop gracefully
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	// Wait up to 10 seconds for graceful shutdown
	select {
	case <-done:
		log.Printf("All services stopped gracefully")
	case <-time.After(10 * time.Second):
		log.Printf("Shutdown timeout exceeded")
	}
}
