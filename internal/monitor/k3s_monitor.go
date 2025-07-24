package monitor

import (
	"encoding/json"
	"fmt"
	"time"

	"k3s-local-agent/internal/config"
	"k3s-local-agent/internal/k3s"
	"k3s-local-agent/pkg/logger"

	"k8s.io/apimachinery/pkg/api/resource"
)

type K3sResourceMonitor struct {
	config       *config.Config
	logger       logger.Logger
	k3sClient    *k3s.K3sClient
	localMonitor ResourceMonitor
}

type K3sResourceData struct {
	LocalSystem   *ResourceData          `json:"local_system"`
	ClusterHealth map[string]interface{} `json:"cluster_health"`
	NodeMetrics   []k3s.NodeMetrics      `json:"node_metrics"`
	PodMetrics    []k3s.PodMetrics       `json:"pod_metrics"`
	Timestamp     time.Time              `json:"timestamp"`
}

type K3sSchedulingInfo struct {
	LocalResources  *ResourceData     `json:"local_resources"`
	ClusterMetrics  []k3s.NodeMetrics `json:"cluster_metrics"`
	BestNode        string            `json:"best_node"`
	AvailableCPU    resource.Quantity `json:"available_cpu"`
	AvailableMemory resource.Quantity `json:"available_memory"`
	Recommendation  string            `json:"recommendation"`
	Timestamp       time.Time         `json:"timestamp"`
}

func NewK3sResourceMonitor(cfg *config.Config, log logger.Logger, namespace string) (*K3sResourceMonitor, error) {
	// Create local monitor
	localMonitor := New(cfg, log)

	// Create K3s client
	k3sClient, err := k3s.NewK3sClient(log, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to create K3s client: %w", err)
	}

	return &K3sResourceMonitor{
		config:       cfg,
		logger:       log,
		k3sClient:    k3sClient,
		localMonitor: localMonitor,
	}, nil
}

// GetAllK3sResources combines local system data with K3s cluster data
func (m *K3sResourceMonitor) GetAllK3sResources() (*K3sResourceData, error) {
	// Get local system resources
	localData, err := m.localMonitor.GetAllResources()
	if err != nil {
		return nil, fmt.Errorf("failed to get local resources: %w", err)
	}

	// Get cluster health
	clusterHealth, err := m.k3sClient.GetClusterHealth()
	if err != nil {
		m.logger.Warn("Failed to get cluster health", "error", err)
		clusterHealth = map[string]interface{}{
			"error": err.Error(),
		}
	}

	// Get node metrics
	nodeMetrics, err := m.k3sClient.GetNodeMetrics()
	if err != nil {
		m.logger.Warn("Failed to get node metrics", "error", err)
		nodeMetrics = []k3s.NodeMetrics{}
	}

	// Get pod metrics
	podMetrics, err := m.k3sClient.GetPodMetrics("")
	if err != nil {
		m.logger.Warn("Failed to get pod metrics", "error", err)
		podMetrics = []k3s.PodMetrics{}
	}

	return &K3sResourceData{
		LocalSystem:   localData,
		ClusterHealth: clusterHealth,
		NodeMetrics:   nodeMetrics,
		PodMetrics:    podMetrics,
		Timestamp:     time.Now(),
	}, nil
}

// GetSchedulingRecommendation provides scheduling advice based on current resources
func (m *K3sResourceMonitor) GetSchedulingRecommendation() (*K3sSchedulingInfo, error) {
	// Get local resources
	localData, err := m.localMonitor.GetAllResources()
	if err != nil {
		return nil, fmt.Errorf("failed to get local resources: %w", err)
	}

	// Get cluster metrics
	nodeMetrics, err := m.k3sClient.GetNodeMetrics()
	if err != nil {
		return nil, fmt.Errorf("failed to get node metrics: %w", err)
	}

	// Find the best node for scheduling
	var bestNode string
	var bestCPU, bestMemory resource.Quantity
	var recommendation string

	if len(nodeMetrics) == 0 {
		recommendation = "No nodes available in cluster"
	} else {
		// Find node with most available resources
		var maxScore float64
		for _, node := range nodeMetrics {
			if node.CPUAvailable.Cmp(bestCPU) > 0 || bestCPU.IsZero() {
				bestCPU = node.CPUAvailable
				bestMemory = node.MemoryAvailable
				bestNode = node.Name

				// Calculate score
				cpuScore := float64(node.CPUAvailable.MilliValue()) / float64(node.CPUCapacity.MilliValue())
				memoryScore := float64(node.MemoryAvailable.Value()) / float64(node.MemoryCapacity.Value())
				score := (cpuScore + memoryScore) / 2

				if score > maxScore {
					maxScore = score
				}
			}
		}

		recommendation = fmt.Sprintf("Best node: %s (CPU: %s, Memory: %s, Score: %.2f%%)",
			bestNode, bestCPU.String(), bestMemory.String(), maxScore*100)
	}

	return &K3sSchedulingInfo{
		LocalResources:  localData,
		ClusterMetrics:  nodeMetrics,
		BestNode:        bestNode,
		AvailableCPU:    bestCPU,
		AvailableMemory: bestMemory,
		Recommendation:  recommendation,
		Timestamp:       time.Now(),
	}, nil
}

// ScheduleWorkload schedules a pod with the specified requirements
func (m *K3sResourceMonitor) ScheduleWorkload(podName, image string, cpuRequest, memoryRequest resource.Quantity) (*k3s.SchedulingDecision, error) {
	decision, err := m.k3sClient.SchedulePod(podName, image, cpuRequest, memoryRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to schedule pod: %w", err)
	}

	m.logger.Info("Workload scheduled successfully",
		"pod", podName,
		"node", decision.TargetNode,
		"cpu", cpuRequest.String(),
		"memory", memoryRequest.String())

	return decision, nil
}

// GetResourceSummary provides a human-readable summary of current resources
func (m *K3sResourceMonitor) GetResourceSummary() (string, error) {
	data, err := m.GetAllK3sResources()
	if err != nil {
		return "", err
	}

	summary := fmt.Sprintf("=== K3s Resource Summary ===\n")
	summary += fmt.Sprintf("Timestamp: %s\n\n", data.Timestamp.Format("2006-01-02 15:04:05"))

	// Local system info
	if data.LocalSystem != nil {
		summary += fmt.Sprintf("Local System:\n")
		summary += fmt.Sprintf("  Hostname: %s\n", data.LocalSystem.System.Hostname)
		summary += fmt.Sprintf("  CPU Usage: %.2f%%\n", data.LocalSystem.CPU.UsagePercent)
		summary += fmt.Sprintf("  Memory Usage: %.2f%%\n", data.LocalSystem.Memory.UsedPercent)
		summary += fmt.Sprintf("  Online: %v\n", data.LocalSystem.Health.IsOnline)
		summary += fmt.Sprintf("  Internet: %v\n\n", data.LocalSystem.Health.HasInternet)
	}

	// Cluster health
	if data.ClusterHealth != nil {
		summary += fmt.Sprintf("Cluster Health:\n")
		if totalNodes, ok := data.ClusterHealth["total_nodes"].(int); ok {
			summary += fmt.Sprintf("  Total Nodes: %d\n", totalNodes)
		}
		if readyNodes, ok := data.ClusterHealth["ready_nodes"].(int); ok {
			summary += fmt.Sprintf("  Ready Nodes: %d\n", readyNodes)
		}
		if totalPods, ok := data.ClusterHealth["total_pods"].(int); ok {
			summary += fmt.Sprintf("  Total Pods: %d\n", totalPods)
		}
		if runningPods, ok := data.ClusterHealth["running_pods"].(int); ok {
			summary += fmt.Sprintf("  Running Pods: %d\n", runningPods)
		}
		if nodeHealth, ok := data.ClusterHealth["node_health"].(float64); ok {
			summary += fmt.Sprintf("  Node Health: %.2f%%\n", nodeHealth)
		}
		if podHealth, ok := data.ClusterHealth["pod_health"].(float64); ok {
			summary += fmt.Sprintf("  Pod Health: %.2f%%\n", podHealth)
		}
		summary += "\n"
	}

	// Node metrics
	if len(data.NodeMetrics) > 0 {
		summary += fmt.Sprintf("Node Metrics:\n")
		for _, node := range data.NodeMetrics {
			summary += fmt.Sprintf("  %s:\n", node.Name)
			summary += fmt.Sprintf("    CPU Available: %s / %s\n", node.CPUAvailable.String(), node.CPUCapacity.String())
			summary += fmt.Sprintf("    Memory Available: %s / %s\n", node.MemoryAvailable.String(), node.MemoryCapacity.String())
		}
		summary += "\n"
	}

	// Pod metrics
	if len(data.PodMetrics) > 0 {
		summary += fmt.Sprintf("Pod Metrics:\n")
		for _, pod := range data.PodMetrics {
			summary += fmt.Sprintf("  %s/%s:\n", pod.Namespace, pod.Name)
			summary += fmt.Sprintf("    CPU Usage: %s\n", pod.CPUUsage.String())
			summary += fmt.Sprintf("    Memory Usage: %s\n", pod.MemoryUsage.String())
		}
	}

	return summary, nil
}

// ExportToJSON exports the complete resource data to JSON
func (m *K3sResourceMonitor) ExportToJSON() ([]byte, error) {
	data, err := m.GetAllK3sResources()
	if err != nil {
		return nil, err
	}

	return json.MarshalIndent(data, "", "  ")
}

// MonitorCluster continuously monitors the cluster and logs changes
func (m *K3sResourceMonitor) MonitorCluster(interval time.Duration, stopCh chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	m.logger.Info("Starting cluster monitoring", "interval", interval)

	for {
		select {
		case <-ticker.C:
			if err := m.logClusterStatus(); err != nil {
				m.logger.Error("Failed to log cluster status", "error", err)
			}
		case <-stopCh:
			m.logger.Info("Cluster monitoring stopped")
			return
		}
	}
}

func (m *K3sResourceMonitor) logClusterStatus() error {
	data, err := m.GetAllK3sResources()
	if err != nil {
		return err
	}

	// Log cluster health
	if data.ClusterHealth != nil {
		if readyNodes, ok := data.ClusterHealth["ready_nodes"].(int); ok {
			if totalNodes, ok := data.ClusterHealth["total_nodes"].(int); ok {
				m.logger.Info("Cluster status",
					"ready_nodes", readyNodes,
					"total_nodes", totalNodes,
					"health_percent", float64(readyNodes)/float64(totalNodes)*100)
			}
		}
	}

	// Log node metrics
	for _, node := range data.NodeMetrics {
		cpuPercent := float64(node.CPUUsage.MilliValue()) / float64(node.CPUCapacity.MilliValue()) * 100
		memoryPercent := float64(node.MemoryUsage.Value()) / float64(node.MemoryCapacity.Value()) * 100

		m.logger.Info("Node metrics",
			"node", node.Name,
			"cpu_usage_percent", cpuPercent,
			"memory_usage_percent", memoryPercent,
			"cpu_available", node.CPUAvailable.String(),
			"memory_available", node.MemoryAvailable.String())
	}

	return nil
}
