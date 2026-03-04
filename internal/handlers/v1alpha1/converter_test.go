package v1alpha1

import (
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/dcm-project/kubevirt-service-provider/internal/api/server"
)

var _ = Describe("Converters", func() {
	Describe("vmSpecToServerVM", func() {
		It("should return error for nil input", func() {
			result, err := vmSpecToServerVM(nil, nil, "")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("vmSpec is nil"))
			Expect(result).To(BeNil())
		})

		It("should set path and UUID on the result", func() {
			vmSpec := newTestVMSpec()
			vmID := "00000000-0000-0000-0000-000000000001"
			path := "/api/v1alpha1/vms/" + vmID

			result, err := vmSpecToServerVM(vmSpec, &path, vmID)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(*result.Path).To(Equal(path))
			Expect(result.Id).To(Equal(uuid.MustParse(vmID)))
		})

		It("should handle invalid UUID gracefully", func() {
			vmSpec := newTestVMSpec()
			path := "/api/v1alpha1/vms/not-a-uuid"

			result, err := vmSpecToServerVM(vmSpec, &path, "not-a-uuid")

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(*result.Path).To(Equal(path))
			// UUID stays zero when parse fails
			Expect(result.Id).To(Equal(uuid.UUID{}))
		})

		It("should handle nil path", func() {
			vmSpec := newTestVMSpec()
			vmID := "00000000-0000-0000-0000-000000000001"

			result, err := vmSpecToServerVM(vmSpec, nil, vmID)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Path).To(BeNil())
			Expect(result.Id).To(Equal(uuid.MustParse(vmID)))
		})
	})

	Describe("createVMRequestToVMSpec", func() {
		It("should return error for nil input", func() {
			result, err := createVMRequestToVMSpec(nil)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("createVM request body is nil"))
			Expect(result).To(BeNil())
		})

		It("should return a non-nil VMSpec for valid input", func() {
			body := &server.CreateVMJSONRequestBody{
				VmSpec: server.VMSpec{
					ServiceType: server.Vm,
					Metadata:    server.ServiceMetadata{Name: "test-vm"},
					GuestOs:     server.GuestOS{Type: "ubuntu"},
					Vcpu:        server.Vcpu{Count: 2},
					Memory:      server.Memory{Size: "2Gi"},
					Storage:     server.Storage{Disks: []server.Disk{{Name: "boot", Capacity: "10Gi"}}},
				},
			}

			result, err := createVMRequestToVMSpec(body)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
		})
	})
})
