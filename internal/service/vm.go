package service

import (
	"context"
	"fmt"
	"log"

	"github.com/dcm-project/kubevirt-service-provider/internal/api/server"
	"github.com/dcm-project/kubevirt-service-provider/internal/service/model"
	"github.com/google/uuid"
	"github.com/spf13/pflag"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"
)

type VMService struct {
	client kubecli.KubevirtClient
}

func NewVMService() *VMService {
	virtClient, err := kubecli.GetKubevirtClientFromClientConfig(
		kubecli.DefaultClientConfig(&pflag.FlagSet{}),
	)
	if err != nil {
		log.Fatalf("cannot obtain KubeVirt client: %v\n", err)
	}
	return &VMService{client: virtClient}
}

func (v *VMService) CreateVM(ctx context.Context, userRequest server.CreateVMJSONRequestBody) (model.DeclaredVM, error) {
	logger := zap.S().Named("vm_service:create_vm")

	metadata := *userRequest.Metadata
	appName, ok := metadata["application"]
	if !ok {
		logger.Warn("Application field not found in metadata")
	}
	id := uuid.New().String()

	logger.Info("Starting VM creation for: ", appName)

	namespace := "us-east-1"

	request := model.Request{
		OsImage:      v.getOSImage(userRequest.GuestOS.Type),
		Ram:          userRequest.Compute.Memory.SizeGB,
		Cpu:          userRequest.Compute.Vcpu.Count,
		Architecture: string(*userRequest.GuestOS.Architecture),
		RequestId:    id,
		VMName:       appName,
		HostName:     *userRequest.Initialization.Hostname,
	}

	logger.Info("Starting deployment for Virtual Machine")

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
					AccessCredentials: []kubevirtv1.AccessCredential{
						{
							SSHPublicKey: &kubevirtv1.SSHPublicKeyAccessCredential{
								Source: kubevirtv1.SSHPublicKeyAccessCredentialSource{
									Secret: &kubevirtv1.AccessCredentialSecretSource{
										SecretName: "myssh",
									},
								},
								PropagationMethod: kubevirtv1.SSHPublicKeyAccessCredentialPropagationMethod{
									NoCloud: &kubevirtv1.NoCloudSSHPublicKeyAccessCredentialPropagation{},
								},
							},
						},
					},
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

	// Create the VirtualMachine in the cluster
	_, err := v.client.VirtualMachine(namespace).Create(ctx, virtualMachine, metav1.CreateOptions{})
	if err != nil {
		return model.DeclaredVM{}, fmt.Errorf("failed to create VirtualMachine: %w", err)
	}

	logger.Info("Successfully created VM", request.RequestId)
	return model.DeclaredVM{ID: request.RequestId, RequestInfo: request}, nil

}

func (v *VMService) DeleteVMApplication(ctx context.Context, appID *string) (model.DeclaredVM, error) {
	logger := zap.S().Named("service-provider:delete_app")
	logger.Info("Deleting VM application", "ID ", appID)

	return model.DeclaredVM{}, nil
}

// generateCloudInitUserData generates cloud-init user data for the VM
func (v *VMService) generateCloudInitUserData(hostname string, vm *model.Request) string {
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
