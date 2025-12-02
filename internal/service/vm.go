package service

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/dcm-project/kubevirt-service-provider/internal/api/server"
	"github.com/dcm-project/kubevirt-service-provider/internal/service/mapper"
	"github.com/dcm-project/kubevirt-service-provider/internal/store"
	"github.com/dcm-project/kubevirt-service-provider/internal/store/model"
	"github.com/google/uuid"
	"github.com/spf13/pflag"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"
)

type VMService struct {
	kubevirtClient kubecli.KubevirtClient
	store          store.Store
}

func NewVMService(store store.Store) *VMService {
	virtClient, err := kubecli.GetKubevirtClientFromClientConfig(
		kubecli.DefaultClientConfig(&pflag.FlagSet{}),
	)
	if err != nil {
		log.Fatalf("cannot obtain KubeVirt client: %v\n", err)
	}
	return &VMService{kubevirtClient: virtClient, store: store}
}

const (
	StatusCreated    = "CREATED"
	StatusInProgress = "IN_PROGRESS"
	StatusReady      = "READY"
)

func (v *VMService) CreateVM(ctx context.Context, userRequest server.CreateVMJSONRequestBody) (mapper.DeclaredVM, error) {
	logger := zap.S().Named("vm_service:create_vm")

	metadata := *userRequest.Metadata
	appName, ok := metadata["application"]
	if !ok {
		logger.Warn("Application field not found in metadata")
	}
	id := uuid.New()

	logger.Info("Starting VM creation for: ", appName)

	// check namespace exists
	namespace := "us-east-1"
	namespaceExists, err := v.NamespaceExists(ctx, namespace)
	if err != nil || !namespaceExists {
		return mapper.DeclaredVM{}, fmt.Errorf("namespace does not exist or cannot confirm it exists: %v", err)
	}

	request := mapper.Request{
		OsImage:      v.getOSImage(userRequest.GuestOS.Type),
		Ram:          userRequest.Compute.Memory.SizeGB,
		Cpu:          userRequest.Compute.Vcpu.Count,
		Architecture: string(*userRequest.GuestOS.Architecture),
		RequestId:    id.String(),
		VMName:       appName,
		HostName:     *userRequest.Initialization.Hostname,
		Namespace:    namespace,
	}
	if userRequest.Initialization.SshKeys != nil {
		request.SshKeys = *userRequest.Initialization.SshKeys
	}

	// Create VM object
	logger.Info("Creating virtual machine object")
	virtualMachine, err := v.createVMObject(ctx, request)
	if err != nil {
		return mapper.DeclaredVM{}, fmt.Errorf("cannot create virtual machine: %v\n", err)
	}

	logger.Info("Starting deployment for Virtual Machine")

	// Create the VirtualMachine in the cluster
	_, err = v.kubevirtClient.VirtualMachine(namespace).Create(ctx, virtualMachine, metav1.CreateOptions{})
	if err != nil {
		return mapper.DeclaredVM{}, fmt.Errorf("failed to create VirtualMachine: %w", err)
	}
	// save to database
	err = v.saveToStore(ctx, request)
	if err != nil {
		return mapper.DeclaredVM{}, fmt.Errorf("failed to save VM in database: %w", err)
	}

	logger.Info("Successfully created VM", request.RequestId)
	return mapper.DeclaredVM{ID: request.RequestId, RequestInfo: request}, nil
}

func (v *VMService) createVMObject(ctx context.Context, request mapper.Request) (*kubevirtv1.VirtualMachine, error) {
	logger := zap.S().Named("vm_service:create_vm_object")
	// Create the VirtualMachine object
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
									Image: request.OsImage,
								},
							},
						},
						{
							Name: "cloudinitdisk",
							VolumeSource: kubevirtv1.VolumeSource{
								CloudInitNoCloud: &kubevirtv1.CloudInitNoCloudSource{
									UserData: v.generateCloudInitUserData(request.VMName, &request),
								},
							},
						},
					},
				},
			},
		},
	}
	if request.SshKeys != nil {
		sshPublicKeys := request.SshKeys

		if len(sshPublicKeys) != 0 {
			// Normalize + filter empty strings
			var mergedKeys []string
			for _, k := range sshPublicKeys {
				k = strings.TrimSpace(k)
				if k != "" {
					mergedKeys = append(mergedKeys, k)
				}
			}
			allKeys := strings.Join(mergedKeys, "\n")

			sshSecretName := fmt.Sprintf("%s-ssh-key", virtualMachine.GenerateName)
			if err := ensureSSHSecretAndAccessCredentials(ctx, v.kubevirtClient, virtualMachine, allKeys, sshSecretName); err != nil {
				return nil, err
			}
		}
	}
	logger.Info("Successfully created virtual machine object", request.VMName)
	return virtualMachine, nil
}

func (v *VMService) saveToStore(ctx context.Context, request mapper.Request) error {
	logger := zap.S().Named("vm_service:save_to_store")
	logger.Info("Saving created VM to store")

	dbApp := model.ProviderApplication{
		ID:           uuid.MustParse(request.RequestId),
		OsImage:      request.OsImage,
		Ram:          request.Ram,
		Cpu:          request.Cpu,
		Namespace:    request.Namespace,
		VMName:       request.VMName,
		Architecture: request.Architecture,
		HostName:     request.HostName,
		Status:       StatusInProgress,
	}

	_, err := v.store.Application().Create(ctx, dbApp)
	if err != nil {
		return fmt.Errorf("failed to create application: %w", err)
	}
	return nil
}

func (v *VMService) DeleteVMApplication(ctx context.Context, appID *string) (mapper.DeclaredVM, error) {
	logger := zap.S().Named("service-provider:delete_app")
	logger.Info("Deleting VM application", "ID ", appID)

	return mapper.DeclaredVM{}, nil
}

func (v *VMService) ListVMsFromDatabase(ctx context.Context) ([]server.VM, error) {
	logger := zap.S().Named("vm_service:list_vms_from_database")
	logger.Info("Listing VMs from database")

	apps, err := v.store.Application().List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list applications: %w", err)
	}

	var vms []server.VM
	for _, app := range apps {
		idStr := app.ID.String()
		vms = append(vms, server.VM{
			Id:        &idStr,
			Name:      &app.VMName,
			Namespace: &app.Namespace,
		})
	}

	logger.Infow("Successfully retrieved VMs", "count", len(vms))
	return vms, nil
}

// GetVMFromCluster retrieves a VM from the cluster by request ID
func (v *VMService) GetVMFromCluster(ctx context.Context, requestID string) ([]server.VM, error) {
	logger := zap.S().Named("vm_service:get_vm_from_cluster")
	logger.Infow("Getting VM from cluster", "requestID", requestID)

	// Parse the request ID to UUID
	vmID, err := uuid.Parse(requestID)
	if err != nil {
		return nil, fmt.Errorf("invalid request ID format: %w", err)
	}

	// First, get the VM from database to get namespace
	dbApp, err := v.store.Application().Get(ctx, vmID)
	if err != nil {
		return nil, fmt.Errorf("failed to get VM from database: %w", err)
	}

	namespace := dbApp.Namespace

	// List all VMs in the namespace and find the one with matching app-id label
	vmList, err := v.kubevirtClient.VirtualMachine(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app-id=%s", requestID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list VMs in namespace %s: %w", namespace, err)
	}

	if len(vmList.Items) == 0 {

		logger.Warnw("No VM found in cluster with request ID", "requestID", requestID, "namespace", namespace)
		// Return the database record even if not found in cluster
		return []server.VM{
			{
				Id:        &requestID,
				Name:      &dbApp.VMName,
				Namespace: &dbApp.Namespace,
			},
		}, nil
	}

	// Convert cluster VM to API response
	vms := make([]server.VM, 0, len(vmList.Items))
	for _, vm := range vmList.Items {
		vmName := vm.Name
		vms = append(vms, server.VM{
			Id:        &requestID,
			Name:      &vmName,
			Namespace: &namespace,
		})
	}

	logger.Infof("Found %d VM(s) in cluster with request ID %s", len(vms), requestID)
	return vms, nil
}

// generateCloudInitUserData generates cloud-init user data for the VM
func (v *VMService) generateCloudInitUserData(hostname string, vm *mapper.Request) string {
	return fmt.Sprintf(`#cloud-config
user: %s
password: auto-generated-pass
chpasswd: { expire: False }
hostname: %s
`, vm.OsImage, hostname)
}

// getOSImage returns the container image for the specified OS
func (v *VMService) getOSImage(os string) string {
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

// ensureSSHSecretAndAccessCredentials optionally creates a Secret with the SSH public key
// and adds an AccessCredential to the VirtualMachine spec.
func ensureSSHSecretAndAccessCredentials(ctx context.Context, kubeClient kubecli.KubevirtClient, vm *kubevirtv1.VirtualMachine, sshPublicKey string, sshSecretName string) error {
	logger := zap.S().Named("service-provider:ensure_ssh_secret")
	logger.Info("Creating SSH secret...")
	// If no SSH key is provided, skip everything.
	if sshPublicKey == "" {
		return nil
	}

	ns := vm.Namespace
	if ns == "" {
		return fmt.Errorf("virtualMachine namespace must be set")
	}

	// SSHSecretDataKey is the key under which we store the public key in the Secret data.
	var SSHSecretDataKey = fmt.Sprintf("%s-ssh-pub", vm.GenerateName)

	// Ensure the Secret exists.
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

	_, err := kubeClient.CoreV1().Secrets(ns).Create(ctx, secret, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		logger.Info("SSH secret already exists")
	}
	if err != nil {
		return fmt.Errorf("failed to create SSH secret %q: %w", sshSecretName, err)
	}

	// Attach AccessCredentials to VM.
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

	return nil
}

// NamespaceExists returns nil if the namespace exists.
// It returns an error if it does not exist or cannot be checked.
func (v *VMService) NamespaceExists(ctx context.Context, namespace string) (bool, error) {
	if namespace == "" {
		return false, fmt.Errorf("namespace name cannot be empty")
	}

	_, err := v.kubevirtClient.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, fmt.Errorf("namespace %q does not exist", namespace)
		}
		return false, fmt.Errorf("failed to check namespace %q: %w", namespace, err)
	}
	return true, nil
}
