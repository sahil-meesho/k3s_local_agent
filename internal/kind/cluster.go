package kind

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"k3s-local-agent/internal/controlplane"
	"k3s-local-agent/pkg/logger"
)

type KindCluster struct {
	name        string
	logger      logger.Logger
	podReceiver *controlplane.PodReceiver
}

type KindClusterConfig struct {
	Name       string
	Port       int
	AgentID    string
	Kubeconfig string
}

type KindPodInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	Ready     string `json:"ready"`
	Restarts  string `json:"restarts"`
	Age       string `json:"age"`
	IP        string `json:"ip"`
	Node      string `json:"node"`
}

func NewKindCluster(config *KindClusterConfig, log logger.Logger) *KindCluster {
	return &KindCluster{
		name:   config.Name,
		logger: log,
	}
}

// CreateCluster creates a new Kind cluster
func (kc *KindCluster) CreateCluster() error {
	kc.logger.Info("Creating Kind cluster", "name", kc.name)

	cmd := exec.Command("kind", "create", "cluster", "--name", kc.name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w, output: %s", err, string(output))
	}

	kc.logger.Info("Kind cluster created successfully", "name", kc.name)
	return nil
}

// DeleteCluster deletes the Kind cluster
func (kc *KindCluster) DeleteCluster() error {
	kc.logger.Info("Deleting Kind cluster", "name", kc.name)

	cmd := exec.Command("kind", "delete", "cluster", "--name", kc.name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete Kind cluster: %w, output: %s", err, string(output))
	}

	kc.logger.Info("Kind cluster deleted successfully", "name", kc.name)
	return nil
}

// GetClusterStatus returns the status of the Kind cluster
func (kc *KindCluster) GetClusterStatus() (string, error) {
	cmd := exec.Command("kind", "get", "clusters")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get cluster status: %w", err)
	}

	clusters := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, cluster := range clusters {
		if cluster == kc.name {
			return "running", nil
		}
	}

	return "not-found", nil
}

// GetPods returns all pods in the Kind cluster
func (kc *KindCluster) GetPods() ([]KindPodInfo, error) {
	cmd := exec.Command("kubectl", "get", "pods", "--all-namespaces", "-o", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get pods: %w", err)
	}

	var result struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			Status struct {
				Phase             string `json:"phase"`
				PodIP             string `json:"podIP"`
				ContainerStatuses []struct {
					Ready        bool `json:"ready"`
					RestartCount int  `json:"restartCount"`
				} `json:"containerStatuses"`
			} `json:"status"`
			Spec struct {
				NodeName string `json:"nodeName"`
			} `json:"spec"`
		} `json:"items"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse pod data: %w", err)
	}

	var pods []KindPodInfo
	for _, item := range result.Items {
		ready := "0/1"
		restarts := "0"
		if len(item.Status.ContainerStatuses) > 0 {
			if item.Status.ContainerStatuses[0].Ready {
				ready = "1/1"
			}
			restarts = fmt.Sprintf("%d", item.Status.ContainerStatuses[0].RestartCount)
		}

		pod := KindPodInfo{
			Name:      item.Metadata.Name,
			Namespace: item.Metadata.Namespace,
			Status:    item.Status.Phase,
			Ready:     ready,
			Restarts:  restarts,
			IP:        item.Status.PodIP,
			Node:      item.Spec.NodeName,
		}
		pods = append(pods, pod)
	}

	return pods, nil
}

// CreatePod creates a pod in the Kind cluster
func (kc *KindCluster) CreatePod(name, namespace, image string) error {
	kc.logger.Info("Creating pod in Kind cluster", "name", name, "namespace", namespace, "image", image)

	cmd := exec.Command("kubectl", "run", name, "--image", image, "-n", namespace)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create pod: %w, output: %s", err, string(output))
	}

	kc.logger.Info("Pod created successfully", "name", name, "namespace", namespace)
	return nil
}

// DeletePod deletes a pod from the Kind cluster
func (kc *KindCluster) DeletePod(name, namespace string) error {
	kc.logger.Info("Deleting pod from Kind cluster", "name", name, "namespace", namespace)

	cmd := exec.Command("kubectl", "delete", "pod", name, "-n", namespace)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete pod: %w, output: %s", err, string(output))
	}

	kc.logger.Info("Pod deleted successfully", "name", name, "namespace", namespace)
	return nil
}

// GetClusterInfo returns basic cluster information
func (kc *KindCluster) GetClusterInfo() (map[string]interface{}, error) {
	info := make(map[string]interface{})

	// Get cluster status
	status, err := kc.GetClusterStatus()
	if err != nil {
		return nil, err
	}
	info["status"] = status
	info["name"] = kc.name

	// Get nodes
	cmd := exec.Command("kubectl", "get", "nodes", "-o", "json")
	output, err := cmd.CombinedOutput()
	if err == nil {
		var nodes struct {
			Items []struct {
				Metadata struct {
					Name string `json:"name"`
				} `json:"metadata"`
				Status struct {
					Conditions []struct {
						Type   string `json:"type"`
						Status string `json:"status"`
					} `json:"conditions"`
				} `json:"status"`
			} `json:"items"`
		}

		if err := json.Unmarshal(output, &nodes); err == nil {
			var readyNodes, totalNodes int
			for _, node := range nodes.Items {
				totalNodes++
				for _, condition := range node.Status.Conditions {
					if condition.Type == "Ready" && condition.Status == "True" {
						readyNodes++
						break
					}
				}
			}
			info["total_nodes"] = totalNodes
			info["ready_nodes"] = readyNodes
			info["node_health"] = float64(readyNodes) / float64(totalNodes) * 100
		}
	}

	// Get pods
	pods, err := kc.GetPods()
	if err == nil {
		var runningPods, totalPods int
		for _, pod := range pods {
			totalPods++
			if pod.Status == "Running" {
				runningPods++
			}
		}
		info["total_pods"] = totalPods
		info["running_pods"] = runningPods
		if totalPods > 0 {
			info["pod_health"] = float64(runningPods) / float64(totalPods) * 100
		} else {
			info["pod_health"] = 0
		}
	}

	info["timestamp"] = time.Now()
	return info, nil
}

// SyncPodsFromControlPlane syncs pod data from control plane to Kind cluster
func (kc *KindCluster) SyncPodsFromControlPlane(podReceiver *controlplane.PodReceiver) error {
	kc.logger.Info("Syncing pods from control plane to Kind cluster")

	pods := podReceiver.GetPodData()

	// Get existing pods in Kind cluster
	existingPods, err := kc.GetPods()
	if err != nil {
		return fmt.Errorf("failed to get existing pods: %w", err)
	}

	// Create a map of existing pods for quick lookup
	existingPodMap := make(map[string]bool)
	for _, pod := range existingPods {
		key := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
		existingPodMap[key] = true
	}

	// Create pods that don't exist in Kind cluster
	for _, pod := range pods {
		key := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
		if !existingPodMap[key] {
			if err := kc.CreatePod(pod.Name, pod.Namespace, pod.Image); err != nil {
				kc.logger.Error("Failed to create pod in Kind cluster",
					"name", pod.Name,
					"namespace", pod.Namespace,
					"error", err)
			} else {
				kc.logger.Info("Created pod in Kind cluster",
					"name", pod.Name,
					"namespace", pod.Namespace)
			}
		}
	}

	kc.logger.Info("Pod sync completed", "control_plane_pods", len(pods), "kind_pods", len(existingPods))
	return nil
}
