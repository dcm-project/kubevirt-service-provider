package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

// VMEvent represents a VM status event
type VMEvent struct {
	VMID      string    `json:"vmId"`
	VMName    string    `json:"vmName"`
	Namespace string    `json:"namespace"`
	Phase     string    `json:"phase"`
	Timestamp time.Time `json:"timestamp"`
}

// Publisher handles NATS event publishing with CloudEvents formatting
type Publisher struct {
	natsConn     *nats.Conn
	natsURL      string
	timeout      time.Duration
	maxReconnect int
}

// PublisherConfig contains configuration for the event publisher
type PublisherConfig struct {
	NATSURL      string
	Timeout      time.Duration
	MaxReconnect int
}

// NewPublisher creates a new NATS publisher
func NewPublisher(config PublisherConfig) (*Publisher, error) {
	p := &Publisher{
		natsURL:      config.NATSURL,
		timeout:      config.Timeout,
		maxReconnect: config.MaxReconnect,
	}

	// Connect to NATS
	if err := p.connect(); err != nil {
		return nil, fmt.Errorf("failed to create NATS publisher: %w", err)
	}

	return p, nil
}

// connect establishes connection to NATS server
func (p *Publisher) connect() error {
	opts := []nats.Option{
		nats.ReconnectWait(2 * time.Second),
		nats.MaxReconnects(p.maxReconnect), // Use configured reconnect limit
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			log.Printf("NATS disconnected: %v", err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			log.Printf("NATS reconnected to %v", nc.ConnectedUrl())
		}),
		nats.ClosedHandler(func(nc *nats.Conn) {
			log.Printf("NATS connection closed")
		}),
	}

	nc, err := nats.Connect(p.natsURL, opts...)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}

	p.natsConn = nc
	return nil
}

// PublishVMEvent publishes a VM phase change event to NATS
func (p *Publisher) PublishVMEvent(ctx context.Context, vmEvent VMEvent) error {
	// Check if connected
	if p.natsConn == nil || !p.natsConn.IsConnected() {
		return fmt.Errorf("NATS connection not available")
	}

	// Create CloudEvent
	event := cloudevents.NewEvent()
	event.SetID(uuid.New().String())
	event.SetType("dcm.providers.kubevirt.vm.status")
	event.SetSource("kubevirt.localhost") // TODO: change to the actual source
	event.SetSubject(fmt.Sprintf("kubevirt.vm.%s", vmEvent.VMID))
	event.SetTime(vmEvent.Timestamp)

	// Set event data
	if err := event.SetData(cloudevents.ApplicationJSON, vmEvent); err != nil {
		return fmt.Errorf("failed to set CloudEvent data: %w", err)
	}

	// Convert to JSON for NATS publishing
	eventData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal CloudEvent: %w", err)
	}

	// Determine NATS subject (per-VM pattern)
	subject := fmt.Sprintf("kubevirt.vm.%s", vmEvent.VMID)

	// Publish to NATS
	if err := p.natsConn.Publish(subject, eventData); err != nil {
		return fmt.Errorf("failed to publish event to NATS: %w", err)
	}

	// Ensure message is flushed to server
	if err := p.natsConn.FlushTimeout(p.timeout); err != nil {
		return fmt.Errorf("failed to flush NATS message: %w", err)
	}

	log.Printf("Successfully published VM event for %s to NATS subject %s", vmEvent.VMID, subject)
	return nil
}

// Close gracefully closes the NATS connection
func (p *Publisher) Close() error {
	if p.natsConn != nil {
		p.natsConn.Close()
	}
	return nil
}

// IsConnected returns whether NATS connection is active
func (p *Publisher) IsConnected() bool {
	return p.natsConn != nil && p.natsConn.IsConnected()
}