package kubevirt

import (
	"fmt"
	"strings"

	"github.com/dcm-project/kubevirt-service-provider/internal/service/mapper"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

// GenerateCloudInitUserData generates cloud-init user data for VM initialization
func GenerateCloudInitUserData(hostname string, vm *mapper.Request) string {
	return fmt.Sprintf(`#cloud-config
user: %s
password: auto-generated-pass
chpasswd: { expire: False }
hostname: %s
`, vm.OsImage, hostname)
}

// GetOSImage returns the container image for the specified OS type
func GetOSImage(os string) string {
	images := map[string]string{
		"fedora": "quay.io/containerdisks/fedora:latest",
		"ubuntu": "quay.io/containerdisks/ubuntu:latest",
		"centos": "quay.io/containerdisks/centos:latest",
		"rhel":   "quay.io/containerdisks/rhel:latest",
	}

	if image, exists := images[os]; exists {
		return image
	}
	// Default to fedora if OS not found
	return "quay.io/containerdisks/fedora:latest"
}

// getSSHUsernameFromVM extracts the SSH username from the VM's cloud-init NoCloud user data
// It extracts the hostname field from cloud-init and uses it as the SSH username
// Falls back to default username based on OS image if cloud-init data is not available or doesn't contain hostname
func (k *Client) getSSHUsernameFromVM(vm *kubevirtv1.VirtualMachine, osImage string) string {
	logger := zap.S().Named("kubevirt:get_ssh_username")

	// Extract cloud-init user data from VM spec
	if vm.Spec.Template == nil || vm.Spec.Template.Spec.Volumes == nil {
		logger.Debug("VM spec template or volumes not found, using default username")
		return getDefaultSSHUsername(osImage)
	}

	// Find the cloudinitdisk volume
	for _, volume := range vm.Spec.Template.Spec.Volumes {
		if volume.Name == "cloudinitdisk" && volume.CloudInitNoCloud != nil {
			userData := volume.CloudInitNoCloud.UserData
			if userData == "" {
				logger.Debug("Cloud-init user data is empty, using default username")
				return getDefaultSSHUsername(osImage)
			}

			// Parse YAML to extract hostname
			hostname, err := extractHostnameFromCloudInit(userData)
			if err != nil {
				logger.Debugw("Failed to parse cloud-init user data, using default username", "error", err)
				return getDefaultSSHUsername(osImage)
			}

			if hostname != "" {
				logger.Debugw("Extracted hostname from cloud-init", "hostname", hostname)
				return hostname
			}
		}
	}

	logger.Debug("Cloud-init volume not found, using default username")
	return getDefaultSSHUsername(osImage)
}

// extractHostnameFromCloudInit parses cloud-init YAML and extracts the hostname field
func extractHostnameFromCloudInit(userData string) (string, error) {
	// Remove the #cloud-config header if present
	userData = strings.TrimSpace(userData)
	if strings.HasPrefix(userData, "#cloud-config") {
		lines := strings.SplitN(userData, "\n", 2)
		if len(lines) > 1 {
			userData = strings.TrimSpace(lines[1])
		}
	}

	// Parse YAML
	var cloudInit struct {
		Hostname string `yaml:"hostname"`
	}

	if err := yaml.Unmarshal([]byte(userData), &cloudInit); err != nil {
		return "", fmt.Errorf("failed to unmarshal cloud-init YAML: %w", err)
	}

	if cloudInit.Hostname == "" {
		return "", fmt.Errorf("no hostname found in cloud-init user data")
	}

	return cloudInit.Hostname, nil
}

// getDefaultSSHUsername returns the default SSH username based on OS image
// This is used as a fallback when cloud-init data is not available
func getDefaultSSHUsername(osImage string) string {
	if strings.Contains(osImage, "fedora") {
		return "fedora"
	}
	if strings.Contains(osImage, "ubuntu") {
		return "ubuntu"
	}
	if strings.Contains(osImage, "centos") || strings.Contains(osImage, "rhel") {
		return "cloud-user"
	}
	return "fedora"
}
