package service

import (
	"context"
	"fmt"
	"log"

	"github.com/dcm-project/kubevirt-service-provider/internal/api/server"
	"github.com/dcm-project/kubevirt-service-provider/internal/service/model"
	"github.com/spf13/pflag"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
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
	logger.Info("Starting VM creation for: ", *userRequest.Name)

	request := model.Request{
		OsImage:   *userRequest.OsImage,
		Ram:       *userRequest.Ram,
		Cpu:       *userRequest.Cpu,
		RequestId: *userRequest.Id,
		Namespace: *userRequest.Namespace,
		VMName:    *userRequest.Name,
	}

	logger.Info("Starting deployment for Virtual Machine")

	// Create Namespace for the Virtual Machine
	namespace := request.Namespace
	logger.Info("Creating namespace ", namespace)
	// Check Namespace exists
	_, err := v.client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		// Create Namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		_, err = v.client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
		if err != nil {
			logger.Error("Error occurred when creating namespace", err)
			return model.DeclaredVM{}, fmt.Errorf("failed to create namespace %s: %w", namespace, err)
		}
	}
	logger.Info("Successfully created namespace ", "Namespace ", namespace)

	// Create the VirtualMachine object
	memory := resource.MustParse(fmt.Sprintf("%dGi", request.Ram))
	virtualMachine := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-", request.VMName),
			Namespace:    namespace,
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
					Architecture: "amd64",
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
	_, err = v.client.VirtualMachine(namespace).Create(ctx, virtualMachine, metav1.CreateOptions{})
	if err != nil {
		return model.DeclaredVM{}, fmt.Errorf("failed to create VirtualMachine: %w", err)
	}

	logger.Info("Successfully created VM", userRequest.Id)
	return model.DeclaredVM{ID: request.RequestId, RequestInfo: request}, nil

}

func (v *VMService) DeleteVMApplication(ctx context.Context, appID *string) (model.DeclaredVM, error) {
	logger := zap.S().Named("service-provider:delete_app")
	logger.Info("Deleting VM application", "ID ", appID)

	return model.DeclaredVM{}, nil
}

// generateCloudInitUserData generates cloud-init user data for the VM
func (v *VMService) generateCloudInitUserData(appName string, vm *model.Request) string {
	return fmt.Sprintf(`#cloud-config
user: %s
password: auto-generated-pass
chpasswd: { expire: False }
hostname: %s
`, vm.OsImage, appName)
}
