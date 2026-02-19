package kubevirt

import (
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	catalogv1alpha1 "github.com/dcm-project/catalog-manager/api/v1alpha1/servicetypes/vm"
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
func (m *Mapper) VMSpecToVirtualMachine(vmSpec *catalogv1alpha1.VMSpec, vmName string, vmID string) (*unstructured.Unstructured, error) {
	vm := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata": map[string]interface{}{
				"name":      vmName,
				"namespace": m.namespace,
				"labels": map[string]interface{}{
					"dcm.project/managed-by":      "dcm",
					"dcm.project/dcm-instance-id": vmID,
				},
			},
		},
	}

	// Build VirtualMachine spec
	spec := map[string]interface{}{
		"running": true,
		"template": map[string]interface{}{
			"metadata": map[string]interface{}{},
			"spec": map[string]interface{}{
				"domain":   m.buildDomainSpec(vmSpec),
				"networks": m.buildNetworks(),
				"volumes":  m.buildVolumes(vmSpec),
			},
		},
	}

	// Add terminationGracePeriodSeconds if needed
	spec["template"].(map[string]interface{})["spec"].(map[string]interface{})["terminationGracePeriodSeconds"] = int64(180)

	// Set the spec
	vm.Object["spec"] = spec

	return vm, nil
}

// buildDomainSpec creates the domain specification for the VM
func (m *Mapper) buildDomainSpec(vmSpec *catalogv1alpha1.VMSpec) map[string]interface{} {
	domain := map[string]interface{}{
		"devices": map[string]interface{}{
			"disks":      m.buildDisks(vmSpec),
			"interfaces": m.buildInterfaces(),
		},
		"resources": m.buildResources(vmSpec),
	}

	// Add machine type if needed
	domain["machine"] = map[string]interface{}{
		"type": "q35",
	}

	return domain
}

// buildResources creates the resource specification
func (m *Mapper) buildResources(vmSpec *catalogv1alpha1.VMSpec) map[string]interface{} {
	resources := map[string]interface{}{
		"requests": map[string]interface{}{},
	}

	// Set CPU
	resources["requests"].(map[string]interface{})["cpu"] = fmt.Sprintf("%d", vmSpec.Vcpu.Count)

	// Set Memory
	memorySize, err := m.parseMemorySize(vmSpec.Memory.Size)
	if err == nil {
		resources["requests"].(map[string]interface{})["memory"] = memorySize
	} else {
		// Fallback to default
		resources["requests"].(map[string]interface{})["memory"] = "2Gi"
	}

	return resources
}

// buildDisks creates the disk specifications
func (m *Mapper) buildDisks(vmSpec *catalogv1alpha1.VMSpec) []interface{} {
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
			diskSpec["bootOrder"] = 1
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
			"bootOrder": 1,
		})
	}

	return disks
}

// buildVolumes creates the volume specifications
func (m *Mapper) buildVolumes(vmSpec *catalogv1alpha1.VMSpec) []interface{} {
	volumes := []interface{}{}

	for i, disk := range vmSpec.Storage.Disks {
		// Create volume with container disk or empty disk
		volumeSpec := map[string]interface{}{
			"name": disk.Name,
		}

		// For boot disk, use container disk for OS images
		if i == 0 || disk.Name == "boot" {
			containerDiskImage := m.getContainerDiskImage(vmSpec.GuestOS)
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
		containerDiskImage := m.getContainerDiskImage(vmSpec.GuestOS)
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
func (m *Mapper) getContainerDiskImage(guestOS catalogv1alpha1.GuestOS) string {
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
	// Handle various memory size formats (e.g., "2GB", "2Gi", "2048Mi")
	sizeStr = strings.TrimSpace(strings.ToUpper(sizeStr))

	// If it's already in Kubernetes format, return as-is
	if strings.HasSuffix(sizeStr, "I") {
		return sizeStr, nil
	}

	// Convert common formats
	if strings.HasSuffix(sizeStr, "GB") {
		// Convert GB to Gi (approximately)
		numStr := strings.TrimSuffix(sizeStr, "GB")
		num, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return "", err
		}
		// 1 GB ≈ 0.93 Gi, so we'll round up slightly
		giBytes := int64(num * 1024 * 1024 * 1024)
		return fmt.Sprintf("%d", giBytes), nil
	}

	if strings.HasSuffix(sizeStr, "MB") {
		numStr := strings.TrimSuffix(sizeStr, "MB")
		num, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return "", err
		}
		miBytes := int64(num)
		return fmt.Sprintf("%dMi", miBytes), nil
	}

	// If just a number, assume MB
	if num, err := strconv.ParseInt(sizeStr, 10, 64); err == nil {
		return fmt.Sprintf("%dMi", num), nil
	}

	// Try to parse as Kubernetes quantity
	_, err := resource.ParseQuantity(sizeStr)
	if err != nil {
		return "", fmt.Errorf("unable to parse memory size: %s", sizeStr)
	}

	return sizeStr, nil
}

// parseStorageSize converts storage size string to Kubernetes resource format
func (m *Mapper) parseStorageSize(sizeStr string) (string, error) {
	// Similar to parseMemorySize but for storage
	return m.parseMemorySize(sizeStr)
}

// VirtualMachineToVMSpec converts a KubeVirt VirtualMachine back to DCM VMSpec format
func (m *Mapper) VirtualMachineToVMSpec(vm *unstructured.Unstructured) (*catalogv1alpha1.VMSpec, error) {
	// Build VMSpec from VirtualMachine data
	vmSpec := &catalogv1alpha1.VMSpec{}

	// Extract CPU information
	cpu, found, err := unstructured.NestedString(vm.Object, "spec", "template", "spec", "domain", "resources", "requests", "cpu")
	if err == nil && found {
		if cpuCount, parseErr := strconv.Atoi(cpu); parseErr == nil {
			vmSpec.Vcpu = catalogv1alpha1.Vcpu{
				Count: cpuCount,
			}
		}
	}

	// If CPU not found, use default
	if vmSpec.Vcpu.Count == 0 {
		vmSpec.Vcpu = catalogv1alpha1.Vcpu{Count: 1}
	}

	// Extract memory information
	memory, found, err := unstructured.NestedString(vm.Object, "spec", "template", "spec", "domain", "resources", "requests", "memory")
	if err == nil && found {
		vmSpec.Memory = catalogv1alpha1.Memory{
			Size: memory,
		}
	} else {
		// Default memory
		vmSpec.Memory = catalogv1alpha1.Memory{Size: "1Gi"}
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

	vmSpec.GuestOS = catalogv1alpha1.GuestOS{
		Type: guestOS,
	}

	// Extract disk information
	disks := []catalogv1alpha1.Disk{}
	diskSpecs, found, err := unstructured.NestedSlice(vm.Object, "spec", "template", "spec", "domain", "devices", "disks")
	if err == nil && found {
		for _, diskInterface := range diskSpecs {
			if disk, ok := diskInterface.(map[string]interface{}); ok {
				if name, found := disk["name"]; found {
					if nameStr, ok := name.(string); ok {
						disks = append(disks, catalogv1alpha1.Disk{
							Name: nameStr,
						})
					}
				}
			}
		}
	}

	// If no disks found, create default boot disk
	if len(disks) == 0 {
		disks = append(disks, catalogv1alpha1.Disk{
			Name: "boot",
		})
	}

	vmSpec.Storage = catalogv1alpha1.Storage{
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
