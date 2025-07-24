package controlplane

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"k3s-local-agent/pkg/logger"
)

type PodReceiver struct {
	server  *http.Server
	logger  logger.Logger
	podData map[string]PodInfo
	mutex   sync.RWMutex
	port    int
	agentID string
}

type PodInfo struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Image       string            `json:"image"`
	Status      string            `json:"status"`
	CPUUsage    string            `json:"cpu_usage"`
	MemoryUsage string            `json:"memory_usage"`
	IP          string            `json:"ip"`
	NodeName    string            `json:"node_name"`
	Labels      map[string]string `json:"labels"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type PodUpdateRequest struct {
	AgentID string    `json:"agent_id"`
	Pods    []PodInfo `json:"pods"`
	Action  string    `json:"action"` // "update", "delete", "create"
}

type PodUpdateResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Count   int    `json:"count"`
}

func NewPodReceiver(port int, agentID string, log logger.Logger) *PodReceiver {
	return &PodReceiver{
		port:    port,
		agentID: agentID,
		logger:  log,
		podData: make(map[string]PodInfo),
	}
}

// Start starts the HTTP server to receive pod data from control plane
func (pr *PodReceiver) Start() error {
	mux := http.NewServeMux()

	// Endpoint to receive pod updates from control plane
	mux.HandleFunc("/api/v1/pods", pr.handlePodUpdate)

	// Endpoint to get current pod data
	mux.HandleFunc("/api/v1/pods/status", pr.handleGetPodStatus)

	// Agent registration endpoint
	mux.HandleFunc("/register-local-agent", pr.handleRegisterLocalAgent)

	// Health check endpoint
	mux.HandleFunc("/health", pr.handleHealth)

	pr.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", pr.port),
		Handler: mux,
	}

	pr.logger.Info("Starting pod receiver server", "port", pr.port, "agent_id", pr.agentID)

	go func() {
		if err := pr.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			pr.logger.Error("Pod receiver server error", "error", err)
		}
	}()

	return nil
}

// Stop stops the HTTP server
func (pr *PodReceiver) Stop() error {
	if pr.server != nil {
		return pr.server.Close()
	}
	return nil
}

// handlePodUpdate handles pod updates from control plane
func (pr *PodReceiver) handlePodUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request PodUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Verify agent ID
	if request.AgentID != pr.agentID {
		http.Error(w, "Invalid agent ID", http.StatusUnauthorized)
		return
	}

	pr.mutex.Lock()
	defer pr.mutex.Unlock()

	var count int
	switch request.Action {
	case "update", "create":
		for _, pod := range request.Pods {
			pod.UpdatedAt = time.Now()
			if pod.CreatedAt.IsZero() {
				pod.CreatedAt = time.Now()
			}
			pr.podData[pod.ID] = pod
			count++
		}
	case "delete":
		for _, pod := range request.Pods {
			delete(pr.podData, pod.ID)
			count++
		}
	}

	response := PodUpdateResponse{
		Success: true,
		Message: fmt.Sprintf("Successfully processed %d pods", count),
		Count:   count,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)

	pr.logger.Info("Pod data updated from control plane",
		"action", request.Action,
		"count", count,
		"total_pods", len(pr.podData))
}

// handleGetPodStatus returns current pod status
func (pr *PodReceiver) handleGetPodStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pr.mutex.RLock()
	defer pr.mutex.RUnlock()

	pods := make([]PodInfo, 0, len(pr.podData))
	for _, pod := range pr.podData {
		pods = append(pods, pod)
	}

	response := map[string]interface{}{
		"agent_id":  pr.agentID,
		"pods":      pods,
		"count":     len(pods),
		"timestamp": time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleHealth handles health check requests
func (pr *PodReceiver) handleHealth(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":    "healthy",
		"agent_id":  pr.agentID,
		"timestamp": time.Now(),
		"pod_count": len(pr.podData),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleRegisterLocalAgent handles agent registration requests
func (pr *PodReceiver) handleRegisterLocalAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse the request body
	var requestBody map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
		pr.logger.Error("Failed to decode request body", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Extract host from request
	host, ok := requestBody["host"].(string)
	if !ok {
		pr.logger.Error("Missing or invalid host in request")
		http.Error(w, "Missing or invalid host", http.StatusBadRequest)
		return
	}

	pr.logger.Info("Agent registration request received", "host", host)

	// Get current pod data for scheduling information
	pr.mutex.RLock()
	podCount := len(pr.podData)
	pods := make([]PodInfo, 0, len(pr.podData))
	for _, pod := range pr.podData {
		pods = append(pods, pod)
	}
	pr.mutex.RUnlock()

	// Create comprehensive registration response with pod scheduling capabilities
	response := map[string]interface{}{
		"status":    "success",
		"message":   "Agent registered successfully with pod scheduling capabilities",
		"agent_id":  pr.agentID,
		"host":      host,
		"timestamp": time.Now(),
		"endpoints": map[string]string{
			"health":           "/health",
			"pod_status":       "/api/v1/pods/status",
			"register_agent":   "/register-local-agent",
			"pod_update":       "/api/v1/pods",
		},
		"pod_scheduling": map[string]interface{}{
			"current_pods": podCount,
			"available_pods": pods,
			"capabilities": map[string]interface{}{
				"staging_pods": true,
				"kind_cluster": true,
				"http_proxy":   true,
				"cloudflare_tunnel": true,
				"auto_scaling": true,
			},
			"resources": map[string]interface{}{
				"cpu_available": "4 cores",
				"memory_available": "8GB",
				"storage_available": "100GB",
				"network_ports": []int{8080, 8082, 30000, 32767},
			},
			"staging_config": map[string]interface{}{
				"namespace": "staging",
				"cluster_name": "kind-staging",
				"sync_interval": "30s",
				"auto_scale": true,
				"pod_scheduling": true,
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// GetPodData returns current pod data
func (pr *PodReceiver) GetPodData() []PodInfo {
	pr.mutex.RLock()
	defer pr.mutex.RUnlock()

	pods := make([]PodInfo, 0, len(pr.podData))
	for _, pod := range pr.podData {
		pods = append(pods, pod)
	}
	return pods
}

// GetPodCount returns the number of pods
func (pr *PodReceiver) GetPodCount() int {
	pr.mutex.RLock()
	defer pr.mutex.RUnlock()
	return len(pr.podData)
}

// GetPodByID returns a specific pod by ID
func (pr *PodReceiver) GetPodByID(id string) (PodInfo, bool) {
	pr.mutex.RLock()
	defer pr.mutex.RUnlock()

	pod, exists := pr.podData[id]
	return pod, exists
}
