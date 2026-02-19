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
		// For testing purposes, use nil dependencies
		// In real tests, these would be mocked
		handler = NewKubevirtHandler(nil, nil)
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

	Describe("CreateVM", func() {
		It("should return validation error when request body is nil", func() {
			ctx := context.Background()
			response, err := handler.CreateVM(ctx, server.CreateVMRequestObject{
				Body: nil,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(response).NotTo(BeNil())

			errorResponse, ok := response.(*server.CreateVMdefaultApplicationProblemPlusJSONResponse)
			Expect(ok).To(BeTrue(), "response should be CreateVMdefaultApplicationProblemPlusJSONResponse")
			Expect(errorResponse.StatusCode).To(Equal(400))
		})
	})

	Describe("DeleteVM", func() {
		It("should exist and be callable", func() {
			// This test exists to ensure the method signature is correct
			// Actual functionality testing requires a real KubeVirt client
			Expect(handler.DeleteVM).NotTo(BeNil())
		})
	})

	Describe("GetVM", func() {
		It("should exist and be callable", func() {
			// This test exists to ensure the method signature is correct
			// Actual functionality testing requires a real KubeVirt client
			Expect(handler.GetVM).NotTo(BeNil())
		})
	})

	Describe("ApplyVM", func() {
		It("should exist and be callable", func() {
			// This test exists to ensure the method signature is correct
			// Actual functionality testing requires a real KubeVirt client
			Expect(handler.ApplyVM).NotTo(BeNil())
		})
	})
})
