package kubevirt_test

import (
	"strconv"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

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

	Describe("VirtualMachineToVMSpec", func() {
		It("should convert a VirtualMachine back to VMSpec with correct CPU, memory, guest OS and disks", func() {
			vmSpec := &v1alpha1.VMSpec{
				ServiceType: v1alpha1.Vm,
				Metadata: v1alpha1.ServiceMetadata{
					Name: "roundtrip-vm",
				},
				GuestOs: v1alpha1.GuestOS{
					Type: "ubuntu",
				},
				Vcpu: v1alpha1.Vcpu{
					Count: 4,
				},
				Memory: v1alpha1.Memory{
					Size: "4Gi",
				},
				Storage: v1alpha1.Storage{
					Disks: []v1alpha1.Disk{
						{Name: "boot", Capacity: "20Gi"},
						{Name: "data", Capacity: "10Gi"},
					},
				},
			}

			vm, err := mapper.VMSpecToVirtualMachine(vmSpec, "roundtrip-vm", "00000000-0000-0000-0000-000000000003")
			Expect(err).NotTo(HaveOccurred())
			Expect(vm).NotTo(BeNil())

			back, err := mapper.VirtualMachineToVMSpec(vm)
			Expect(err).NotTo(HaveOccurred())
			Expect(back).NotTo(BeNil())

			Expect(back.Vcpu.Count).To(Equal(4))
			Expect(back.Memory.Size).To(Equal("4Gi"))
			Expect(back.GuestOs.Type).To(Equal("ubuntu"))
			Expect(back.Storage.Disks).To(HaveLen(2))
			Expect(back.Storage.Disks[0].Name).To(Equal("boot"))
			Expect(back.Storage.Disks[1].Name).To(Equal("data"))
		})

		It("should infer guest OS from container disk image", func() {
			vm := kubevirtVMWithContainerDisk("quay.io/kubevirt/fedora-container-disk-demo:latest", 2, "2Gi")

			back, err := mapper.VirtualMachineToVMSpec(vm)
			Expect(err).NotTo(HaveOccurred())
			Expect(back).NotTo(BeNil())
			Expect(back.GuestOs.Type).To(Equal("fedora"))
			Expect(back.Vcpu.Count).To(Equal(2))
			Expect(back.Memory.Size).To(Equal("2Gi"))
		})

		It("should default to cirros and boot disk when VM has minimal or no domain data", func() {
			vm := kubevirtVMWithContainerDisk("quay.io/something/unknown:latest", 1, "1Gi")

			back, err := mapper.VirtualMachineToVMSpec(vm)
			Expect(err).NotTo(HaveOccurred())
			Expect(back).NotTo(BeNil())
			Expect(back.GuestOs.Type).To(Equal("cirros"))
			Expect(back.Storage.Disks).NotTo(BeEmpty())
			Expect(back.Storage.Disks[0].Name).To(Equal("boot"))
		})
	})
})

// kubevirtVMWithContainerDisk builds an unstructured VirtualMachine with the given container disk image, CPU count and memory.
func kubevirtVMWithContainerDisk(containerImage string, cpuCount int, memorySize string) *unstructured.Unstructured {
	cpuStr := strconv.Itoa(cpuCount)
	if cpuStr == "0" {
		cpuStr = "1"
	}
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata": map[string]interface{}{
				"name":      "test-vm",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"running": true,
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"domain": map[string]interface{}{
							"resources": map[string]interface{}{
								"requests": map[string]interface{}{
									"cpu":    cpuStr,
									"memory": memorySize,
								},
							},
							"devices": map[string]interface{}{
								"disks": []interface{}{
									map[string]interface{}{
										"name":      "boot",
										"disk":      map[string]interface{}{"bus": "virtio"},
										"bootOrder": float64(1),
									},
								},
							},
						},
						"volumes": []interface{}{
							map[string]interface{}{
								"name": "boot",
								"containerDisk": map[string]interface{}{
									"image": containerImage,
								},
							},
						},
					},
				},
			},
		},
	}
}
