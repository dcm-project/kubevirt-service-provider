package kubevirt_test

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8sv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

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

			vm, err := mapper.VMSpecToVirtualMachine(vmSpec, "00000000-0000-0000-0000-000000000001")

			Expect(err).NotTo(HaveOccurred())
			Expect(vm).NotTo(BeNil())

			// Check basic metadata
			Expect(vm.GenerateName).To(Equal("dcm-"))
			Expect(vm.Namespace).To(Equal("default"))
			Expect(vm.TypeMeta.APIVersion).To(Equal("kubevirt.io/v1"))
			Expect(vm.TypeMeta.Kind).To(Equal("VirtualMachine"))
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

			vm, err := mapper.VMSpecToVirtualMachine(vmSpec, "00000000-0000-0000-0000-000000000002")

			Expect(err).NotTo(HaveOccurred())
			Expect(vm).NotTo(BeNil())
			Expect(vm.Spec.Template.Spec.Domain.Devices.Disks).To(HaveLen(1))
			Expect(vm.Spec.Template.Spec.Domain.Devices.Disks[0].Name).To(Equal("boot"))
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

			vm, err := mapper.VMSpecToVirtualMachine(vmSpec, "00000000-0000-0000-0000-000000000003")
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

// kubevirtVMWithContainerDisk builds a typed VirtualMachine with the given container disk image, CPU count and memory.
func kubevirtVMWithContainerDisk(containerImage string, cpuCount int, memorySize string) *kubevirtv1.VirtualMachine {
	if cpuCount == 0 {
		cpuCount = 1
	}
	bootOrder := uint(1)
	running := true

	return &kubevirtv1.VirtualMachine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "kubevirt.io/v1",
			Kind:       "VirtualMachine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vm",
			Namespace: "default",
		},
		Spec: kubevirtv1.VirtualMachineSpec{
			Running: &running,
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Domain: kubevirtv1.DomainSpec{
						Resources: kubevirtv1.ResourceRequirements{
							Requests: k8sv1.ResourceList{
								k8sv1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%d", cpuCount)),
								k8sv1.ResourceMemory: resource.MustParse(memorySize),
							},
						},
						Devices: kubevirtv1.Devices{
							Disks: []kubevirtv1.Disk{
								{
									Name: "boot",
									DiskDevice: kubevirtv1.DiskDevice{
										Disk: &kubevirtv1.DiskTarget{
											Bus: kubevirtv1.DiskBusVirtio,
										},
									},
									BootOrder: &bootOrder,
								},
							},
						},
					},
					Volumes: []kubevirtv1.Volume{
						{
							Name: "boot",
							VolumeSource: kubevirtv1.VolumeSource{
								ContainerDisk: &kubevirtv1.ContainerDiskSource{
									Image: containerImage,
								},
							},
						},
					},
				},
			},
		},
	}
}
