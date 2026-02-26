package kubevirt_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/dcm-project/kubevirt-service-provider/api/v1alpha1"
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
			vmSpec := &v1alpha1.VMSpec{
				ServiceType: v1alpha1.Vm,
				Metadata: v1alpha1.ServiceMetadata{
					Name: "test-vm",
				},
				GuestOs: v1alpha1.GuestOS{
					Type: "ubuntu",
				},
				Vcpu: v1alpha1.Vcpu{
					Count: 2,
				},
				Memory: v1alpha1.Memory{
					Size: "2Gi",
				},
				Storage: v1alpha1.Storage{
					Disks: []v1alpha1.Disk{
						{
							Name:     "boot",
							Capacity: "10Gi",
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
			vmSpec := &v1alpha1.VMSpec{
				ServiceType: v1alpha1.Vm,
				Metadata: v1alpha1.ServiceMetadata{
					Name: "minimal-vm",
				},
				GuestOs: v1alpha1.GuestOS{
					Type: "cirros",
				},
				Vcpu: v1alpha1.Vcpu{
					Count: 1,
				},
				Memory: v1alpha1.Memory{
					Size: "1Gi",
				},
				Storage: v1alpha1.Storage{
					Disks: []v1alpha1.Disk{},
				},
			}

			vm, err := mapper.VMSpecToVirtualMachine(vmSpec, "minimal-vm", "00000000-0000-0000-0000-000000000002")

			Expect(err).NotTo(HaveOccurred())
			Expect(vm).NotTo(BeNil())
			Expect(vm.GetName()).To(Equal("minimal-vm"))
		})
	})
})