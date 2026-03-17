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
	"github.com/nats-io/nats.go/jetstream"
)

// VMEvent represents a VM status event
type VMEvent struct {
	Id        string    `json:"id"`
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
}

// Publisher handles NATS JetStream event publishing with CloudEvents formatting
type Publisher struct {
	natsConn     *nats.Conn
	js           jetstream.JetStream
	natsURL      string
	subject      string
	maxReconnect int
}

// PublisherConfig contains configuration for the event publisher
type PublisherConfig struct {
	NATSURL      string
	Subject      string
	MaxReconnect int
}

// NewPublisher creates a new NATS JetStream publisher
func NewPublisher(config PublisherConfig) (*Publisher, error) {
	p := &Publisher{
		natsURL:      config.NATSURL,
		subject:      config.Subject,
		maxReconnect: config.MaxReconnect,
	}

	if err := p.connect(); err != nil {
		return nil, fmt.Errorf("failed to create NATS publisher: %w", err)
	}

	return p, nil
}

// connect establishes connection to NATS server and sets up JetStream
func (p *Publisher) connect() error {
	opts := []nats.Option{
		nats.ReconnectWait(2 * time.Second),
		nats.MaxReconnects(p.maxReconnect),
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

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return fmt.Errorf("failed to create JetStream context: %w", err)
	}
	p.js = js

	log.Printf("Connected to NATS, publishing to subject %q", p.subject)
	return nil
}

// PublishVMEvent publishes a VM phase change event to NATS JetStream
func (p *Publisher) PublishVMEvent(ctx context.Context, vmEvent VMEvent) error {
	if !p.IsConnected() {
		return fmt.Errorf("NATS connection not available")
	}

	// Create CloudEvent
	event := cloudevents.NewEvent()
	event.SetID(uuid.New().String())
	event.SetType("dcm.status.vm")
	event.SetSource("kubevirt.localhost") // TODO: change to the actual source
	event.SetSubject(p.subject)
	event.SetTime(vmEvent.Timestamp)

	if err := event.SetData(cloudevents.ApplicationJSON, vmEvent); err != nil {
		return fmt.Errorf("failed to set CloudEvent data: %w", err)
	}

	eventData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal CloudEvent: %w", err)
	}

	// Publish to JetStream with acknowledgement
	_, err = p.js.Publish(ctx, p.subject, eventData)
	if err != nil {
		return fmt.Errorf("failed to publish event to JetStream: %w", err)
	}

	log.Printf("Successfully published VM event for %s to JetStream subject %s", vmEvent.Id, p.subject)
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
