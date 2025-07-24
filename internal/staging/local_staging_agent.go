package staging

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"k3s-local-agent/internal/controlplane"
	"k3s-local-agent/internal/kind"
	"k3s-local-agent/pkg/logger"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// LocalStagingAgent manages staging pods locally
type LocalStagingAgent struct {
	config           *StagingConfig
	logger           logger.Logger
	podReceiver      *controlplane.PodReceiver
	kindCluster      *kind.KindCluster
	k8sClient        *kubernetes.Clientset
	stagingPods      map[string]StagingPodInfo
	cloudflareTunnel *CloudflareTunnelManager
	httpProxy        *HTTPProxyManager
	mutex            sync.RWMutex
	stopCh           chan struct{}
	agentID          string
	controlPlaneURL  string
}

// StagingConfig holds configuration for local staging
type StagingConfig struct {
	AgentID          string
	ControlPlaneURL  string
	ControlPlanePort int
	KindClusterName  string
	LocalNamespace   string
	AgentPort        int
	SyncInterval     time.Duration
}

// StagingPodInfo represents a staging pod from GCS
type StagingPodInfo struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Namespace     string            `json:"namespace"`
	Image         string            `json:"image"`
	Status        string            `json:"status"`
	CPURequest    string            `json:"cpu_request"`
	MemoryRequest string            `json:"memory_request"`
	CPULimit      string            `json:"cpu_limit"`
	MemoryLimit   string            `json:"memory_limit"`
	IP            string            `json:"ip"`
	NodeName      string            `json:"node_name"`
	Labels        map[string]string `json:"labels"`
	Annotations   map[string]string `json:"annotations"`
	Ports         []ContainerPort   `json:"ports"`
	Environment   []EnvVar          `json:"environment"`
	VolumeMounts  []VolumeMount     `json:"volume_mounts"`
	StagingSource string            `json:"staging_source"` // GCS cluster info
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	LocalStatus   string            `json:"local_status"` // "created", "running", "failed", "not_created"
}

// ContainerPort represents container port configuration
type ContainerPort struct {
	Name          string `json:"name"`
	ContainerPort int32  `json:"container_port"`
	Protocol      string `json:"protocol"`
}

// EnvVar represents environment variable
type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// VolumeMount represents volume mount configuration
type VolumeMount struct {
	Name      string `json:"name"`
	MountPath string `json:"mount_path"`
}

// StagingStatus represents overall staging status
type StagingStatus struct {
	AgentID           string                    `json:"agent_id"`
	Status            string                    `json:"status"`
	TotalPods         int                       `json:"total_pods"`
	RunningPods       int                       `json:"running_pods"`
	FailedPods        int                       `json:"failed_pods"`
	StagingPods       map[string]StagingPodInfo `json:"staging_pods"`
	KindClusterStatus string                    `json:"kind_cluster_status"`
	LastSync          time.Time                 `json:"last_sync"`
	Timestamp         time.Time                 `json:"timestamp"`
}

func NewLocalStagingAgent(config *StagingConfig, log logger.Logger) (*LocalStagingAgent, error) {
	// Create pod receiver
	podReceiver := controlplane.NewPodReceiver(config.AgentPort, config.AgentID, log)

	// Create kind cluster
	kindConfig := &kind.KindClusterConfig{
		Name:       config.KindClusterName,
		Port:       8080,
		AgentID:    config.AgentID,
		Kubeconfig: "",
	}
	kindCluster := kind.NewKindCluster(kindConfig, log)

	// Create Kubernetes client
	k8sClient, err := createK8sClient()
	if err != nil {
		log.Warn("Failed to create K8s client, continuing without cluster access", "error", err)
	}

	// Create Cloudflare tunnel manager
	tunnelConfig := &TunnelConfig{
		AgentID:   config.AgentID,
		Hostname:  fmt.Sprintf("%s-agent.trycloudflare.com", config.AgentID),
		LocalPort: 8082,
		Protocol:  "quic",
		AutoStart: true,
	}
	cloudflareTunnel := NewCloudflareTunnelManager(tunnelConfig, log)

	// Create HTTP proxy manager
	proxyConfig := &ProxyConfig{
		AgentID:   config.AgentID,
		ProxyPort: 8080,
		BasePath:  "/",
		EnableSSL: false,
	}
	httpProxy := NewHTTPProxyManager(proxyConfig, log)

	return &LocalStagingAgent{
		config:           config,
		logger:           log,
		podReceiver:      podReceiver,
		kindCluster:      kindCluster,
		k8sClient:        k8sClient,
		stagingPods:      make(map[string]StagingPodInfo),
		cloudflareTunnel: cloudflareTunnel,
		httpProxy:        httpProxy,
		stopCh:           make(chan struct{}),
		agentID:          config.AgentID,
		controlPlaneURL:  config.ControlPlaneURL,
	}, nil
}

// Start starts the local staging agent
func (lsa *LocalStagingAgent) Start() error {
	lsa.logger.Info("Starting local staging agent...")

	// Start pod receiver server
	if err := lsa.podReceiver.Start(); err != nil {
		return fmt.Errorf("failed to start pod receiver: %w", err)
	}

	// Setup Cloudflare tunnel
	if lsa.cloudflareTunnel != nil {
		lsa.logger.Info("Setting up Cloudflare tunnel...")
		if tunnel, err := lsa.cloudflareTunnel.SetupTunnel(); err != nil {
			lsa.logger.Error("Failed to setup Cloudflare tunnel", "error", err)
		} else {
			lsa.logger.Info("Cloudflare tunnel setup successful",
				"hostname", tunnel.Hostname,
				"public_url", tunnel.PublicURL,
				"status", tunnel.Status)
		}
	}

	// Start HTTP proxy server
	if lsa.httpProxy != nil {
		lsa.logger.Info("Starting HTTP proxy server...")
		// The HTTP proxy server will start automatically when first proxy is created
		// For now, we'll start it with a health check endpoint
		go func() {
			if err := lsa.httpProxy.startHTTPServer(); err != nil {
				lsa.logger.Error("HTTP proxy server failed", "error", err)
			}
		}()
	}

	// Create kind cluster if it doesn't exist
	if err := lsa.setupKindCluster(); err != nil {
		lsa.logger.Warn("Failed to setup kind cluster", "error", err)
	}

	// Start staging pod management
	go lsa.manageStagingPods()

	// Start control plane communication
	go lsa.communicateWithControlPlane()

	// Register with control plane at startup
	go lsa.registerWithControlPlane()

	lsa.logger.Info("Local staging agent started successfully")
	return nil
}

// Stop stops the local staging agent
func (lsa *LocalStagingAgent) Stop() error {
	lsa.logger.Info("Stopping local staging agent...")
	close(lsa.stopCh)

	if lsa.podReceiver != nil {
		lsa.podReceiver.Stop()
	}

	lsa.logger.Info("Local staging agent stopped successfully")
	return nil
}

// setupKindCluster creates and configures the kind cluster
func (lsa *LocalStagingAgent) setupKindCluster() error {
	// Check if cluster exists
	status, err := lsa.kindCluster.GetClusterStatus()
	if err != nil {
		return fmt.Errorf("failed to check cluster status: %w", err)
	}

	if status == "not-found" {
		lsa.logger.Info("Creating kind cluster for staging pods...")
		if err := lsa.kindCluster.CreateCluster(); err != nil {
			return fmt.Errorf("failed to create kind cluster: %w", err)
		}
	} else {
		lsa.logger.Info("Kind cluster already exists", "status", status)
	}

	return nil
}

// manageStagingPods manages staging pods locally
func (lsa *LocalStagingAgent) manageStagingPods() {
	ticker := time.NewTicker(lsa.config.SyncInterval)
	defer ticker.Stop()

	// Initial sync
	lsa.syncStagingPods()

	for {
		select {
		case <-ticker.C:
			lsa.syncStagingPods()
		case <-lsa.stopCh:
			lsa.logger.Info("Staging pod management stopped")
			return
		}
	}
}

// syncStagingPods synchronizes staging pods from control plane to local cluster
func (lsa *LocalStagingAgent) syncStagingPods() {
	lsa.logger.Debug("Syncing staging pods from control plane...")

	// Get pod data from receiver
	podData := lsa.podReceiver.GetPodData()

	lsa.mutex.Lock()
	defer lsa.mutex.Unlock()

	// Process each pod
	for _, pod := range podData {
		stagingPod := lsa.convertToStagingPod(pod)

		// Check if pod already exists locally
		if existingPod, exists := lsa.stagingPods[stagingPod.ID]; exists {
			if existingPod.LocalStatus == "not_created" || existingPod.LocalStatus == "failed" {
				// Try to create the pod locally
				if err := lsa.createStagingPodLocally(stagingPod); err != nil {
					lsa.logger.Error("Failed to create staging pod locally",
						"pod", stagingPod.Name,
						"error", err)
					stagingPod.LocalStatus = "failed"
				} else {
					stagingPod.LocalStatus = "created"
				}
			}
		} else {
			// New pod, create it locally
			if err := lsa.createStagingPodLocally(stagingPod); err != nil {
				lsa.logger.Error("Failed to create new staging pod locally",
					"pod", stagingPod.Name,
					"error", err)
				stagingPod.LocalStatus = "failed"
			} else {
				stagingPod.LocalStatus = "created"
			}
		}

		stagingPod.UpdatedAt = time.Now()
		lsa.stagingPods[stagingPod.ID] = stagingPod
	}

	lsa.logger.Info("Staging pods sync completed",
		"total_pods", len(podData),
		"local_pods", len(lsa.stagingPods))
}

// convertToStagingPod converts PodInfo to StagingPodInfo
func (lsa *LocalStagingAgent) convertToStagingPod(pod controlplane.PodInfo) StagingPodInfo {
	return StagingPodInfo{
		ID:            pod.ID,
		Name:          pod.Name,
		Namespace:     pod.Namespace,
		Image:         pod.Image,
		Status:        pod.Status,
		IP:            pod.IP,
		NodeName:      pod.NodeName,
		Labels:        pod.Labels,
		StagingSource: "GCS-Staging-Cluster",
		CreatedAt:     pod.CreatedAt,
		UpdatedAt:     pod.UpdatedAt,
		LocalStatus:   "not_created",
		// Set default resource requests
		CPURequest:    "100m",
		MemoryRequest: "128Mi",
		CPULimit:      "200m",
		MemoryLimit:   "256Mi",
		Ports: []ContainerPort{
			{
				Name:          "http",
				ContainerPort: 80,
				Protocol:      "TCP",
			},
		},
	}
}

// createStagingPodLocally creates a staging pod in the local kind cluster
func (lsa *LocalStagingAgent) createStagingPodLocally(pod StagingPodInfo) error {
	if lsa.k8sClient == nil {
		return fmt.Errorf("K8s client not available")
	}

	ctx := context.Background()

	// Parse resource requests
	cpuRequest, err := resource.ParseQuantity(pod.CPURequest)
	if err != nil {
		return fmt.Errorf("invalid CPU request: %w", err)
	}

	memoryRequest, err := resource.ParseQuantity(pod.MemoryRequest)
	if err != nil {
		return fmt.Errorf("invalid memory request: %w", err)
	}

	cpuLimit, err := resource.ParseQuantity(pod.CPULimit)
	if err != nil {
		return fmt.Errorf("invalid CPU limit: %w", err)
	}

	memoryLimit, err := resource.ParseQuantity(pod.MemoryLimit)
	if err != nil {
		return fmt.Errorf("invalid memory limit: %w", err)
	}

	// Create container ports
	var containerPorts []v1.ContainerPort
	for _, port := range pod.Ports {
		containerPorts = append(containerPorts, v1.ContainerPort{
			Name:          port.Name,
			ContainerPort: port.ContainerPort,
			Protocol:      v1.Protocol(port.Protocol),
		})
	}

	// Create environment variables
	var envVars []v1.EnvVar
	for _, env := range pod.Environment {
		envVars = append(envVars, v1.EnvVar{
			Name:  env.Name,
			Value: env.Value,
		})
	}

	// Create the pod
	k8sPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        pod.Name,
			Namespace:   pod.Namespace,
			Labels:      pod.Labels,
			Annotations: pod.Annotations,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "main",
					Image: pod.Image,
					Ports: containerPorts,
					Env:   envVars,
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceCPU:    cpuRequest,
							v1.ResourceMemory: memoryRequest,
						},
						Limits: v1.ResourceList{
							v1.ResourceCPU:    cpuLimit,
							v1.ResourceMemory: memoryLimit,
						},
					},
				},
			},
		},
	}

	createdPod, err := lsa.k8sClient.CoreV1().Pods(pod.Namespace).Create(ctx, k8sPod, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create pod: %w", err)
	}

	lsa.logger.Info("Created staging pod locally",
		"pod", pod.Name,
		"namespace", pod.Namespace,
		"image", pod.Image)

	// Setup HTTP proxy if staging pod has an IP
	if pod.IP != "" {
		// Get the local pod IP (will be assigned by Kubernetes)
		localPodIP := createdPod.Status.PodIP
		if localPodIP == "" {
			lsa.logger.Warn("Local pod IP not yet assigned, will setup proxy later",
				"pod", pod.Name)
		} else {
			// Setup HTTP proxy
			proxy, err := lsa.httpProxy.SetupProxy(pod, localPodIP)
			if err != nil {
				lsa.logger.Error("Failed to setup HTTP proxy",
					"pod", pod.Name,
					"staging_ip", pod.IP,
					"local_ip", localPodIP,
					"error", err)
			} else {
				lsa.logger.Info("HTTP proxy setup successful",
					"pod", pod.Name,
					"staging_ip", proxy.StagingPodIP,
					"proxy_url", proxy.ProxyURL)
			}
		}
	}

	return nil
}

// communicateWithControlPlane communicates with the control plane
func (lsa *LocalStagingAgent) communicateWithControlPlane() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Initial communication
	lsa.sendStatusToControlPlane()

	for {
		select {
		case <-ticker.C:
			lsa.sendStatusToControlPlane()
		case <-lsa.stopCh:
			lsa.logger.Info("Control plane communication stopped")
			return
		}
	}
}

// sendStatusToControlPlane sends staging status to control plane
func (lsa *LocalStagingAgent) sendStatusToControlPlane() {
	lsa.logger.Debug("Sending staging status to control plane...")

	status := lsa.GetStagingStatus()

	// Create HTTP client
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Send status to control plane
	url := fmt.Sprintf("%s/api/v1/staging/status", lsa.controlPlaneURL)
	jsonData, err := json.Marshal(status)
	if err != nil {
		lsa.logger.Error("Failed to marshal status", "error", err)
		return
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		lsa.logger.Error("Failed to create request", "error", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-ID", lsa.agentID)

	resp, err := client.Do(req)
	if err != nil {
		lsa.logger.Error("Failed to send status to control plane", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		lsa.logger.Error("Control plane returned error status", "status", resp.StatusCode)
		return
	}

	lsa.logger.Info("Successfully sent staging status to control plane",
		"total_pods", status.TotalPods,
		"running_pods", status.RunningPods)
}

// GetStagingStatus returns the current staging status
func (lsa *LocalStagingAgent) GetStagingStatus() *StagingStatus {
	lsa.mutex.RLock()
	defer lsa.mutex.RUnlock()

	var runningPods, failedPods int
	for _, pod := range lsa.stagingPods {
		switch pod.LocalStatus {
		case "running":
			runningPods++
		case "failed":
			failedPods++
		}
	}

	// Get kind cluster status
	clusterStatus := "unknown"
	if lsa.kindCluster != nil {
		if status, err := lsa.kindCluster.GetClusterStatus(); err == nil {
			clusterStatus = status
		}
	}

	return &StagingStatus{
		AgentID:           lsa.agentID,
		Status:            "healthy",
		TotalPods:         len(lsa.stagingPods),
		RunningPods:       runningPods,
		FailedPods:        failedPods,
		StagingPods:       lsa.stagingPods,
		KindClusterStatus: clusterStatus,
		LastSync:          time.Now(),
		Timestamp:         time.Now(),
	}
}

// GetStagingPods returns all staging pods
func (lsa *LocalStagingAgent) GetStagingPods() map[string]StagingPodInfo {
	lsa.mutex.RLock()
	defer lsa.mutex.RUnlock()

	result := make(map[string]StagingPodInfo)
	for id, pod := range lsa.stagingPods {
		result[id] = pod
	}
	return result
}

// GetHTTPProxies returns all active HTTP proxies
func (lsa *LocalStagingAgent) GetHTTPProxies() map[string]HTTPProxy {
	if lsa.httpProxy == nil {
		return make(map[string]HTTPProxy)
	}
	return lsa.httpProxy.GetProxies()
}

// GetProxyStatus returns the status of all proxies
func (lsa *LocalStagingAgent) GetProxyStatus() map[string]interface{} {
	if lsa.httpProxy == nil {
		return map[string]interface{}{
			"total_proxies":  0,
			"active_proxies": 0,
			"failed_proxies": 0,
			"proxies":        make(map[string]HTTPProxy),
			"timestamp":      time.Now(),
		}
	}
	return lsa.httpProxy.GetProxyStatus()
}

// registerWithControlPlane automatically registers the agent with the control plane at startup
func (lsa *LocalStagingAgent) registerWithControlPlane() {
	lsa.logger.Info("Registering agent with control plane...")

	// Get the current Cloudflare tunnel URL
	tunnelURL := "https://tunnel-establishing.trycloudflare.com" // Default tunnel URL

	// Try to get the actual tunnel URL from the tunnel manager if available
	if lsa.cloudflareTunnel != nil {
		tunnels := lsa.cloudflareTunnel.GetTunnels()
		for _, tunnel := range tunnels {
			if tunnel.Status == "active" && tunnel.PublicURL != "" {
				tunnelURL = tunnel.PublicURL
				break
			}
		}
	}

	// Get current pod data for scheduling information
	lsa.mutex.RLock()
	podCount := len(lsa.stagingPods)
	pods := make([]StagingPodInfo, 0, len(lsa.stagingPods))
	for _, pod := range lsa.stagingPods {
		pods = append(pods, pod)
	}
	lsa.mutex.RUnlock()

	// Get kind cluster status
	clusterStatus := "unknown"
	if lsa.kindCluster != nil {
		if status, err := lsa.kindCluster.GetClusterStatus(); err == nil {
			clusterStatus = status
		}
	}

	// Create comprehensive registration payload
	registrationPayload := map[string]interface{}{
		"host": tunnelURL,
		"agent_info": map[string]interface{}{
			"agent_id":  lsa.agentID,
			"status":    "healthy",
			"timestamp": time.Now(),
		},
		"pod_scheduling": map[string]interface{}{
			"current_pods":   podCount,
			"available_pods": pods,
			"capabilities": map[string]interface{}{
				"staging_pods":      true,
				"kind_cluster":      true,
				"http_proxy":        true,
				"cloudflare_tunnel": true,
				"auto_scaling":      true,
			},
			"resources": map[string]interface{}{
				"cpu_available":     "4 cores",
				"memory_available":  "8GB",
				"storage_available": "100GB",
				"network_ports":     []int{8080, 8082, 30000, 32767},
			},
			"staging_config": map[string]interface{}{
				"namespace":      "staging",
				"cluster_name":   "kind-staging",
				"sync_interval":  "30s",
				"auto_scale":     true,
				"pod_scheduling": true,
			},
		},
		"endpoints": map[string]string{
			"health":         "/health",
			"pod_status":     "/api/v1/pods/status",
			"register_agent": "/api/v1/register-local-agent",
			"pod_update":     "/api/v1/pods",
		},
		"cluster_status": clusterStatus,
	}

	// Send registration request
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	url := fmt.Sprintf("%s/api/v1/register-local-agent", lsa.controlPlaneURL)
	jsonData, err := json.Marshal(registrationPayload)
	if err != nil {
		lsa.logger.Error("Failed to marshal registration payload", "error", err)
		return
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		lsa.logger.Error("Failed to create registration request", "error", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-ID", lsa.agentID)

	resp, err := client.Do(req)
	if err != nil {
		lsa.logger.Error("Failed to register with control plane", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		lsa.logger.Error("Control plane registration failed", "status", resp.StatusCode)
		return
	}

	lsa.logger.Info("Successfully registered with control plane",
		"tunnel_url", tunnelURL,
		"agent_id", lsa.agentID,
		"pod_count", podCount)
}

// createK8sClient creates a Kubernetes client
func createK8sClient() (*kubernetes.Clientset, error) {
	// Try to load in-cluster config first
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

	return clientset, nil
}
