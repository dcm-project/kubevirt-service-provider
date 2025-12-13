package service

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/dcm-project/kubevirt-service-provider/internal/api/server"
	"github.com/dcm-project/kubevirt-service-provider/internal/store"
	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"
)

type VMStatusSyncService struct {
	client      kubecli.KubevirtClient
	store       store.Store
	logger      *zap.SugaredLogger
	watchers    map[string]watch.Interface // key: vmID
	mu          sync.RWMutex
	dcmUrl      string
	restyClient *resty.Client
}

func NewVMStatusSyncService(client kubecli.KubevirtClient, store store.Store, dcmUrl string) *VMStatusSyncService {
	return &VMStatusSyncService{
		client:   client,
		store:    store,
		logger:   zap.S().Named("vm_status_sync"),
		watchers: make(map[string]watch.Interface),
		dcmUrl:   dcmUrl,
		restyClient: resty.New().
			SetTimeout(5*time.Second).
			SetHeader("Content-Type", "application/json"),
	}
}

// mapVMStatusToAppStatus maps KubeVirt VM status to application status
func (s *VMStatusSyncService) mapVMStatusToAppStatus(vm *kubevirtv1.VirtualMachine) string {
	// Check if VM is ready (VMI is running)
	if vm.Status.Ready {
		return StatusReady
	}

	// Check if VM is created
	if vm.Status.Created {
		return StatusCreated
	}

	// Check if there's a VMI running
	if vm.Status.PrintableStatus == kubevirtv1.VirtualMachineStatusRunning {
		return StatusReady
	}

	// Check if VM is starting
	if vm.Status.PrintableStatus == kubevirtv1.VirtualMachineStatusStarting {
		return StatusInProgress
	}

	// Default to in progress
	return StatusInProgress
}

// StartWatcher starts watching VMI events for each VM instance and updates database status in real-time
// It watches each VM individually using VM ID (app-id label selector)
func (s *VMStatusSyncService) StartWatcher(ctx context.Context) {
	s.logger.Info("Starting VMI watcher")

	// Get only active (non-deleted) applications to watch
	apps, err := s.store.Application().ListActive(ctx)
	if err != nil {
		s.logger.Errorw("Failed to list active applications", "error", err)
		return
	}

	if len(apps) == 0 {
		s.logger.Info("No active applications found to watch")
		return
	}

	s.logger.Infof("Starting watchers for %d active VM instances", len(apps))

	// Start a watcher for each VM instance
	var wg sync.WaitGroup
	for _, app := range apps {
		wg.Add(1)
		go func(vmID uuid.UUID, namespace string) {
			defer wg.Done()
			s.watchVMInstance(ctx, vmID, namespace)
		}(app.ID, app.Namespace)
	}

	// Wait for context cancellation
	<-ctx.Done()
	s.logger.Info("Stopping VMI watchers")
	s.stopAllWatchers()

	// Wait for all watchers to finish
	wg.Wait()
	s.logger.Info("All VMI watchers stopped")
}

// watchVMInstance watches VMI events for a specific VM instance using VM ID
func (s *VMStatusSyncService) watchVMInstance(ctx context.Context, vmID uuid.UUID, namespace string) {
	vmIDStr := vmID.String()
	s.logger.Infow("Starting VMI watcher for VM instance", "vm-id", vmIDStr, "namespace", namespace)

	// Watch VMIs with app-id label matching the VM ID
	watcher, err := s.client.VirtualMachineInstance(namespace).Watch(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app-id=%s", vmIDStr),
	})
	if err != nil {
		s.logger.Errorw("Failed to create VMI watcher", "vm-id", vmIDStr, "namespace", namespace, "error", err)
		// Retry after a delay
		time.Sleep(5 * time.Second)
		return
	}

	s.mu.Lock()
	s.watchers[vmIDStr] = watcher
	s.mu.Unlock()

	defer func() {
		watcher.Stop()
		s.mu.Lock()
		delete(s.watchers, vmIDStr)
		s.mu.Unlock()
		s.logger.Infow("Stopped VMI watcher", "vm-id", vmIDStr, "namespace", namespace)
	}()

	s.logger.Infow("VMI watcher established successfully", "vm-id", vmIDStr, "namespace", namespace)

	// Process events
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.ResultChan():
			if !ok {
				s.logger.Warnw("VMI watcher channel closed, retrying", "vm-id", vmIDStr, "namespace", namespace)
				// Retry watching after a delay
				time.Sleep(5 * time.Second)
				go s.watchVMInstance(ctx, vmID, namespace)
				return
			}

			if err := s.handleVMIEvent(ctx, event, vmIDStr); err != nil {
				s.logger.Errorw("Failed to handle VMI event",
					"vm-id", vmIDStr,
					"namespace", namespace,
					"event", event.Type,
					"error", err)
			}
		}
	}
}

// handleVMIEvent processes a VMI event and updates the database and external system
func (s *VMStatusSyncService) handleVMIEvent(ctx context.Context, event watch.Event, vmIDStr string) error {
	vmi, ok := event.Object.(*kubevirtv1.VirtualMachineInstance)
	if !ok {
		// Not a VMI event, skip
		return nil
	}

	timestamp := time.Now().Format(time.RFC3339)
	s.logger.Infow("VMI event received",
		"timestamp", timestamp,
		"event-type", event.Type,
		"vm-id", vmIDStr,
		"vmi-name", vmi.Name,
		"namespace", vmi.Namespace,
		"vmi-phase", vmi.Status.Phase)

	// Parse VM ID
	vmID, err := uuid.Parse(vmIDStr)
	if err != nil {
		s.logger.Warnw("Invalid VM ID", "vm-id", vmIDStr, "error", err)
		return nil
	}

	// Get the application from database
	app, err := s.store.Application().Get(ctx, vmID)
	if err != nil {
		// Application not found in database (may have been deleted), stop watching
		s.logger.Infow("Application not found in database, stopping watcher (may have been deleted)", "vm-id", vmIDStr)
		// Stop the watcher for this VM
		s.mu.Lock()
		if watcher, exists := s.watchers[vmIDStr]; exists {
			watcher.Stop()
			delete(s.watchers, vmIDStr)
		}
		s.mu.Unlock()
		return nil
	}

	// Check if application is deleted (status check as additional safeguard)
	if app.Status == StatusDeleted {
		s.logger.Infow("Application is deleted, stopping watcher", "vm-id", vmIDStr)
		// Stop the watcher for this VM
		s.mu.Lock()
		if watcher, exists := s.watchers[vmIDStr]; exists {
			watcher.Stop()
			delete(s.watchers, vmIDStr)
		}
		s.mu.Unlock()
		return nil
	}

	// Map VMI phase to application status
	newStatus := s.mapVMIPhaseToAppStatus(vmi.Status.Phase)

	// Update database if status changed
	oldStatus := app.Status
	if app.Status != newStatus {
		app.Status = newStatus
		if err := s.store.Application().Update(ctx, *app); err != nil {
			return fmt.Errorf("failed to update application status in database: %w", err)
		}
		s.logger.Infow("Updated application status in database",
			"vm-id", vmIDStr,
			"vmi-name", vmi.Name,
			"vmi-phase", vmi.Status.Phase,
			"old-status", oldStatus,
			"new-status", newStatus,
			"event-type", event.Type)
	}

	// Send status update to DCM system
	s.updateDCMStatus(vmIDStr, newStatus, vmi.Status.Phase)
	return nil
}

// updateDCMStatus sends status update to DCM system
func (s *VMStatusSyncService) updateDCMStatus(vmIDStr, status string, phase kubevirtv1.VirtualMachineInstancePhase) {
	if s.dcmUrl == "" {
		s.logger.Debugw("External API URL not configured, skipping status update", "vm-id", vmIDStr)
		return
	}

	url := fmt.Sprintf("%s/instances/%s/status", s.dcmUrl, vmIDStr)

	// Map VMI phase to external system phase format
	phaseStr := string(phase)
	message := fmt.Sprintf("The VMI is in %s Phase", phaseStr)
	payload := server.VMStatus{Status: status, Message: &message}

	resp, err := s.restyClient.R().
		SetBody(payload).
		Put(url)

	if err != nil {
		s.logger.Errorw("Error sending status update to external system", "vm-id", vmIDStr, "url", url, "error", err)
		return
	}

	if resp.StatusCode() != http.StatusOK {
		s.logger.Warnw("Status update returned non-success status", "vm-id", vmIDStr, "url", url, "status", resp.Status(), "status-code", resp.StatusCode())
	}
	s.logger.Infow("Successfully updated status in external system", "vm-id", vmIDStr, "url", url, "status-code", resp.StatusCode())

}

// mapVMIPhaseToAppStatus maps KubeVirt VMI phase to DCM status
func (s *VMStatusSyncService) mapVMIPhaseToAppStatus(phase kubevirtv1.VirtualMachineInstancePhase) string {
	switch phase {
	case kubevirtv1.Running:
		return StatusReady
	case kubevirtv1.Scheduled, kubevirtv1.Scheduling, kubevirtv1.Pending:
		return StatusInProgress
	case kubevirtv1.Succeeded:
		return StatusInProgress
	case kubevirtv1.Unknown:
		return StatusUnknown
	case kubevirtv1.Failed:
		return StatusFailed
	default:
		return StatusUnknown
	}
}

// stopAllWatchers stops all active watchers
func (s *VMStatusSyncService) stopAllWatchers() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for vmID, watcher := range s.watchers {
		watcher.Stop()
		s.logger.Infow("Stopped watcher", "vm-id", vmID)
	}
	s.watchers = make(map[string]watch.Interface)
}
