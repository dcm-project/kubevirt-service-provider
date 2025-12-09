package service

import (
	"context"
	"fmt"

	"github.com/dcm-project/kubevirt-service-provider/internal/api/server"
	"github.com/dcm-project/kubevirt-service-provider/internal/service/mapper"
	"github.com/dcm-project/kubevirt-service-provider/internal/store"
	"github.com/dcm-project/kubevirt-service-provider/internal/store/model"
	"github.com/google/uuid"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"kubevirt.io/client-go/kubecli"
)

// VMService handles VM lifecycle operations (Create, Get, Delete)
// It coordinates between the database store and KubeVirt cluster operations
type VMService struct {
	kubevirtClient kubecli.KubevirtClient
	kubevirt       *KubeVirtClient
	store          store.Store
}

// NewVMService creates a new VMService instance with KubeVirt client and store
// The KubeVirt client should be initialized at the server level and passed in via dependency injection
func NewVMService(kubevirtClient kubecli.KubevirtClient, store store.Store) *VMService {
	return &VMService{
		kubevirtClient: kubevirtClient,
		kubevirt:       NewKubeVirtClient(kubevirtClient),
		store:          store,
	}
}

// VM status constants
const (
	StatusCreated    = "CREATED"
	StatusInProgress = "IN_PROGRESS"
	StatusReady      = "READY"
)

// CreateVM creates a new virtual machine in the cluster and stores its metadata in the database
// It handles VM creation, SSH configuration, NodePort service creation, and database persistence
func (v *VMService) CreateVM(ctx context.Context, userRequest server.CreateVMJSONRequestBody) (server.VM, error) {
	logger := zap.S().Named("vm_service:create_vm")

	// Extract application name from metadata
	metadata := *userRequest.Metadata
	appName, ok := metadata["application"]
	if !ok {
		logger.Warn("Application field not found in metadata")
	}

	// Generate unique request ID
	id := uuid.New()
	logger.Info("Starting VM creation", "appName", appName, "requestID", id.String())

	// Validate namespace exists
	namespace := "us-east-1"
	namespaceExists, err := v.kubevirt.NamespaceExists(ctx, namespace)
	if err != nil || !namespaceExists {
		return server.VM{}, fmt.Errorf("namespace does not exist or cannot confirm it exists: %v", err)
	}

	// Build request from API payload
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

	// Generate cloud-init user data
	cloudInitUserData := v.generateCloudInitUserData(request.VMName, &request)

	// Create VirtualMachine object
	logger.Info("Creating VirtualMachine object")
	virtualMachine, err := v.kubevirt.CreateVirtualMachineObject(ctx, request, request.OsImage, cloudInitUserData)
	if err != nil {
		return server.VM{}, fmt.Errorf("cannot create virtual machine: %v", err)
	}

	// Create the VirtualMachine in the cluster
	logger.Info("Deploying VirtualMachine to cluster")
	createdVM, err := v.kubevirtClient.VirtualMachine(namespace).Create(ctx, virtualMachine, metav1.CreateOptions{})
	if err != nil {
		return server.VM{}, fmt.Errorf("failed to create VirtualMachine: %w", err)
	}

	// Create NodePort service for SSH if SSH keys are provided
	if len(request.SshKeys) > 0 {
		if err := v.kubevirt.CreateSSHNodePortService(ctx, createdVM, request.RequestId); err != nil {
			logger.Warnw("Failed to create SSH NodePort service", "error", err)
			// Don't fail VM creation if service creation fails
		}
	}

	// Save VM metadata to database
	dbApp, err := v.saveToStore(ctx, request)
	if err != nil {
		return server.VM{}, fmt.Errorf("failed to save VM in database: %w", err)
	}

	logger.Info("Successfully created VM", "requestID", request.RequestId)
	return server.VM{Id: &request.RequestId, Name: &request.VMName, Namespace: &request.Namespace, Status: &dbApp.Status}, nil
}

// GetVMFromCluster retrieves a VM from the cluster by request ID and returns detailed VMInstance information
// It queries both the database and cluster to provide comprehensive VM details including status, IP, and SSH configuration
func (v *VMService) GetVMFromCluster(ctx context.Context, requestID string) ([]server.VMInstance, error) {
	logger := zap.S().Named("vm_service:get_vm_from_cluster")
	logger.Infow("Getting VM from cluster", "requestID", requestID)

	// Parse the request ID to UUID
	vmID, err := uuid.Parse(requestID)
	if err != nil {
		return nil, fmt.Errorf("invalid request ID format: %w", err)
	}

	// Get the VM from database to get namespace and metadata
	dbApp, err := v.store.Application().Get(ctx, vmID)
	if err != nil {
		return nil, fmt.Errorf("failed to get VM from database: %w", err)
	}
	namespace := dbApp.Namespace

	// List all VMs in the namespace with matching app-id label
	vmList, err := v.kubevirtClient.VirtualMachine(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app-id=%s", requestID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list VMs in namespace %s: %w", namespace, err)
	}

	// If VM not found in cluster, return database record
	if len(vmList.Items) == 0 {
		logger.Warnw("No VM found in cluster with request ID", "requestID", requestID, "namespace", namespace)
		idUuidFmt, err := uuid.Parse(requestID)
		if err != nil {
			return nil, fmt.Errorf("invalid request ID format: %w", err)
		}
		return []server.VMInstance{
			{
				RequestId: &idUuidFmt,
				Name:      &dbApp.VMName,
				Namespace: &dbApp.Namespace,
				Status:    &dbApp.Status,
			},
		}, nil
	}

	// Get VirtualMachineInstance to get IP and status
	vmiList, err := v.kubevirtClient.VirtualMachineInstance(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app-id=%s", requestID),
	})

	// Convert cluster VM to VMInstance response
	vms := make([]server.VMInstance, 0, len(vmList.Items))
	for _, vm := range vmList.Items {
		vmInstance := server.VMInstance{
			RequestId: &vmID,
			Name:      &dbApp.VMName,
			Namespace: &namespace,
		}

		// Get VMI status and IP
		var vmiIP string
		var vmiStatus string
		if err == nil && len(vmiList.Items) > 0 {
			vmi := vmiList.Items[0]
			vmiStatus = string(vmi.Status.Phase)

			// Get IP from VMI interfaces
			if len(vmi.Status.Interfaces) > 0 {
				vmiIP = vmi.Status.Interfaces[0].IP
			}
		} else {
			// Fallback to database status if VMI not found
			vmiStatus = dbApp.Status
		}

		vmInstance.Status = &vmiStatus
		if vmiIP != "" {
			vmInstance.Ip = &vmiIP
		}

		// Populate SSH configuration from cluster
		v.kubevirt.PopulateSSHConfiguration(ctx, &vmInstance, &vm, requestID, vmiIP, dbApp.OsImage)

		vms = append(vms, vmInstance)
	}

	logger.Infow("Found VM(s) with details", "count", len(vms), "requestID", requestID)
	return vms, nil
}

// DeleteVMApplication deletes a VM application from both the cluster and database
func (v *VMService) DeleteVMApplication(ctx context.Context, appID *string) (mapper.DeclaredVM, error) {
	logger := zap.S().Named("vm_service:delete_app")
	logger.Info("Deleting VM application", "ID", appID)

	// TODO: Implement VM deletion from cluster
	// TODO: Implement database record deletion

	return mapper.DeclaredVM{}, nil
}

// ListVMsFromDatabase retrieves all VMs from the database
// Returns basic VM information without cluster details
func (v *VMService) ListVMsFromDatabase(ctx context.Context) ([]server.VMInstance, error) {
	logger := zap.S().Named("vm_service:list_vms_from_database")
	logger.Info("Listing VMs from database")

	apps, err := v.store.Application().List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list applications: %w", err)
	}

	var vms []server.VMInstance
	for _, app := range apps {
		idUuidFmt, err := uuid.Parse(app.ID.String())
		if err != nil {
			return nil, fmt.Errorf("failed to parse application ID %q: %w", app.ID.String(), err)
		}
		vms = append(vms, server.VMInstance{
			RequestId: &idUuidFmt,
			Name:      &app.VMName,
			Namespace: &app.Namespace,
			Status:    &app.Status,
		})
	}

	logger.Infow("Successfully retrieved VMs", "count", len(vms))
	return vms, nil
}

// saveToStore saves VM metadata to the database
func (v *VMService) saveToStore(ctx context.Context, request mapper.Request) (model.ProviderApplication, error) {
	logger := zap.S().Named("vm_service:save_to_store")
	logger.Info("Saving VM metadata to database")

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
		return model.ProviderApplication{}, fmt.Errorf("failed to create application: %w", err)
	}

	return dbApp, nil
}

// generateCloudInitUserData generates cloud-init user data for VM initialization
func (v *VMService) generateCloudInitUserData(hostname string, vm *mapper.Request) string {
	return fmt.Sprintf(`#cloud-config
user: %s
password: auto-generated-pass
chpasswd: { expire: False }
hostname: %s
`, vm.OsImage, hostname)
}

// getOSImage returns the container image for the specified OS type
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
