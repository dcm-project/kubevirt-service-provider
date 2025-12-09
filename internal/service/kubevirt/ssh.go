package kubevirt

import (
	"context"
	"fmt"

	"github.com/dcm-project/kubevirt-service-provider/internal/api/server"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

// EnsureSSHSecretAndAccessCredentials creates a Kubernetes Secret with SSH public keys
// and configures the VirtualMachine to use them for SSH access
func (k *Client) EnsureSSHSecretAndAccessCredentials(ctx context.Context, vm *kubevirtv1.VirtualMachine, sshPublicKey string, sshSecretName string) error {
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
func (k *Client) CreateSSHNodePortService(ctx context.Context, vm *kubevirtv1.VirtualMachine, requestID string) error {
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

// DeleteSSHNodePortService deletes the SSH NodePort service for a VM
func (k *Client) DeleteSSHNodePortService(ctx context.Context, namespace, serviceName string) error {
	logger := zap.S().Named("kubevirt:delete_ssh_service")
	logger.Infow("Deleting SSH NodePort service", "service", serviceName, "namespace", namespace)

	err := k.client.CoreV1().Services(namespace).Delete(ctx, serviceName, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		logger.Debugw("SSH NodePort service not found, skipping deletion", "service", serviceName)
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to delete SSH NodePort service: %w", err)
	}

	logger.Infow("Successfully deleted SSH NodePort service", "service", serviceName)
	return nil
}

// DeleteSSHSecret deletes the SSH secret for a VM
func (k *Client) DeleteSSHSecret(ctx context.Context, namespace, secretName string) error {
	logger := zap.S().Named("kubevirt:delete_ssh_secret")
	logger.Infow("Deleting SSH secret", "secret", secretName, "namespace", namespace)

	err := k.client.CoreV1().Secrets(namespace).Delete(ctx, secretName, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		logger.Debugw("SSH secret not found, skipping deletion", "secret", secretName)
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to delete SSH secret: %w", err)
	}

	logger.Infow("Successfully deleted SSH secret", "secret", secretName)
	return nil
}

// PopulateSSHConfiguration retrieves SSH configuration from the cluster and populates it on VMInstance
// It includes clusterSSH command and NodePort connection details
func (k *Client) PopulateSSHConfiguration(ctx context.Context, vmInstance *server.VMInstance, vm *kubevirtv1.VirtualMachine, requestID, vmIP, osImage string) {
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
