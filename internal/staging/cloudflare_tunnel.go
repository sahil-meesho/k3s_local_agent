package staging

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"k3s-local-agent/pkg/logger"
)

// CloudflareTunnelManager handles Cloudflare tunneling for staging pods
type CloudflareTunnelManager struct {
	logger    logger.Logger
	tunnels   map[string]CloudflareTunnel
	mutex     sync.RWMutex
	agentID   string
	hostname  string
	localPort int
	protocol  string
	autoStart bool
}

// CloudflareTunnel represents a Cloudflare tunnel configuration
type CloudflareTunnel struct {
	TunnelID  string    `json:"tunnel_id"`
	Hostname  string    `json:"hostname"`
	LocalPort int       `json:"local_port"`
	Protocol  string    `json:"protocol"`
	Status    string    `json:"status"` // "active", "failed", "pending"
	PublicURL string    `json:"public_url"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TunnelConfig holds configuration for Cloudflare tunnel
type TunnelConfig struct {
	AgentID   string
	Hostname  string
	LocalPort int
	Protocol  string
	AutoStart bool
}

// NewCloudflareTunnelManager creates a new Cloudflare tunnel manager
func NewCloudflareTunnelManager(config *TunnelConfig, log logger.Logger) *CloudflareTunnelManager {
	return &CloudflareTunnelManager{
		logger:    log,
		tunnels:   make(map[string]CloudflareTunnel),
		agentID:   config.AgentID,
		hostname:  config.Hostname,
		localPort: config.LocalPort,
		protocol:  config.Protocol,
		autoStart: config.AutoStart,
	}
}

// SetupTunnel creates a Cloudflare tunnel
func (ctm *CloudflareTunnelManager) SetupTunnel() (*CloudflareTunnel, error) {
	ctm.mutex.Lock()
	defer ctm.mutex.Unlock()

	// Check if tunnel already exists
	tunnelKey := fmt.Sprintf("%s-%d", ctm.hostname, ctm.localPort)
	if existing, exists := ctm.tunnels[tunnelKey]; exists {
		ctm.logger.Info("Cloudflare tunnel already exists",
			"hostname", existing.Hostname,
			"tunnel_id", existing.TunnelID,
			"public_url", existing.PublicURL)
		return &existing, nil
	}

	// Create tunnel ID
	tunnelID := fmt.Sprintf("cloudflared-tunnel-%d-%s", ctm.localPort, time.Now().Format("20060102"))

	// Create tunnel
	tunnel := &CloudflareTunnel{
		TunnelID:  tunnelID,
		Hostname:  ctm.hostname,
		LocalPort: ctm.localPort,
		Protocol:  ctm.protocol,
		Status:    "pending",
		PublicURL: fmt.Sprintf("https://%s", ctm.hostname),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Start tunnel if auto-start is enabled
	if ctm.autoStart {
		if err := ctm.startTunnel(tunnel); err != nil {
			tunnel.Status = "failed"
			ctm.logger.Error("Failed to start Cloudflare tunnel",
				"hostname", tunnel.Hostname,
				"error", err)
		} else {
			tunnel.Status = "active"
		}
	}

	// Store tunnel
	ctm.tunnels[tunnelKey] = *tunnel

	ctm.logger.Info("Cloudflare tunnel setup completed",
		"hostname", tunnel.Hostname,
		"tunnel_id", tunnel.TunnelID,
		"public_url", tunnel.PublicURL,
		"status", tunnel.Status)

	return tunnel, nil
}

// startTunnel starts a Cloudflare tunnel using cloudflared
func (ctm *CloudflareTunnelManager) startTunnel(tunnel *CloudflareTunnel) error {
	// Build cloudflared command - use trycloudflare.com for random hostname
	cmd := exec.Command("cloudflared", "tunnel",
		"--url", fmt.Sprintf("http://localhost:%d", tunnel.LocalPort))

	// Capture output to get the tunnel URL
	var output strings.Builder
	cmd.Stdout = &output
	cmd.Stderr = &output

	// Start the tunnel in background
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start cloudflared tunnel: %w", err)
	}

	ctm.logger.Info("Cloudflare tunnel started",
		"local_port", tunnel.LocalPort,
		"pid", cmd.Process.Pid)

	// Wait a moment for the tunnel to establish and get the URL
	time.Sleep(5 * time.Second)

	// Try to extract the tunnel URL from the output
	outputStr := output.String()
	if strings.Contains(outputStr, "https://") {
		// Look for the tunnel URL in the output
		lines := strings.Split(outputStr, "\n")
		for _, line := range lines {
			if strings.Contains(line, "https://") && strings.Contains(line, ".trycloudflare.com") {
				// Extract the URL using regex-like approach
				start := strings.Index(line, "https://")
				if start != -1 {
					// Find the end of the URL (space, newline, or end of line)
					end := len(line)
					for i := start; i < len(line); i++ {
						if line[i] == ' ' || line[i] == '\n' || line[i] == '\r' {
							end = i
							break
						}
					}
					tunnel.PublicURL = strings.TrimSpace(line[start:end])
					break
				}
			}
		}
	}

	// If we couldn't extract the URL, use a placeholder
	if tunnel.PublicURL == "" {
		tunnel.PublicURL = "https://tunnel-establishing.trycloudflare.com"
	}

	ctm.logger.Info("Tunnel URL extracted", "public_url", tunnel.PublicURL)

	return nil
}

// RemoveTunnel removes a Cloudflare tunnel
func (ctm *CloudflareTunnelManager) RemoveTunnel(hostname string, localPort int) error {
	ctm.mutex.Lock()
	defer ctm.mutex.Unlock()

	tunnelKey := fmt.Sprintf("%s-%d", hostname, localPort)
	tunnel, exists := ctm.tunnels[tunnelKey]
	if !exists {
		return fmt.Errorf("tunnel not found for %s:%d", hostname, localPort)
	}

	// Kill cloudflared process if running
	if tunnel.Status == "active" {
		// Note: In a real implementation, you'd track the process ID
		// and kill it properly. For now, we'll just remove from map.
		ctm.logger.Info("Removing Cloudflare tunnel",
			"hostname", tunnel.Hostname,
			"tunnel_id", tunnel.TunnelID)
	}

	// Remove from tunnels map
	delete(ctm.tunnels, tunnelKey)

	return nil
}

// GetTunnels returns all active tunnels
func (ctm *CloudflareTunnelManager) GetTunnels() map[string]CloudflareTunnel {
	ctm.mutex.RLock()
	defer ctm.mutex.RUnlock()

	result := make(map[string]CloudflareTunnel)
	for k, v := range ctm.tunnels {
		result[k] = v
	}
	return result
}

// GetTunnelStatus returns the status of all tunnels
func (ctm *CloudflareTunnelManager) GetTunnelStatus() map[string]interface{} {
	ctm.mutex.RLock()
	defer ctm.mutex.RUnlock()

	status := map[string]interface{}{
		"total_tunnels":  len(ctm.tunnels),
		"active_tunnels": 0,
		"failed_tunnels": 0,
		"tunnels":        ctm.tunnels,
		"timestamp":      time.Now(),
	}

	for _, tunnel := range ctm.tunnels {
		if tunnel.Status == "active" {
			status["active_tunnels"] = status["active_tunnels"].(int) + 1
		} else if tunnel.Status == "failed" {
			status["failed_tunnels"] = status["failed_tunnels"].(int) + 1
		}
	}

	return status
}

// HealthCheck performs a health check on all tunnels
func (ctm *CloudflareTunnelManager) HealthCheck() map[string]interface{} {
	status := ctm.GetTunnelStatus()

	// Check each tunnel's health
	for tunnelKey, tunnel := range ctm.tunnels {
		isHealthy := ctm.isTunnelHealthy(&tunnel)
		status["tunnels"].(map[string]CloudflareTunnel)[tunnelKey] = tunnel

		if !isHealthy && tunnel.Status == "active" {
			ctm.logger.Warn("Tunnel health check failed",
				"tunnel_id", tunnel.TunnelID,
				"hostname", tunnel.Hostname)
		}
	}

	return status
}

// isTunnelHealthy checks if a tunnel is healthy
func (ctm *CloudflareTunnelManager) isTunnelHealthy(tunnel *CloudflareTunnel) bool {
	// Test the tunnel endpoint
	resp, err := exec.Command("curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", tunnel.PublicURL).Output()
	if err != nil {
		return false
	}

	// Check if response is 200
	return string(resp) == "200"
}

// StartAllTunnels starts all configured tunnels
func (ctm *CloudflareTunnelManager) StartAllTunnels() error {
	ctm.logger.Info("Starting all Cloudflare tunnels...")

	// Setup the main tunnel
	tunnel, err := ctm.SetupTunnel()
	if err != nil {
		return fmt.Errorf("failed to setup main tunnel: %w", err)
	}

	ctm.logger.Info("All Cloudflare tunnels started",
		"active_tunnels", len(ctm.tunnels),
		"main_tunnel_url", tunnel.PublicURL)

	return nil
}
