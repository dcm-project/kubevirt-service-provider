package service

import (
	"context"

	"github.com/dcm-project/service-provider-api/internal/service/model"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type ClusterResourceService struct {
	kubeClient kubernetes.Interface
}

func NewClusterResourceService(kubeClient kubernetes.Interface) *ClusterResourceService {
	return &ClusterResourceService{kubeClient: kubeClient}
}

func (c *ClusterResourceService) GetClusterResources(ctx context.Context) (*model.ClusterResource, error) {
	logger := zap.S().Named("cluster-resource")
	logger.Debug("Retrieving cluster resources information")
	// Get node information
	nodes, err := c.kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var availableCPU, availableMemory, availableStorage resource.Quantity

	for _, node := range nodes.Items {
		availableCPU.Add(*node.Status.Allocatable.Cpu())
		availableMemory.Add(*node.Status.Allocatable.Memory())
		availableStorage.Add(*node.Status.Allocatable.Storage())
	}

	return &model.ClusterResource{
		AvailableCPU:     availableCPU.String(),
		AvailableMemory:  availableMemory.String(),
		AvailableStorage: availableStorage.String(),
	}, nil
}
