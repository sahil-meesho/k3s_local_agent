package controlplane

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"k3s-local-agent/internal/monitor"
	"k3s-local-agent/pkg/logger"
)

type ControlPlaneClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
	logger  logger.Logger
	agentID string
}

type ControlPlaneConfig struct {
	BaseURL string
	APIKey  string
	AgentID string
	Timeout time.Duration
}

type MonitoringData struct {
	AgentID     string                   `json:"agent_id"`
	Timestamp   time.Time                `json:"timestamp"`
	LocalSystem *monitor.ResourceData    `json:"local_system"`
	ClusterData *monitor.K3sResourceData `json:"cluster_data"`
	AgentStatus string                   `json:"agent_status"`
}

type ControlPlaneResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func NewControlPlaneClient(config *ControlPlaneConfig, log logger.Logger) *ControlPlaneClient {
	timeout := config.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &ControlPlaneClient{
		baseURL: config.BaseURL,
		apiKey:  config.APIKey,
		client: &http.Client{
			Timeout: timeout,
		},
		logger:  log,
		agentID: config.AgentID,
	}
}

// SendMonitoringData sends monitoring data to the control plane
func (c *ControlPlaneClient) SendMonitoringData(k3sData *monitor.K3sResourceData) error {
	data := &MonitoringData{
		AgentID:     c.agentID,
		Timestamp:   time.Now(),
		LocalSystem: k3sData.LocalSystem,
		ClusterData: k3sData,
		AgentStatus: "healthy",
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal monitoring data: %w", err)
	}

	// Send to control plane
	url := fmt.Sprintf("%s/api/v1/monitoring", c.baseURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	req.Header.Set("X-Agent-ID", c.agentID)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("control plane returned status: %d", resp.StatusCode)
	}

	var response ControlPlaneResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if !response.Success {
		return fmt.Errorf("control plane error: %s", response.Message)
	}

	c.logger.Info("Successfully sent monitoring data to control plane",
		"agent_id", c.agentID,
		"timestamp", data.Timestamp)

	return nil
}

// SendHealthCheck sends a simple health check to the control plane
func (c *ControlPlaneClient) SendHealthCheck() error {
	data := map[string]interface{}{
		"agent_id":  c.agentID,
		"timestamp": time.Now(),
		"status":    "healthy",
		"version":   "1.0.0",
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal health check data: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/health", c.baseURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	req.Header.Set("X-Agent-ID", c.agentID)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send health check: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed with status: %d", resp.StatusCode)
	}

	c.logger.Info("Health check sent successfully", "agent_id", c.agentID)
	return nil
}

// SendSchedulingDecision sends pod scheduling decisions to the control plane
func (c *ControlPlaneClient) SendSchedulingDecision(decision interface{}) error {
	data := map[string]interface{}{
		"agent_id":  c.agentID,
		"timestamp": time.Now(),
		"decision":  decision,
		"type":      "scheduling_decision",
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal scheduling decision: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/scheduling", c.baseURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create scheduling request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	req.Header.Set("X-Agent-ID", c.agentID)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send scheduling decision: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("scheduling decision failed with status: %d", resp.StatusCode)
	}

	c.logger.Info("Scheduling decision sent successfully", "agent_id", c.agentID)
	return nil
}

// TestConnection tests the connection to the control plane
func (c *ControlPlaneClient) TestConnection() error {
	url := fmt.Sprintf("%s/api/v1/ping", c.baseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create ping request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	req.Header.Set("X-Agent-ID", c.agentID)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to ping control plane: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ping failed with status: %d", resp.StatusCode)
	}

	c.logger.Info("Control plane connection test successful", "agent_id", c.agentID)
	return nil
}
