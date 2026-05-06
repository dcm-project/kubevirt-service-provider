package kubevirt

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

var _ = Describe("Client", func() {
	Describe("CheckHealth", func() {
		It("should succeed when the K8s API server is reachable", func() {
			c := &Client{kubeClient: fake.NewSimpleClientset()}

			err := c.CheckHealth(context.Background())
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error when Discovery endpoint fails", func() {
			kubeClient := fake.NewSimpleClientset()
			kubeClient.PrependReactor("get", "version", func(_ k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, fmt.Errorf("simulated discovery failure")
			})
			c := &Client{kubeClient: kubeClient}

			err := c.CheckHealth(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("simulated discovery failure"))
		})
	})
})
