package kubevirt

import (
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

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

// VMSpecToVirtualMachine converts a DCM VMSpec to a KubeVirt VirtualMachine unstructured object
func (m *Mapper) VMSpecToVirtualMachine(vmSpec *types.VMSpec, vmID string) (*unstructured.Unstructured, error) {
	vm := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata": map[string]interface{}{
				"generateName": "dcm-",
				"namespace":    m.namespace,
				"labels": map[string]interface{}{
					constants.DCMLabelManagedBy:  constants.DCMManagedByValue,
					constants.DCMLabelInstanceID: vmID,
				},
			},
		},
	}

	// Build VirtualMachine spec
	spec := map[string]interface{}{
		"running": true,
		"template": map[string]interface{}{
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					constants.DCMLabelManagedBy:  constants.DCMManagedByValue,
					constants.DCMLabelInstanceID: vmID,
				},
			},
			"spec": map[string]interface{}{
				"domain":   m.buildDomainSpec(vmSpec),
				"networks": m.buildNetworks(),
				"volumes":  m.buildVolumes(vmSpec),
			},
		},
	}

	// Set the spec
	vm.Object["spec"] = spec

	return vm, nil
}

// buildDomainSpec creates the domain specification for the VM
func (m *Mapper) buildDomainSpec(vmSpec *types.VMSpec) map[string]interface{} {
	domain := map[string]interface{}{
		"devices": map[string]interface{}{
			"disks":      m.buildDisks(vmSpec),
			"interfaces": m.buildInterfaces(),
		},
		"resources": m.buildResources(vmSpec),
	}

	// Add machine type
	domain["machine"] = map[string]interface{}{
		"type": "q35",
	}

	return domain
}

// buildResources creates the resource specification
func (m *Mapper) buildResources(vmSpec *types.VMSpec) map[string]interface{} {
	resources := map[string]interface{}{
		"requests": map[string]interface{}{},
	}

	// Set CPU
	resources["requests"].(map[string]interface{})["cpu"] = fmt.Sprintf("%d", vmSpec.Vcpu.Count)

	// Set Memory
	if memorySize, err := m.parseMemorySize(vmSpec.Memory.Size); err == nil {
		resources["requests"].(map[string]interface{})["memory"] = memorySize
	}

	return resources
}

// buildDisks creates the disk specifications
func (m *Mapper) buildDisks(vmSpec *types.VMSpec) []interface{} {
	disks := []interface{}{}

	// Create disk entries for each storage disk
	for i, disk := range vmSpec.Storage.Disks {
		diskSpec := map[string]interface{}{
			"name": disk.Name,
			"disk": map[string]interface{}{
				"bus": "virtio",
			},
		}

		// Set as boot disk if this is the first disk or named "boot"
		if i == 0 || disk.Name == "boot" {
			diskSpec["bootOrder"] = float64(1)
		}

		disks = append(disks, diskSpec)
	}

	// If no disks defined, create a default boot disk
	if len(disks) == 0 {
		disks = append(disks, map[string]interface{}{
			"name": "boot",
			"disk": map[string]interface{}{
				"bus": "virtio",
			},
			"bootOrder": float64(1),
		})
	}

	return disks
}

// buildVolumes creates the volume specifications
func (m *Mapper) buildVolumes(vmSpec *types.VMSpec) []interface{} {
	volumes := []interface{}{}

	for i, disk := range vmSpec.Storage.Disks {
		// Create volume with container disk or empty disk
		volumeSpec := map[string]interface{}{
			"name": disk.Name,
		}

		// For boot disk, use container disk for OS images
		if i == 0 || disk.Name == "boot" {
			containerDiskImage := m.getContainerDiskImage(vmSpec.GuestOs)
			volumeSpec["containerDisk"] = map[string]interface{}{
				"image": containerDiskImage,
			}
		} else {
			// For data disks, create empty disk with default size
			parsedSize := "10Gi" // default size for now

			volumeSpec["emptyDisk"] = map[string]interface{}{
				"capacity": parsedSize,
			}
		}

		volumes = append(volumes, volumeSpec)
	}

	// If no volumes defined, create a default boot volume
	if len(volumes) == 0 {
		containerDiskImage := m.getContainerDiskImage(vmSpec.GuestOs)
		volumes = append(volumes, map[string]interface{}{
			"name": "boot",
			"containerDisk": map[string]interface{}{
				"image": containerDiskImage,
			},
		})
	}

	return volumes
}

// buildNetworks creates the network specifications. Must include a network
// named "default" (pod network) when using masquerade in domain.devices.interfaces.
func (m *Mapper) buildNetworks() []interface{} {
	return []interface{}{
		map[string]interface{}{
			"name": "default",
			"pod":  map[string]interface{}{},
		},
	}
}

// buildInterfaces creates the network interface specifications. Interface names
// must match network names; masquerade is only valid with the pod network.
func (m *Mapper) buildInterfaces() []interface{} {
	return []interface{}{
		map[string]interface{}{
			"name":       "default",
			"masquerade": map[string]interface{}{},
			"model":      "virtio",
		},
	}
}

// getContainerDiskImage maps guest OS to container disk image
func (m *Mapper) getContainerDiskImage(guestOS types.GuestOS) string {
	// Map common OS types to container disk images
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
		// Default to CirrOS for unknown OS types
		return "quay.io/kubevirt/cirros-container-disk-demo:latest"
	}
}

// parseMemorySize converts memory size string to Kubernetes resource format
func (m *Mapper) parseMemorySize(sizeStr string) (string, error) {
	// Trim whitespace but preserve original casing for valid Kubernetes units
	sizeStr = strings.TrimSpace(sizeStr)

	// First try to parse as a valid Kubernetes quantity
	if quantity, err := resource.ParseQuantity(sizeStr); err == nil {
		// Return canonical string representation
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
		// Convert GB to Gi: 1 GB = 1000^3 bytes, 1 Gi = 1024^3 bytes
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
		// Convert MB to Mi: 1 MB = 1000^2 bytes, 1 Mi = 1024^2 bytes
		miValue := num * 1000 * 1000 / (1024 * 1024)
		return resource.NewQuantity(int64(miValue*1024*1024), resource.BinarySI).String(), nil
	}

	// If just a number, assume Mi
	if num, err := strconv.ParseInt(sizeStr, 10, 64); err == nil {
		return resource.NewQuantity(num*1024*1024, resource.BinarySI).String(), nil
	}

	return "", fmt.Errorf("unable to parse memory size: %s", sizeStr)
}

// VirtualMachineToVMSpec converts a KubeVirt VirtualMachine back to DCM VMSpec format
func (m *Mapper) VirtualMachineToVMSpec(vm *unstructured.Unstructured) (*types.VMSpec, error) {
	// Build VMSpec from VirtualMachine data
	vmSpec := &types.VMSpec{}

	// Extract CPU information
	cpu, found, err := unstructured.NestedString(vm.Object, "spec", "template", "spec", "domain", "resources", "requests", "cpu")
	if err == nil && found {
		if cpuCount, parseErr := strconv.Atoi(cpu); parseErr == nil {
			vmSpec.Vcpu = types.Vcpu{
				Count: cpuCount,
			}
		}
	}

	// Extract memory information
	memory, found, err := unstructured.NestedString(vm.Object, "spec", "template", "spec", "domain", "resources", "requests", "memory")
	if err == nil && found {
		vmSpec.Memory = types.Memory{
			Size: memory,
		}
	}

	// Extract guest OS from container disk image (best effort)
	guestOS := "cirros" // default
	volumes, found, err := unstructured.NestedSlice(vm.Object, "spec", "template", "spec", "volumes")
	if err == nil && found && len(volumes) > 0 {
		if volume, ok := volumes[0].(map[string]interface{}); ok {
			if containerDisk, found := volume["containerDisk"]; found {
				if cd, ok := containerDisk.(map[string]interface{}); ok {
					if image, found := cd["image"]; found {
						if imageStr, ok := image.(string); ok {
							guestOS = m.inferGuestOSFromImage(imageStr)
						}
					}
				}
			}
		}
	}

	vmSpec.GuestOs = types.GuestOS{
		Type: guestOS,
	}

	// Extract disk information
	disks := []types.Disk{}
	diskSpecs, found, err := unstructured.NestedSlice(vm.Object, "spec", "template", "spec", "domain", "devices", "disks")
	if err == nil && found {
		for _, diskInterface := range diskSpecs {
			if disk, ok := diskInterface.(map[string]interface{}); ok {
				if name, found := disk["name"]; found {
					if nameStr, ok := name.(string); ok {
						disks = append(disks, types.Disk{
							Name: nameStr,
						})
					}
				}
			}
		}
	}

	// If no disks found, create default boot disk
	if len(disks) == 0 {
		disks = append(disks, types.Disk{
			Name: "boot",
		})
	}

	vmSpec.Storage = types.Storage{
		Disks: disks,
	}

	// Note: Metadata and ServiceType are typically set by the client/caller
	// For reverse mapping from KubeVirt, we focus on the VM-specific configuration

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

	return "cirros" // default fallback
}
