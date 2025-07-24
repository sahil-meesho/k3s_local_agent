package k3s

import (
	"context"
	"fmt"
	"time"

	"k3s-local-agent/pkg/logger"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
)

type K3sClient struct {
	clientset     *kubernetes.Clientset
	metricsClient *metricsclientset.Clientset
	logger        logger.Logger
	namespace     string
}

type NodeMetrics struct {
	Name            string            `json:"name"`
	CPUUsage        resource.Quantity `json:"cpu_usage"`
	MemoryUsage     resource.Quantity `json:"memory_usage"`
	CPUCapacity     resource.Quantity `json:"cpu_capacity"`
	MemoryCapacity  resource.Quantity `json:"memory_capacity"`
	CPUAvailable    resource.Quantity `json:"cpu_available"`
	MemoryAvailable resource.Quantity `json:"memory_available"`
	Timestamp       time.Time         `json:"timestamp"`
}

type PodMetrics struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	CPUUsage    resource.Quantity `json:"cpu_usage"`
	MemoryUsage resource.Quantity `json:"memory_usage"`
	Timestamp   time.Time         `json:"timestamp"`
}

type SchedulingDecision struct {
	PodName       string            `json:"pod_name"`
	TargetNode    string            `json:"target_node"`
	Reason        string            `json:"reason"`
	CPURequest    resource.Quantity `json:"cpu_request"`
	MemoryRequest resource.Quantity `json:"memory_request"`
	Timestamp     time.Time         `json:"timestamp"`
}

func NewK3sClient(logger logger.Logger, namespace string) (*K3sClient, error) {
	// Try to load in-cluster config first (when running inside K3s)
	config, err := rest.InClusterConfig()
	if err != nil {
		// Fall back to kubeconfig file
		kubeconfig := clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	metricsClient, err := metricsclientset.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics client: %w", err)
	}

	return &K3sClient{
		clientset:     clientset,
		metricsClient: metricsClient,
		logger:        logger,
		namespace:     namespace,
	}, nil
}

// GetNodeMetrics retrieves current resource metrics for all nodes
func (k *K3sClient) GetNodeMetrics() ([]NodeMetrics, error) {
	ctx := context.Background()

	// Get node list
	nodes, err := k.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	// Get node metrics
	nodeMetrics, err := k.metricsClient.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get node metrics: %w", err)
	}

	// Create a map of node metrics for easy lookup
	metricsMap := make(map[string]*metricsv1beta1.NodeMetrics)
	for i := range nodeMetrics.Items {
		metricsMap[nodeMetrics.Items[i].Name] = &nodeMetrics.Items[i]
	}

	var result []NodeMetrics
	for _, node := range nodes.Items {
		metrics := NodeMetrics{
			Name:      node.Name,
			Timestamp: time.Now(),
		}

		// Get node capacity
		cpuCapacity := node.Status.Capacity[v1.ResourceCPU]
		memoryCapacity := node.Status.Capacity[v1.ResourceMemory]
		metrics.CPUCapacity = cpuCapacity
		metrics.MemoryCapacity = memoryCapacity

		// Get node allocatable resources
		cpuAllocatable := node.Status.Allocatable[v1.ResourceCPU]
		memoryAllocatable := node.Status.Allocatable[v1.ResourceMemory]

		// Get current usage from metrics
		if nodeMetric, exists := metricsMap[node.Name]; exists {
			metrics.CPUUsage = nodeMetric.Usage[v1.ResourceCPU]
			metrics.MemoryUsage = nodeMetric.Usage[v1.ResourceMemory]
		}

		// Calculate available resources
		metrics.CPUAvailable = cpuAllocatable
		metrics.CPUAvailable.Sub(metrics.CPUUsage)
		metrics.MemoryAvailable = memoryAllocatable
		metrics.MemoryAvailable.Sub(metrics.MemoryUsage)

		result = append(result, metrics)
	}

	return result, nil
}

// GetPodMetrics retrieves current resource metrics for all pods
func (k *K3sClient) GetPodMetrics(namespace string) ([]PodMetrics, error) {
	ctx := context.Background()

	if namespace == "" {
		namespace = k.namespace
	}

	podMetrics, err := k.metricsClient.MetricsV1beta1().PodMetricses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod metrics: %w", err)
	}

	var result []PodMetrics
	for _, podMetric := range podMetrics.Items {
		// Sum up container metrics for the pod
		var totalCPU, totalMemory resource.Quantity
		for _, container := range podMetric.Containers {
			totalCPU.Add(container.Usage[v1.ResourceCPU])
			totalMemory.Add(container.Usage[v1.ResourceMemory])
		}

		metrics := PodMetrics{
			Name:        podMetric.Name,
			Namespace:   podMetric.Namespace,
			CPUUsage:    totalCPU,
			MemoryUsage: totalMemory,
			Timestamp:   time.Now(),
		}
		result = append(result, metrics)
	}

	return result, nil
}

// SchedulePod attempts to schedule a pod based on available resources
func (k *K3sClient) SchedulePod(podName, image string, cpuRequest, memoryRequest resource.Quantity) (*SchedulingDecision, error) {
	ctx := context.Background()

	// Get current node metrics
	nodeMetrics, err := k.GetNodeMetrics()
	if err != nil {
		return nil, fmt.Errorf("failed to get node metrics: %w", err)
	}

	// Find the best node for scheduling
	var bestNode string
	var bestReason string
	var maxScore float64

	for _, node := range nodeMetrics {
		// Check if node has enough resources
		if node.CPUAvailable.Cmp(cpuRequest) >= 0 && node.MemoryAvailable.Cmp(memoryRequest) >= 0 {
			// Calculate a simple score based on available resources
			cpuScore := float64(node.CPUAvailable.MilliValue()) / float64(node.CPUCapacity.MilliValue())
			memoryScore := float64(node.MemoryAvailable.Value()) / float64(node.MemoryCapacity.Value())
			score := (cpuScore + memoryScore) / 2

			if score > maxScore {
				maxScore = score
				bestNode = node.Name
				bestReason = fmt.Sprintf("Best resource availability (CPU: %.2f%%, Memory: %.2f%%)",
					cpuScore*100, memoryScore*100)
			}
		}
	}

	if bestNode == "" {
		return nil, fmt.Errorf("no suitable node found for pod %s", podName)
	}

	// Create the pod
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: k.namespace,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "main",
					Image: image,
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceCPU:    cpuRequest,
							v1.ResourceMemory: memoryRequest,
						},
						Limits: v1.ResourceList{
							v1.ResourceCPU:    cpuRequest,
							v1.ResourceMemory: memoryRequest,
						},
					},
				},
			},
			NodeSelector: map[string]string{
				"kubernetes.io/hostname": bestNode,
			},
		},
	}

	_, err = k.clientset.CoreV1().Pods(k.namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create pod: %w", err)
	}

	decision := &SchedulingDecision{
		PodName:       podName,
		TargetNode:    bestNode,
		Reason:        bestReason,
		CPURequest:    cpuRequest,
		MemoryRequest: memoryRequest,
		Timestamp:     time.Now(),
	}

	k.logger.Info("Pod scheduled successfully",
		"pod", podName,
		"node", bestNode,
		"reason", bestReason)

	return decision, nil
}

// GetClusterHealth returns overall cluster health information
func (k *K3sClient) GetClusterHealth() (map[string]interface{}, error) {
	ctx := context.Background()

	// Get nodes
	nodes, err := k.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	// Get pods
	pods, err := k.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	// Calculate cluster statistics
	var readyNodes, totalNodes int
	var runningPods, totalPods int

	for _, node := range nodes.Items {
		totalNodes++
		for _, condition := range node.Status.Conditions {
			if condition.Type == v1.NodeReady && condition.Status == v1.ConditionTrue {
				readyNodes++
				break
			}
		}
	}

	for _, pod := range pods.Items {
		totalPods++
		if pod.Status.Phase == v1.PodRunning {
			runningPods++
		}
	}

	health := map[string]interface{}{
		"total_nodes":  totalNodes,
		"ready_nodes":  readyNodes,
		"total_pods":   totalPods,
		"running_pods": runningPods,
		"node_health":  float64(readyNodes) / float64(totalNodes) * 100,
		"pod_health":   float64(runningPods) / float64(totalPods) * 100,
		"timestamp":    time.Now(),
	}

	return health, nil
}

// DeletePod deletes a pod by name
func (k *K3sClient) DeletePod(podName string) error {
	ctx := context.Background()

	err := k.clientset.CoreV1().Pods(k.namespace).Delete(ctx, podName, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete pod %s: %w", podName, err)
	}

	k.logger.Info("Pod deleted successfully", "pod", podName)
	return nil
}
