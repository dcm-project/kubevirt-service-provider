package kubevirt_test

import (
	"testing"

	catalogv1alpha1 "github.com/dcm-project/catalog-manager/api/v1alpha1/servicetypes/vm"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/dcm-project/kubevirt-service-provider/internal/kubevirt"
)

func TestMapper(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Mapper Suite")
}

var _ = Describe("Mapper", func() {
	var mapper *kubevirt.Mapper

	BeforeEach(func() {
		mapper = kubevirt.NewMapper("default")
	})

	Describe("VMSpecToVirtualMachine", func() {
		It("should convert a basic VMSpec to VirtualMachine without errors", func() {
			vmSpec := &catalogv1alpha1.VMSpec{
				GuestOS: catalogv1alpha1.GuestOS{
					Type: "ubuntu",
				},
				Vcpu: catalogv1alpha1.Vcpu{
					Count: 2,
				},
				Memory: catalogv1alpha1.Memory{
					Size: "2Gi",
				},
				Storage: catalogv1alpha1.Storage{
					Disks: []catalogv1alpha1.Disk{
						{
							Name: "boot",
						},
					},
				},
			}

			vm, err := mapper.VMSpecToVirtualMachine(vmSpec, "test-vm", "00000000-0000-0000-0000-000000000001")

			Expect(err).NotTo(HaveOccurred())
			Expect(vm).NotTo(BeNil())

			// Check basic metadata
			Expect(vm.GetName()).To(Equal("test-vm"))
			Expect(vm.GetNamespace()).To(Equal("default"))
			Expect(vm.GetAPIVersion()).To(Equal("kubevirt.io/v1"))
			Expect(vm.GetKind()).To(Equal("VirtualMachine"))
		})

		It("should handle empty storage with default boot disk", func() {
			vmSpec := &catalogv1alpha1.VMSpec{
				GuestOS: catalogv1alpha1.GuestOS{
					Type: "cirros",
				},
				Vcpu: catalogv1alpha1.Vcpu{
					Count: 1,
				},
				Memory: catalogv1alpha1.Memory{
					Size: "1Gi",
				},
				Storage: catalogv1alpha1.Storage{
					Disks: []catalogv1alpha1.Disk{},
				},
			}

			vm, err := mapper.VMSpecToVirtualMachine(vmSpec, "minimal-vm", "00000000-0000-0000-0000-000000000002")

			Expect(err).NotTo(HaveOccurred())
			Expect(vm).NotTo(BeNil())
			Expect(vm.GetName()).To(Equal("minimal-vm"))
		})
	})
})