package kubevirt

import (
	"context"
	"fmt"
	"strings"

	"github.com/dcm-project/kubevirt-service-provider/internal/service/mapper"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

// CreateVirtualMachineObject creates a KubeVirt VirtualMachine object from a request
// It configures CPU, memory, disks, networks, and optionally SSH access credentials
func (k *Client) CreateVirtualMachineObject(ctx context.Context, request mapper.Request, osImage string, cloudInitUserData string) (*kubevirtv1.VirtualMachine, error) {
	logger := zap.S().Named("kubevirt:create_vm_object")
	logger.Info("Creating VirtualMachine object", "vmName", request.VMName)

	memory := resource.MustParse(fmt.Sprintf("%dGi", request.Ram))
	virtualMachine := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-", request.VMName),
			Namespace:    request.Namespace,
			Name:         request.VMName,
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

		sshSecretName := fmt.Sprintf("%s-ssh-key", virtualMachine.Name)
		if err := k.EnsureSSHSecretAndAccessCredentials(ctx, virtualMachine, allKeys, sshSecretName); err != nil {
			return nil, fmt.Errorf("failed to configure SSH access: %w", err)
		}
	}

	logger.Info("Successfully created VirtualMachine object", "vmName", request.VMName)
	return virtualMachine, nil
}
