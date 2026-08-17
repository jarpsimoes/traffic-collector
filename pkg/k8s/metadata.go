package k8s

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// PodInfo contains Kubernetes pod metadata
type PodInfo struct {
	PID       uint32
	Namespace string
	PodName   string
	Node      string
	Labels    map[string]string
}

// Metadata provides Kubernetes metadata enrichment
type Metadata struct {
	client   kubernetes.Interface
	logger   *zap.SugaredLogger
	mu       sync.RWMutex
	podCache map[uint32]*PodInfo
	cacheTTL int64
}

// NewMetadata creates a new Kubernetes metadata provider
func NewMetadata(logger *zap.SugaredLogger) (*Metadata, error) {
	if logger == nil {
		logger = zap.NewNop().Sugar()
	}

	// Create in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create in-cluster config: %w", err)
	}

	// Create Kubernetes client
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	logger.Infow("Kubernetes metadata provider initialized")

	return &Metadata{
		client:   client,
		logger:   logger,
		podCache: make(map[uint32]*PodInfo),
	}, nil
}

// GetPodInfo retrieves pod metadata by PID
func (m *Metadata) GetPodInfo(pid uint32) *PodInfo {
	m.mu.RLock()
	podInfo, exists := m.podCache[pid]
	m.mu.RUnlock()

	if exists {
		return podInfo
	}

	// Fetch pod info from Kubernetes API
	podInfo = m.fetchPodInfo(pid)

	if podInfo != nil {
		m.mu.Lock()
		m.podCache[pid] = podInfo
		m.mu.Unlock()
	}

	return podInfo
}

func (m *Metadata) fetchPodInfo(pid uint32) *PodInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 5*1e9)
	defer cancel()

	// List all pods in the cluster
	pods, err := m.client.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		m.logger.Debugw("Failed to list pods", "error", err)
		return nil
	}

	// Try to match PID to pod
	for _, pod := range pods.Items {
		if m.podMatchesPID(pod, pid) {
			return &PodInfo{
				PID:       pid,
				Namespace: pod.Namespace,
				PodName:   pod.Name,
				Node:      pod.Spec.NodeName,
				Labels:    pod.Labels,
			}
		}
	}

	return nil
}

func (m *Metadata) podMatchesPID(pod corev1.Pod, pid uint32) bool {
	// This is a simplified check. In production, you would need to
	// map container PIDs to pod info using cgroup information
	// or by reading /proc/pid/cgroup

	// For now, return false - this would be implemented with
	// actual PID-to-cgroup mapping
	return false
}

// ClearCache clears the pod cache
func (m *Metadata) ClearCache() {
	m.mu.Lock()
	m.podCache = make(map[uint32]*PodInfo)
	m.mu.Unlock()
}

// Close closes the metadata provider
func (m *Metadata) Close() error {
	return nil
}
