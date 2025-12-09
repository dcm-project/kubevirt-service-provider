package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/dcm-project/kubevirt-service-provider/internal/api/server"
	"github.com/dcm-project/kubevirt-service-provider/internal/service/mapper"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"
)

// KubeVirtClient wraps KubeVirt client operations
type KubeVirtClient struct {
	client kubecli.KubevirtClient
}

// NewKubeVirtClient creates a new KubeVirt client wrapper
func NewKubeVirtClient(client kubecli.KubevirtClient) *KubeVirtClient {
	return &KubeVirtClient{client: client}
}

// CreateVirtualMachineObject creates a KubeVirt VirtualMachine object from a request
// It configures CPU, memory, disks, networks, and optionally SSH access credentials
func (k *KubeVirtClient) CreateVirtualMachineObject(ctx context.Context, request mapper.Request, osImage string, cloudInitUserData string) (*kubevirtv1.VirtualMachine, error) {
	logger := zap.S().Named("kubevirt:create_vm_object")
	logger.Info("Creating VirtualMachine object", "vmName", request.VMName)

	memory := resource.MustParse(fmt.Sprintf("%dGi", request.Ram))
	virtualMachine := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-", request.VMName),
			Namespace:    request.Namespace,
			Labels: map[string]string{
				"app-id": request.RequestId,
			},
		},
		Spec: kubevirtv1.VirtualMachineSpec{
			RunStrategy: &[]kubevirtv1.VirtualMachineRunStrategy{kubevirtv1.RunStrategyRerunOnFailure}[0],
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app-id": request.RequestId,
					},
				},
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Architecture: request.Architecture,
					Domain: kubevirtv1.DomainSpec{
						CPU: &kubevirtv1.CPU{
							Cores: uint32(request.Cpu),
						},
						Memory: &kubevirtv1.Memory{
							Guest: &memory,
						},
						Devices: kubevirtv1.Devices{
							Disks: []kubevirtv1.Disk{
								{
									Name:      fmt.Sprintf("%s-disk", request.VMName),
									BootOrder: &[]uint{1}[0],
									DiskDevice: kubevirtv1.DiskDevice{
										Disk: &kubevirtv1.DiskTarget{
											Bus: kubevirtv1.DiskBusVirtio,
										},
									},
								},
								{
									Name:      "cloudinitdisk",
									BootOrder: &[]uint{2}[0],
									DiskDevice: kubevirtv1.DiskDevice{
										Disk: &kubevirtv1.DiskTarget{
											Bus: kubevirtv1.DiskBusVirtio,
										},
									},
								},
							},
							Interfaces: []kubevirtv1.Interface{
								{
									Name: "myvmnic",
									InterfaceBindingMethod: kubevirtv1.InterfaceBindingMethod{
										Bridge: &kubevirtv1.InterfaceBridge{},
									},
								},
							},
							Rng: &kubevirtv1.Rng{},
						},
						Features: &kubevirtv1.Features{
							ACPI: kubevirtv1.FeatureState{},
							SMM: &kubevirtv1.FeatureState{
								Enabled: &[]bool{true}[0],
							},
						},
						Machine: &kubevirtv1.Machine{
							Type: "pc-q35-rhel9.6.0",
						},
					},
					Networks: []kubevirtv1.Network{
						{
							Name: "myvmnic",
							NetworkSource: kubevirtv1.NetworkSource{
								Pod: &kubevirtv1.PodNetwork{},
							},
						},
					},
					TerminationGracePeriodSeconds: &[]int64{180}[0],
					Volumes: []kubevirtv1.Volume{
						{
							Name: fmt.Sprintf("%s-disk", request.VMName),
							VolumeSource: kubevirtv1.VolumeSource{
								ContainerDisk: &kubevirtv1.ContainerDiskSource{
									Image: osImage,
								},
							},
						},
						{
							Name: "cloudinitdisk",
							VolumeSource: kubevirtv1.VolumeSource{
								CloudInitNoCloud: &kubevirtv1.CloudInitNoCloudSource{
									UserData: cloudInitUserData,
								},
							},
						},
					},
				},
			},
		},
	}

	// Configure SSH access if SSH keys are provided
	if len(request.SshKeys) > 0 {
		// Normalize and filter empty strings
		var mergedKeys []string
		for _, k := range request.SshKeys {
			k = strings.TrimSpace(k)
			if k != "" {
				mergedKeys = append(mergedKeys, k)
			}
		}
		allKeys := strings.Join(mergedKeys, "\n")

		sshSecretName := fmt.Sprintf("%s-ssh-key", &virtualMachine.Name)
		if err := k.EnsureSSHSecretAndAccessCredentials(ctx, virtualMachine, allKeys, sshSecretName); err != nil {
			return nil, fmt.Errorf("failed to configure SSH access: %w", err)
		}
	}

	logger.Info("Successfully created VirtualMachine object", "vmName", request.VMName)
	return virtualMachine, nil
}

// EnsureSSHSecretAndAccessCredentials creates a Kubernetes Secret with SSH public keys
// and configures the VirtualMachine to use them for SSH access
func (k *KubeVirtClient) EnsureSSHSecretAndAccessCredentials(ctx context.Context, vm *kubevirtv1.VirtualMachine, sshPublicKey string, sshSecretName string) error {
	logger := zap.S().Named("kubevirt:ensure_ssh_secret")
	logger.Info("Configuring SSH access credentials", "secretName", sshSecretName)

	// If no SSH key is provided, skip everything
	if sshPublicKey == "" {
		return nil
	}

	ns := vm.Namespace
	if ns == "" {
		return fmt.Errorf("virtualMachine namespace must be set")
	}

	// SSHSecretDataKey is the key under which we store the public key in the Secret data
	SSHSecretDataKey := fmt.Sprintf("%s-ssh-pub", vm.GenerateName)

	// Create the Secret with SSH public key
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sshSecretName,
			Namespace: ns,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			SSHSecretDataKey: []byte(sshPublicKey),
		},
	}

	_, err := k.client.CoreV1().Secrets(ns).Create(ctx, secret, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		logger.Info("SSH secret already exists, skipping creation", "secretName", sshSecretName)
	} else if err != nil {
		return fmt.Errorf("failed to create SSH secret %q: %w", sshSecretName, err)
	}

	// Attach AccessCredentials to VM spec
	vm.Spec.Template.Spec.AccessCredentials = []kubevirtv1.AccessCredential{
		{
			SSHPublicKey: &kubevirtv1.SSHPublicKeyAccessCredential{
				Source: kubevirtv1.SSHPublicKeyAccessCredentialSource{
					Secret: &kubevirtv1.AccessCredentialSecretSource{
						SecretName: sshSecretName,
					},
				},
				PropagationMethod: kubevirtv1.SSHPublicKeyAccessCredentialPropagationMethod{
					NoCloud: &kubevirtv1.NoCloudSSHPublicKeyAccessCredentialPropagation{},
				},
			},
		},
	}

	logger.Info("Successfully configured SSH access credentials")
	return nil
}

// CreateSSHNodePortService creates a Kubernetes NodePort Service to expose SSH access to the VM
// This allows external access to the VM via SSH through a NodePort
func (k *KubeVirtClient) CreateSSHNodePortService(ctx context.Context, vm *kubevirtv1.VirtualMachine, requestID string) error {
	logger := zap.S().Named("kubevirt:create_ssh_nodeport")
	logger.Info("Creating NodePort service for SSH access", "vmName", vm.Name)

	// Service name based on VM name
	serviceName := fmt.Sprintf("%s-ssh", vm.Name)
	if serviceName == "-ssh" {
		// If VM name is empty (using GenerateName), use request ID
		serviceName = fmt.Sprintf("%s-ssh", requestID)
	}

	// Create NodePort service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: vm.Namespace,
			Labels: map[string]string{
				"app-id":       requestID,
				"service-type": "ssh",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Selector: map[string]string{
				"app-id": requestID,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "ssh",
					Protocol:   corev1.ProtocolTCP,
					Port:       22,
					TargetPort: intstr.FromInt32(22),
				},
			},
		},
	}

	_, err := k.client.CoreV1().Services(vm.Namespace).Create(ctx, service, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		logger.Infow("SSH NodePort service already exists", "service", serviceName)
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to create SSH NodePort service: %w", err)
	}

	logger.Infow("Successfully created SSH NodePort service", "service", serviceName)
	return nil
}

// PopulateSSHConfiguration retrieves SSH configuration from the cluster and populates it on VMInstance
// It includes clusterSSH command and NodePort connection details
func (k *KubeVirtClient) PopulateSSHConfiguration(ctx context.Context, vmInstance *server.VMInstance, vm *kubevirtv1.VirtualMachine, requestID, vmIP, osImage string) {
	logger := zap.S().Named("kubevirt:populate_ssh_config")

	// Check if VM has SSH access credentials configured
	var sshSecretName string
	sshEnabled := false

	if vm.Spec.Template != nil && len(vm.Spec.Template.Spec.AccessCredentials) > 0 {
		for _, cred := range vm.Spec.Template.Spec.AccessCredentials {
			if cred.SSHPublicKey != nil && cred.SSHPublicKey.Source.Secret != nil {
				sshEnabled = true
				sshSecretName = cred.SSHPublicKey.Source.Secret.SecretName
				break
			}
		}
	}

	if !sshEnabled {
		return
	}

	// Get SSH username from cloud-init user data in VM spec
	sshUsername := k.getSSHUsernameFromVM(vm, osImage)

	// Build clusterSSH command if IP is available
	var clusterSSH *string
	if vmIP != "" {
		clusterSSHCmd := fmt.Sprintf("ssh %s@%s", sshUsername, vmIP)
		clusterSSH = &clusterSSHCmd
	}

	// Get NodePort service details
	var nodePortConfig *struct {
		Node *string `json:"node,omitempty"`
		Port *int    `json:"port,omitempty"`
	}

	serviceName := fmt.Sprintf("%s-ssh", vm.Name)
	if serviceName == "-ssh" {
		serviceName = fmt.Sprintf("%s-ssh", requestID)
	}

	service, err := k.client.CoreV1().Services(vm.Namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err == nil && service.Spec.Type == corev1.ServiceTypeNodePort && len(service.Spec.Ports) > 0 {
		nodePort := int(service.Spec.Ports[0].NodePort)
		nodePortConfig = &struct {
			Node *string `json:"node,omitempty"`
			Port *int    `json:"port,omitempty"`
		}{
			Port: &nodePort,
		}

		// Get node IP (get the first node's internal IP)
		nodes, err := k.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err == nil && len(nodes.Items) > 0 {
			for _, addr := range nodes.Items[0].Status.Addresses {
				if addr.Type == corev1.NodeInternalIP {
					nodeIP := addr.Address
					nodePortConfig.Node = &nodeIP
					break
				}
			}
		}
	} else if err != nil {
		logger.Debugw("Could not get NodePort service", "service", serviceName, "error", err)
	}

	// Build ConnectMethods
	var connectMethods *struct {
		ClusterSSH *string `json:"clusterSSH,omitempty"`
		NodePort   *struct {
			Node *string `json:"node,omitempty"`
			Port *int    `json:"port,omitempty"`
		} `json:"nodePort,omitempty"`
	}

	if clusterSSH != nil || nodePortConfig != nil {
		connectMethods = &struct {
			ClusterSSH *string `json:"clusterSSH,omitempty"`
			NodePort   *struct {
				Node *string `json:"node,omitempty"`
				Port *int    `json:"port,omitempty"`
			} `json:"nodePort,omitempty"`
		}{
			ClusterSSH: clusterSSH,
			NodePort:   nodePortConfig,
		}
	}

	// Initialize SSH struct on VMInstance
	vmInstance.Ssh = &struct {
		ConnectMethods *struct {
			ClusterSSH *string `json:"clusterSSH,omitempty"`
			NodePort   *struct {
				Node *string `json:"node,omitempty"`
				Port *int    `json:"port,omitempty"`
			} `json:"nodePort,omitempty"`
		} `json:"connectMethods,omitempty"`
		Enabled    *bool   `json:"enabled,omitempty"`
		SecretName *string `json:"secretName,omitempty"`
		Username   *string `json:"username,omitempty"`
	}{
		ConnectMethods: connectMethods,
		Enabled:        &sshEnabled,
		SecretName:     &sshSecretName,
		Username:       &sshUsername,
	}
	logger.Info("Successfully populated SSH configuration on VMInstance")
}

// NamespaceExists checks if a Kubernetes namespace exists
func (k *KubeVirtClient) NamespaceExists(ctx context.Context, namespace string) (bool, error) {
	if namespace == "" {
		return false, fmt.Errorf("namespace name cannot be empty")
	}

	_, err := k.client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, fmt.Errorf("namespace %q does not exist", namespace)
		}
		return false, fmt.Errorf("failed to check namespace %q: %w", namespace, err)
	}
	return true, nil
}

// getSSHUsernameFromVM extracts the SSH username from the VM's cloud-init NoCloud user data
// It extracts the hostname field from cloud-init and uses it as the SSH username
// Falls back to default username based on OS image if cloud-init data is not available or doesn't contain hostname
func (k *KubeVirtClient) getSSHUsernameFromVM(vm *kubevirtv1.VirtualMachine, osImage string) string {
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
