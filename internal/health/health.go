package health

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

	"k3s-local-agent/pkg/logger"
)

type HealthMonitor interface {
	GetOverallHealth() (*HealthData, error)
	CheckVPNStatus() (*VPNStatus, error)
}

type healthMonitor struct {
	logger logger.Logger
}

type HealthData struct {
	IsHealthy   bool       `json:"is_healthy"`
	IsOnline    bool       `json:"is_online"`
	HasInternet bool       `json:"has_internet"`
	VPNStatus   *VPNStatus `json:"vpn_status,omitempty"`
	Timestamp   time.Time  `json:"timestamp"`
}

type VPNStatus struct {
	IsConnected bool      `json:"is_connected"`
	VPNType     string    `json:"vpn_type"`
	IPAddress   string    `json:"ip_address"`
	Interface   string    `json:"interface"`
	Timestamp   time.Time `json:"timestamp"`
}

func New(log logger.Logger) HealthMonitor {
	return &healthMonitor{
		logger: log,
	}
}

func (h *healthMonitor) GetOverallHealth() (*HealthData, error) {
	healthData := &HealthData{
		IsHealthy:   true,
		IsOnline:    false,
		HasInternet: false,
		Timestamp:   time.Now(),
	}

	// Check network connectivity
	interfaces, err := net.Interfaces()
	if err != nil {
		healthData.IsHealthy = false
		return healthData, nil
	}

	// Check for active interfaces
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 {
			healthData.IsOnline = true
			break
		}
	}

	// Check internet connectivity
	if healthData.IsOnline {
		conn, err := net.DialTimeout("tcp", "8.8.8.8:53", 5*time.Second)
		if err == nil {
			conn.Close()
			healthData.HasInternet = true
		}
	}

	// Get VPN status
	vpnStatus, err := h.CheckVPNStatus()
	if err != nil {
		h.logger.Debug("Failed to check VPN status", err)
	} else {
		healthData.VPNStatus = vpnStatus
	}

	return healthData, nil
}

func (h *healthMonitor) CheckVPNStatus() (*VPNStatus, error) {
	vpnStatus := &VPNStatus{
		IsConnected: false,
		Timestamp:   time.Now(),
	}

	// Check for VPN interfaces
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to get network interfaces: %w", err)
	}

	// Check for VPN interfaces (utun, tun, ppp)
	for _, iface := range interfaces {
		// Check if it's a VPN interface
		isVPNInterface := strings.HasPrefix(iface.Name, "utun") ||
			strings.HasPrefix(iface.Name, "tun") ||
			strings.HasPrefix(iface.Name, "ppp") ||
			strings.HasPrefix(iface.Name, "vpn")

		if (iface.Flags&net.FlagUp != 0) &&
			(iface.Flags&net.FlagLoopback == 0) &&
			isVPNInterface {

			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}

			for _, addr := range addrs {
				if ipnet, ok := addr.(*net.IPNet); ok {
					if ipnet.IP.To4() != nil {
						vpnStatus.IsConnected = true
						vpnStatus.IPAddress = ipnet.IP.String()
						vpnStatus.Interface = iface.Name

						// Determine VPN type based on interface name
						if strings.HasPrefix(iface.Name, "utun") {
							vpnStatus.VPNType = "GlobalProtect"
						} else if strings.HasPrefix(iface.Name, "tun") {
							vpnStatus.VPNType = "OpenVPN"
						} else if strings.HasPrefix(iface.Name, "ppp") {
							vpnStatus.VPNType = "PPTP/L2TP"
						} else {
							vpnStatus.VPNType = "Unknown"
						}

						return vpnStatus, nil
					}
				}
			}
		}
	}

	// Additional check: Look for specific VPN IP ranges
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 {
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}

			for _, addr := range addrs {
				if ipnet, ok := addr.(*net.IPNet); ok {
					if ipnet.IP.To4() != nil {
						// Check if IP is in common VPN ranges
						ip := ipnet.IP.To4()
						if (ip[0] == 10 && ip[1] == 255) || // 10.255.x.x range
							(ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31) || // 172.16-31.x.x range
							(ip[0] == 192 && ip[1] == 168) { // 192.168.x.x range
							vpnStatus.IsConnected = true
							vpnStatus.IPAddress = ipnet.IP.String()
							vpnStatus.Interface = iface.Name
							vpnStatus.VPNType = "VPN (IP Range Detection)"
							return vpnStatus, nil
						}
					}
				}
			}
		}
	}

	// Check for GlobalProtect specifically on macOS
	if h.checkGlobalProtectStatus() {
		vpnStatus.IsConnected = true
		vpnStatus.VPNType = "GlobalProtect"
		vpnStatus.IPAddress = "Connected"
		return vpnStatus, nil
	}

	return vpnStatus, nil
}

func (h *healthMonitor) checkGlobalProtectStatus() bool {
	// Check if GlobalProtect is running on macOS
	cmd := exec.Command("pgrep", "-f", "GlobalProtect")
	if err := cmd.Run(); err != nil {
		return false
	}

	// Check GlobalProtect connection status
	cmd = exec.Command("defaults", "read", "/Library/Preferences/com.paloaltonetworks.GlobalProtect.settings", "Palo", "Alto", "Networks", "GlobalProtect", "PanSetup", "Portal")
	if err := cmd.Run(); err == nil {
		return true
	}

	return false
}
