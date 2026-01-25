package v1alpha1

import (
	"context"

	"github.com/dcm-project/kubevirt-service-provider/internal/api/server"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("KubevirtHandler", func() {
	var handler *KubevirtHandler

	BeforeEach(func() {
		handler = NewKubevirtHandler()
	})

	Describe("GetHealth", func() {
		It("should return a successful health response with correct status and path", func() {
			ctx := context.Background()
			response, err := handler.GetHealth(ctx, server.GetHealthRequestObject{})

			Expect(err).NotTo(HaveOccurred())
			Expect(response).NotTo(BeNil())

			healthResponse, ok := response.(server.GetHealth200JSONResponse)
			Expect(ok).To(BeTrue(), "response should be GetHealth200JSONResponse")

			Expect(healthResponse.Status).NotTo(BeNil())
			Expect(*healthResponse.Status).To(Equal("ok"))

			Expect(healthResponse.Path).NotTo(BeNil())
			Expect(*healthResponse.Path).To(Equal("/api/v1alpha1/health"))
		})
	})
})
