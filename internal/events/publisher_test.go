package events

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestEvents(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Events Suite")
}

var _ = Describe("Publisher", func() {
	Describe("IsConnected", func() {
		It("should return false when natsConn is nil", func() {
			p := &Publisher{}
			Expect(p.IsConnected()).To(BeFalse())
		})
	})

	Describe("Close", func() {
		It("should return no error when natsConn is nil", func() {
			p := &Publisher{}
			Expect(p.Close()).NotTo(HaveOccurred())
		})
	})

	Describe("PublishVMEvent", func() {
		It("should return not-connected error when natsConn is nil", func() {
			p := &Publisher{}
			err := p.PublishVMEvent(context.Background(), VMEvent{
				VMID:      "test-id",
				VMName:    "test-vm",
				Namespace: "default",
				Phase:     "Running",
				Timestamp: time.Now(),
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not available"))
		})
	})

	Describe("NewPublisher", func() {
		It("should return error when NATS server is unreachable", func() {
			_, err := NewPublisher(PublisherConfig{
				NATSURL:      "nats://127.0.0.1:14222",
				Timeout:      1 * time.Second,
				MaxReconnect: 0,
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create NATS publisher"))
		})
	})
})
