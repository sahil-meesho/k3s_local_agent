package staging

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"

	"k3s-local-agent/pkg/logger"
)

// IPRedirectionManager handles pod IP redirection from GCS staging to local
type IPRedirectionManager struct {
	logger       logger.Logger
	redirections map[string]PodRedirection
	mutex        sync.RWMutex
	agentID      string
}

// PodRedirection represents an IP redirection mapping
type PodRedirection struct {
	StagingPodID   string    `json:"staging_pod_id"`
	StagingPodName string    `json:"staging_pod_name"`
	StagingPodIP   string    `json:"staging_pod_ip"` // Original IP from GCS staging
	LocalPodName   string    `json:"local_pod_name"`
	LocalPodIP     string    `json:"local_pod_ip"` // New IP in local kind cluster
	LocalPort      int       `json:"local_port"`   // Port forwarding port
	StagingPort    int       `json:"staging_port"` // Original port from staging
	Status         string    `json:"status"`       // "active", "failed", "pending"
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// RedirectionConfig holds configuration for IP redirection
type RedirectionConfig struct {
	AgentID           string
	LocalHost         string
	PortRangeStart    int
	PortRangeEnd      int
	EnablePortForward bool
	EnableDNSProxy    bool
}

func NewIPRedirectionManager(config *RedirectionConfig, log logger.Logger) *IPRedirectionManager {
	return &IPRedirectionManager{
		logger:       log,
		redirections: make(map[string]PodRedirection),
		agentID:      config.AgentID,
	}
}

// SetupRedirection creates IP redirection for a staging pod
func (irm *IPRedirectionManager) SetupRedirection(stagingPod StagingPodInfo, localPodIP string) (*PodRedirection, error) {
	irm.mutex.Lock()
	defer irm.mutex.Unlock()

	// Check if redirection already exists
	if existing, exists := irm.redirections[stagingPod.ID]; exists {
		irm.logger.Info("Redirection already exists",
			"pod", stagingPod.Name,
			"staging_ip", existing.StagingPodIP,
			"local_ip", existing.LocalPodIP)
		return &existing, nil
	}

	// Create new redirection
	redirection := PodRedirection{
		StagingPodID:   stagingPod.ID,
		StagingPodName: stagingPod.Name,
		StagingPodIP:   stagingPod.IP,
		LocalPodName:   stagingPod.Name,
		LocalPodIP:     localPodIP,
		StagingPort:    80, // Default port
		LocalPort:      0,  // Will be assigned
		Status:         "pending",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Assign local port
	if err := irm.assignLocalPort(&redirection); err != nil {
		return nil, fmt.Errorf("failed to assign local port: %w", err)
	}

	// Setup port forwarding
	if err := irm.setupPortForwarding(&redirection); err != nil {
		redirection.Status = "failed"
		irm.logger.Error("Failed to setup port forwarding",
			"pod", stagingPod.Name,
			"error", err)
	} else {
		redirection.Status = "active"
	}

	// Setup DNS redirection
	if err := irm.setupDNSRedirection(&redirection); err != nil {
		irm.logger.Warn("Failed to setup DNS redirection",
			"pod", stagingPod.Name,
			"error", err)
	}

	// Store redirection
	irm.redirections[stagingPod.ID] = redirection

	irm.logger.Info("IP redirection setup completed",
		"pod", stagingPod.Name,
		"staging_ip", redirection.StagingPodIP,
		"local_ip", redirection.LocalPodIP,
		"local_port", redirection.LocalPort,
		"status", redirection.Status)

	return &redirection, nil
}

// assignLocalPort assigns an available local port for redirection
func (irm *IPRedirectionManager) assignLocalPort(redirection *PodRedirection) error {
	// Start from port 8080 and find an available port
	for port := 8080; port <= 9000; port++ {
		if irm.isPortAvailable(port) {
			redirection.LocalPort = port
			return nil
		}
	}
	return fmt.Errorf("no available ports in range 8080-9000")
}

// isPortAvailable checks if a port is available
func (irm *IPRedirectionManager) isPortAvailable(port int) bool {
	// Check if port is already used by another redirection
	for _, redir := range irm.redirections {
		if redir.LocalPort == port {
			return false
		}
	}

	// Check if port is available on system
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// setupPortForwarding sets up port forwarding from local port to staging IP
func (irm *IPRedirectionManager) setupPortForwarding(redirection *PodRedirection) error {
	// Create port forwarding using socat or similar tool
	cmd := exec.Command("socat",
		"TCP-LISTEN:"+fmt.Sprintf("%d", redirection.LocalPort),
		fmt.Sprintf("TCP:%s:%d", redirection.StagingPodIP, redirection.StagingPort),
		"&")

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Fallback: try using netcat
		cmd = exec.Command("nc", "-l", fmt.Sprintf("%d", redirection.LocalPort))
		output, err = cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to setup port forwarding: %w, output: %s", err, string(output))
		}
	}

	irm.logger.Info("Port forwarding setup",
		"local_port", redirection.LocalPort,
		"staging_ip", redirection.StagingPodIP,
		"staging_port", redirection.StagingPort)

	return nil
}

// setupDNSRedirection sets up DNS redirection for the staging pod
func (irm *IPRedirectionManager) setupDNSRedirection(redirection *PodRedirection) error {
	// Add entry to /etc/hosts for local resolution
	hostsEntry := fmt.Sprintf("%s %s-staging.local", redirection.LocalPodIP, redirection.StagingPodName)

	cmd := exec.Command("sh", "-c", fmt.Sprintf("echo '%s' >> /tmp/staging_hosts", hostsEntry))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add hosts entry: %w", err)
	}

	irm.logger.Info("DNS redirection setup",
		"hostname", fmt.Sprintf("%s-staging.local", redirection.StagingPodName),
		"ip", redirection.LocalPodIP)

	return nil
}

// RemoveRedirection removes IP redirection for a pod
func (irm *IPRedirectionManager) RemoveRedirection(podID string) error {
	irm.mutex.Lock()
	defer irm.mutex.Unlock()

	redirection, exists := irm.redirections[podID]
	if !exists {
		return fmt.Errorf("redirection not found for pod %s", podID)
	}

	// Stop port forwarding
	if err := irm.stopPortForwarding(&redirection); err != nil {
		irm.logger.Error("Failed to stop port forwarding",
			"pod", redirection.StagingPodName,
			"error", err)
	}

	// Remove DNS redirection
	if err := irm.removeDNSRedirection(&redirection); err != nil {
		irm.logger.Error("Failed to remove DNS redirection",
			"pod", redirection.StagingPodName,
			"error", err)
	}

	// Remove from redirections map
	delete(irm.redirections, podID)

	irm.logger.Info("IP redirection removed",
		"pod", redirection.StagingPodName,
		"staging_ip", redirection.StagingPodIP,
		"local_ip", redirection.LocalPodIP)

	return nil
}

// stopPortForwarding stops port forwarding for a redirection
func (irm *IPRedirectionManager) stopPortForwarding(redirection *PodRedirection) error {
	// Kill processes using the local port
	cmd := exec.Command("lsof", "-ti", fmt.Sprintf(":%d", redirection.LocalPort))
	output, err := cmd.Output()
	if err == nil {
		pids := strings.TrimSpace(string(output))
		if pids != "" {
			killCmd := exec.Command("kill", "-9", pids)
			killCmd.Run()
		}
	}

	return nil
}

// removeDNSRedirection removes DNS redirection for a pod
func (irm *IPRedirectionManager) removeDNSRedirection(redirection *PodRedirection) error {
	// Remove entry from /tmp/staging_hosts
	cmd := exec.Command("sed", "-i", fmt.Sprintf("/%s/d", redirection.StagingPodName), "/tmp/staging_hosts")
	return cmd.Run()
}

// GetRedirections returns all active redirections
func (irm *IPRedirectionManager) GetRedirections() map[string]PodRedirection {
	irm.mutex.RLock()
	defer irm.mutex.RUnlock()

	result := make(map[string]PodRedirection)
	for id, redirection := range irm.redirections {
		result[id] = redirection
	}
	return result
}

// GetRedirectionByStagingIP returns redirection for a staging IP
func (irm *IPRedirectionManager) GetRedirectionByStagingIP(stagingIP string) (*PodRedirection, bool) {
	irm.mutex.RLock()
	defer irm.mutex.RUnlock()

	for _, redirection := range irm.redirections {
		if redirection.StagingPodIP == stagingIP {
			return &redirection, true
		}
	}
	return nil, false
}

// GetRedirectionByLocalIP returns redirection for a local IP
func (irm *IPRedirectionManager) GetRedirectionByLocalIP(localIP string) (*PodRedirection, bool) {
	irm.mutex.RLock()
	defer irm.mutex.RUnlock()

	for _, redirection := range irm.redirections {
		if redirection.LocalPodIP == localIP {
			return &redirection, true
		}
	}
	return nil, false
}

// GetRedirectionStatus returns the status of all redirections
func (irm *IPRedirectionManager) GetRedirectionStatus() map[string]interface{} {
	irm.mutex.RLock()
	defer irm.mutex.RUnlock()

	status := map[string]interface{}{
		"total_redirections":  len(irm.redirections),
		"active_redirections": 0,
		"failed_redirections": 0,
		"redirections":        irm.redirections,
		"timestamp":           time.Now(),
	}

	for _, redirection := range irm.redirections {
		switch redirection.Status {
		case "active":
			status["active_redirections"] = status["active_redirections"].(int) + 1
		case "failed":
			status["failed_redirections"] = status["failed_redirections"].(int) + 1
		}
	}

	return status
}
