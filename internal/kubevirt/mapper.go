package kubevirt

import (
	"fmt"
	"strconv"
	"strings"

	k8sv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	types "github.com/dcm-project/kubevirt-service-provider/api/v1alpha1"
	"github.com/dcm-project/kubevirt-service-provider/internal/constants"
)

// Mapper handles conversion from VMSpec to KubeVirt VirtualMachine resources
type Mapper struct {
	namespace string
}

// NewMapper creates a new mapper instance
func NewMapper(namespace string) *Mapper {
	return &Mapper{
		namespace: namespace,
	}
}

// VMSpecToVirtualMachine converts a DCM VMSpec to a typed KubeVirt VirtualMachine
func (m *Mapper) VMSpecToVirtualMachine(vmSpec *types.VMSpec, vmID string) (*kubevirtv1.VirtualMachine, error) {
	running := true

	vm := &kubevirtv1.VirtualMachine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "kubevirt.io/v1",
			Kind:       "VirtualMachine",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "dcm-",
			Namespace:    m.namespace,
			Labels: map[string]string{
				constants.DCMLabelManagedBy:  constants.DCMManagedByValue,
				constants.DCMLabelInstanceID: vmID,
			},
		},
		Spec: kubevirtv1.VirtualMachineSpec{
			Running: &running,
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						constants.DCMLabelManagedBy:  constants.DCMManagedByValue,
						constants.DCMLabelInstanceID: vmID,
					},
				},
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Domain: kubevirtv1.DomainSpec{
						Devices:   m.buildDevices(vmSpec),
						Resources: m.buildResources(vmSpec),
						Machine: &kubevirtv1.Machine{
							Type: "q35",
						},
					},
					Networks: m.buildNetworks(),
					Volumes:  m.buildVolumes(vmSpec),
				},
			},
		},
	}

	return vm, nil
}

// buildDevices creates the device specification
func (m *Mapper) buildDevices(vmSpec *types.VMSpec) kubevirtv1.Devices {
	return kubevirtv1.Devices{
		Disks:      m.buildDisks(vmSpec),
		Interfaces: m.buildInterfaces(),
	}
}

// buildResources creates the resource specification
func (m *Mapper) buildResources(vmSpec *types.VMSpec) kubevirtv1.ResourceRequirements {
	requests := k8sv1.ResourceList{
		k8sv1.ResourceCPU: resource.MustParse(fmt.Sprintf("%d", vmSpec.Vcpu.Count)),
	}

	if memorySize, err := m.parseMemorySize(vmSpec.Memory.Size); err == nil {
		requests[k8sv1.ResourceMemory] = resource.MustParse(memorySize)
	}

	return kubevirtv1.ResourceRequirements{
		Requests: requests,
	}
}

// buildDisks creates the disk specifications
func (m *Mapper) buildDisks(vmSpec *types.VMSpec) []kubevirtv1.Disk {
	var disks []kubevirtv1.Disk

	for i, disk := range vmSpec.Storage.Disks {
		d := kubevirtv1.Disk{
			Name: disk.Name,
			DiskDevice: kubevirtv1.DiskDevice{
				Disk: &kubevirtv1.DiskTarget{
					Bus: kubevirtv1.DiskBusVirtio,
				},
			},
		}

		// Set as boot disk if this is the first disk or named "boot"
		if i == 0 || disk.Name == "boot" {
			bootOrder := uint(1)
			d.BootOrder = &bootOrder
		}

		disks = append(disks, d)
	}

	// If no disks defined, create a default boot disk
	if len(disks) == 0 {
		bootOrder := uint(1)
		disks = append(disks, kubevirtv1.Disk{
			Name: "boot",
			DiskDevice: kubevirtv1.DiskDevice{
				Disk: &kubevirtv1.DiskTarget{
					Bus: kubevirtv1.DiskBusVirtio,
				},
			},
			BootOrder: &bootOrder,
		})
	}

	return disks
}

// buildVolumes creates the volume specifications
func (m *Mapper) buildVolumes(vmSpec *types.VMSpec) []kubevirtv1.Volume {
	var volumes []kubevirtv1.Volume

	for i, disk := range vmSpec.Storage.Disks {
		vol := kubevirtv1.Volume{
			Name: disk.Name,
		}

		// For boot disk, use container disk for OS images
		if i == 0 || disk.Name == "boot" {
			vol.VolumeSource = kubevirtv1.VolumeSource{
				ContainerDisk: &kubevirtv1.ContainerDiskSource{
					Image: m.getContainerDiskImage(vmSpec.GuestOs),
				},
			}
		} else {
			// For data disks, create empty disk with default size
			vol.VolumeSource = kubevirtv1.VolumeSource{
				EmptyDisk: &kubevirtv1.EmptyDiskSource{
					Capacity: resource.MustParse("10Gi"),
				},
			}
		}

		volumes = append(volumes, vol)
	}

	// If no volumes defined, create a default boot volume
	if len(volumes) == 0 {
		volumes = append(volumes, kubevirtv1.Volume{
			Name: "boot",
			VolumeSource: kubevirtv1.VolumeSource{
				ContainerDisk: &kubevirtv1.ContainerDiskSource{
					Image: m.getContainerDiskImage(vmSpec.GuestOs),
				},
			},
		})
	}

	return volumes
}

// buildNetworks creates the network specifications. Must include a network
// named "default" (pod network) when using masquerade in domain.devices.interfaces.
func (m *Mapper) buildNetworks() []kubevirtv1.Network {
	return []kubevirtv1.Network{
		{
			Name: "default",
			NetworkSource: kubevirtv1.NetworkSource{
				Pod: &kubevirtv1.PodNetwork{},
			},
		},
	}
}

// buildInterfaces creates the network interface specifications. Interface names
// must match network names; masquerade is only valid with the pod network.
func (m *Mapper) buildInterfaces() []kubevirtv1.Interface {
	return []kubevirtv1.Interface{
		{
			Name:  "default",
			Model: "virtio",
			InterfaceBindingMethod: kubevirtv1.InterfaceBindingMethod{
				Masquerade: &kubevirtv1.InterfaceMasquerade{},
			},
		},
	}
}

// getContainerDiskImage maps guest OS to container disk image
func (m *Mapper) getContainerDiskImage(guestOS types.GuestOS) string {
	switch strings.ToLower(guestOS.Type) {
	case "ubuntu":
		return "quay.io/kubevirt/ubuntu-container-disk-demo:latest"
	case "centos":
		return "quay.io/kubevirt/centos-container-disk-demo:latest"
	case "fedora":
		return "quay.io/kubevirt/fedora-container-disk-demo:latest"
	case "cirros":
		return "quay.io/kubevirt/cirros-container-disk-demo:latest"
	default:
		return "quay.io/kubevirt/cirros-container-disk-demo:latest"
	}
}

// parseMemorySize converts memory size string to Kubernetes resource format
func (m *Mapper) parseMemorySize(sizeStr string) (string, error) {
	sizeStr = strings.TrimSpace(sizeStr)

	// First try to parse as a valid Kubernetes quantity
	if quantity, err := resource.ParseQuantity(sizeStr); err == nil {
		return quantity.String(), nil
	}

	// Handle common non-Kubernetes formats
	upperStr := strings.ToUpper(sizeStr)

	// Convert decimal GB to Gi
	if strings.HasSuffix(upperStr, "GB") {
		numStr := strings.TrimSuffix(upperStr, "GB")
		num, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return "", fmt.Errorf("invalid GB value: %s", numStr)
		}
		giValue := num * 1000 * 1000 * 1000 / (1024 * 1024 * 1024)
		return resource.NewQuantity(int64(giValue*1024*1024*1024), resource.BinarySI).String(), nil
	}

	// Convert decimal MB to Mi
	if strings.HasSuffix(upperStr, "MB") {
		numStr := strings.TrimSuffix(upperStr, "MB")
		num, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return "", fmt.Errorf("invalid MB value: %s", numStr)
		}
		miValue := num * 1000 * 1000 / (1024 * 1024)
		return resource.NewQuantity(int64(miValue*1024*1024), resource.BinarySI).String(), nil
	}

	// If just a number, assume Mi
	if num, err := strconv.ParseInt(sizeStr, 10, 64); err == nil {
		return resource.NewQuantity(num*1024*1024, resource.BinarySI).String(), nil
	}

	return "", fmt.Errorf("unable to parse memory size: %s", sizeStr)
}

// VirtualMachineToVMSpec converts a typed KubeVirt VirtualMachine back to DCM VMSpec format
func (m *Mapper) VirtualMachineToVMSpec(vm *kubevirtv1.VirtualMachine) (*types.VMSpec, error) {
	vmSpec := &types.VMSpec{}

	if vm.Spec.Template == nil {
		return vmSpec, nil
	}

	domain := vm.Spec.Template.Spec.Domain

	// Extract CPU information
	if cpuQty, ok := domain.Resources.Requests[k8sv1.ResourceCPU]; ok {
		if cpuCount, err := strconv.Atoi(cpuQty.String()); err == nil {
			vmSpec.Vcpu = types.Vcpu{Count: cpuCount}
		}
	}

	// Extract memory information
	if memQty, ok := domain.Resources.Requests[k8sv1.ResourceMemory]; ok {
		vmSpec.Memory = types.Memory{Size: memQty.String()}
	}

	// Extract guest OS from container disk image (best effort)
	guestOS := "cirros"
	if vols := vm.Spec.Template.Spec.Volumes; len(vols) > 0 {
		if cd := vols[0].ContainerDisk; cd != nil {
			guestOS = m.inferGuestOSFromImage(cd.Image)
		}
	}
	vmSpec.GuestOs = types.GuestOS{Type: guestOS}

	// Extract disk information
	var disks []types.Disk
	for _, d := range domain.Devices.Disks {
		disks = append(disks, types.Disk{Name: d.Name})
	}
	if len(disks) == 0 {
		disks = append(disks, types.Disk{Name: "boot"})
	}
	vmSpec.Storage = types.Storage{Disks: disks}

	return vmSpec, nil
}

// inferGuestOSFromImage tries to determine guest OS from container disk image
func (m *Mapper) inferGuestOSFromImage(image string) string {
	image = strings.ToLower(image)

	if strings.Contains(image, "ubuntu") {
		return "ubuntu"
	} else if strings.Contains(image, "centos") {
		return "centos"
	} else if strings.Contains(image, "fedora") {
		return "fedora"
	} else if strings.Contains(image, "cirros") {
		return "cirros"
	}

	return "cirros"
}
