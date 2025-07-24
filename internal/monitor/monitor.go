package monitor

import (
	"fmt"
	"net"
	"runtime"
	"strings"
	"time"

	"k3s-local-agent/internal/config"
	"k3s-local-agent/pkg/logger"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
)

type ResourceMonitor interface {
	GetSystemInfo() (*SystemInfo, error)
	GetCPUInfo() (*CPUInfo, error)
	GetMemoryInfo() (*MemoryInfo, error)
	GetVPNInfo() (*VPNInfo, error)
	GetHealthInfo() (*HealthInfo, error)
	GetAllResources() (*ResourceData, error)
}

type monitor struct {
	config *config.Config
	logger logger.Logger
}

type SystemInfo struct {
	Hostname     string    `json:"hostname"`
	Platform     string    `json:"platform"`
	OS           string    `json:"os"`
	Architecture string    `json:"architecture"`
	Uptime       uint64    `json:"uptime"`
	BootTime     time.Time `json:"boot_time"`
}

type CPUInfo struct {
	UsagePercent float64   `json:"usage_percent"`
	CoreCount    int       `json:"core_count"`
	ModelName    string    `json:"model_name"`
	Timestamp    time.Time `json:"timestamp"`
}

type MemoryInfo struct {
	Total       uint64    `json:"total"`
	Available   uint64    `json:"available"`
	Used        uint64    `json:"used"`
	Free        uint64    `json:"free"`
	UsedPercent float64   `json:"used_percent"`
	Timestamp   time.Time `json:"timestamp"`
}

type VPNInfo struct {
	IsConnected bool      `json:"is_connected"`
	IPAddress   string    `json:"ip_address"`
	Interface   string    `json:"interface"`
	Timestamp   time.Time `json:"timestamp"`
}

type HealthInfo struct {
	IsHealthy   bool      `json:"is_healthy"`
	IsOnline    bool      `json:"is_online"`
	HasInternet bool      `json:"has_internet"`
	Timestamp   time.Time `json:"timestamp"`
}

type ResourceData struct {
	System    *SystemInfo `json:"system"`
	CPU       *CPUInfo    `json:"cpu"`
	Memory    *MemoryInfo `json:"memory"`
	VPN       *VPNInfo    `json:"vpn"`
	Health    *HealthInfo `json:"health"`
	Timestamp time.Time   `json:"timestamp"`
}

func New(cfg *config.Config, log logger.Logger) ResourceMonitor {
	return &monitor{
		config: cfg,
		logger: log,
	}
}

// Get system information
func (m *monitor) GetSystemInfo() (*SystemInfo, error) {
	hostInfo, err := host.Info()
	if err != nil {
		return nil, fmt.Errorf("failed to get host info: %w", err)
	}

	return &SystemInfo{
		Hostname:     hostInfo.Hostname,
		Platform:     hostInfo.Platform,
		OS:           hostInfo.OS,
		Architecture: hostInfo.KernelArch,
		Uptime:       hostInfo.Uptime,
		BootTime:     time.Unix(int64(hostInfo.BootTime), 0),
	}, nil
}

// Get CPU information
func (m *monitor) GetCPUInfo() (*CPUInfo, error) {
	usage, err := cpu.Percent(0, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get CPU usage: %w", err)
	}

	info, err := cpu.Info()
	if err != nil {
		return nil, fmt.Errorf("failed to get CPU info: %w", err)
	}

	var modelName string
	if len(info) > 0 {
		modelName = info[0].ModelName
	}

	return &CPUInfo{
		UsagePercent: usage[0],
		CoreCount:    runtime.NumCPU(),
		ModelName:    modelName,
		Timestamp:    time.Now(),
	}, nil
}

// Get memory information
func (m *monitor) GetMemoryInfo() (*MemoryInfo, error) {
	memInfo, err := mem.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("failed to get memory info: %w", err)
	}

	return &MemoryInfo{
		Total:       memInfo.Total,
		Available:   memInfo.Available,
		Used:        memInfo.Used,
		Free:        memInfo.Free,
		UsedPercent: memInfo.UsedPercent,
		Timestamp:   time.Now(),
	}, nil
}

// Get VPN information using unified detection method
func (m *monitor) GetVPNInfo() (*VPNInfo, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to get network interfaces: %w", err)
	}

	vpnInfo := &VPNInfo{
		IsConnected: false,
		Timestamp:   time.Now(),
	}

	// Check for VPN interfaces and IP ranges
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
						vpnInfo.IsConnected = true
						vpnInfo.IPAddress = ipnet.IP.String()
						vpnInfo.Interface = iface.Name
						return vpnInfo, nil
					}
				}
			}
		}
	}

	// Check for VPN IP ranges on any interface
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
							vpnInfo.IsConnected = true
							vpnInfo.IPAddress = ipnet.IP.String()
							vpnInfo.Interface = iface.Name
							return vpnInfo, nil
						}
					}
				}
			}
		}
	}

	return vpnInfo, nil
}

// Get health information
func (m *monitor) GetHealthInfo() (*HealthInfo, error) {
	healthInfo := &HealthInfo{
		IsHealthy:   true,
		IsOnline:    false,
		HasInternet: false,
		Timestamp:   time.Now(),
	}

	// Check if we have network interfaces
	interfaces, err := net.Interfaces()
	if err != nil {
		healthInfo.IsHealthy = false
		return healthInfo, nil
	}

	// Check for active interfaces
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 {
			healthInfo.IsOnline = true
			break
		}
	}

	// Check internet connectivity
	if healthInfo.IsOnline {
		conn, err := net.DialTimeout("tcp", "8.8.8.8:53", 5*time.Second)
		if err == nil {
			conn.Close()
			healthInfo.HasInternet = true
		}
	}

	return healthInfo, nil
}

// Get all resources
func (m *monitor) GetAllResources() (*ResourceData, error) {
	system, err := m.GetSystemInfo()
	if err != nil {
		return nil, err
	}

	cpu, err := m.GetCPUInfo()
	if err != nil {
		return nil, err
	}

	memory, err := m.GetMemoryInfo()
	if err != nil {
		return nil, err
	}

	vpn, err := m.GetVPNInfo()
	if err != nil {
		return nil, err
	}

	health, err := m.GetHealthInfo()
	if err != nil {
		return nil, err
	}

	return &ResourceData{
		System:    system,
		CPU:       cpu,
		Memory:    memory,
		VPN:       vpn,
		Health:    health,
		Timestamp: time.Now(),
	}, nil
}
