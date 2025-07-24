package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"k3s-local-agent/internal/agent"
	"k3s-local-agent/internal/config"
	"k3s-local-agent/internal/monitor"
	"k3s-local-agent/pkg/logger"
)

// UnifiedConfig holds configuration for the unified tool
type UnifiedConfig struct {
	OutputFile       string
	LogFile          string
	WaitForAgent     time.Duration
	CaptureHealth    bool
	CaptureResources bool
	PrettyPrint      bool
	MonitorMode      bool
	CheckInterval    time.Duration
}

func main() {
	// Parse command line flags
	var (
		outputFile    = flag.String("output", "", "Output file path (default: reports/unified_YYYYMMDD_HHMMSS.txt)")
		logFile       = flag.String("log", "", "Log file path (default: logs/unified.log)")
		waitTime      = flag.Duration("wait", 5*time.Second, "Time to wait for agent to start")
		healthOnly    = flag.Bool("health-only", false, "Capture only health information")
		resourcesOnly = flag.Bool("resources-only", false, "Capture only resource information")
		prettyPrint   = flag.Bool("pretty", false, "Pretty print JSON output")
		monitorMode   = flag.Bool("monitor", false, "Run in monitoring mode")
		checkInterval = flag.Duration("interval", 30*time.Second, "Check interval for monitoring mode")
		help          = flag.Bool("help", false, "Show help information")
	)
	flag.Parse()

	if *help {
		printHelp()
		return
	}

	// Setup configuration
	cfg := setupUnifiedConfig(*outputFile, *logFile, *waitTime, *healthOnly, *resourcesOnly, *prettyPrint, *monitorMode, *checkInterval)

	// Setup logger
	log := logger.New()
	log.Info("Starting unified local agent tool...")

	// Create output directory
	if err := os.MkdirAll(filepath.Dir(cfg.OutputFile), 0755); err != nil {
		log.Fatal("Failed to create output directory", err)
	}

	// Load application config
	appConfig, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load configuration", err)
	}

	// Create monitor
	resourceMonitor := monitor.New(appConfig, log)

	if cfg.MonitorMode {
		// Run in monitoring mode
		runMonitoringMode(cfg, log, resourceMonitor)
	} else {
		// Run in capture mode
		runCaptureMode(cfg, log, resourceMonitor)
	}
}

// Setup unified configuration
func setupUnifiedConfig(outputFile, logFile string, waitTime time.Duration, healthOnly, resourcesOnly, prettyPrint, monitorMode bool, checkInterval time.Duration) *UnifiedConfig {
	// Generate default output file name if not provided
	if outputFile == "" {
		timestamp := time.Now().Format("20060102_150405")
		if monitorMode {
			outputFile = fmt.Sprintf("reports/monitor_%s.txt", timestamp)
		} else {
			outputFile = fmt.Sprintf("reports/unified_%s.txt", timestamp)
		}
	}

	// Generate default log file name if not provided
	if logFile == "" {
		if monitorMode {
			logFile = "logs/monitor.log"
		} else {
			logFile = "logs/unified.log"
		}
	}

	return &UnifiedConfig{
		OutputFile:       outputFile,
		LogFile:          logFile,
		WaitForAgent:     waitTime,
		CaptureHealth:    !resourcesOnly,
		CaptureResources: !healthOnly,
		PrettyPrint:      prettyPrint,
		MonitorMode:      monitorMode,
		CheckInterval:    checkInterval,
	}
}

// Run in capture mode - single capture with agent
func runCaptureMode(cfg *UnifiedConfig, log logger.Logger, resourceMonitor monitor.ResourceMonitor) {
	log.Info("Running in capture mode...")

	// Load application config
	appConfig, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load configuration", err)
	}

	// Start the agent
	agent := agent.New(appConfig, resourceMonitor, log)
	if err := agent.Start(); err != nil {
		log.Fatal("Failed to start agent", err)
	}

	// Wait for agent to be ready
	log.Info("Waiting for agent to be ready...")
	time.Sleep(cfg.WaitForAgent)

	// Capture data
	captureData(cfg, log, resourceMonitor)

	// Stop the agent
	if err := agent.Stop(); err != nil {
		log.Error("Failed to stop agent", err)
	}

	log.Info("Capture mode completed successfully")
}

// Run in monitoring mode - continuous monitoring
func runMonitoringMode(cfg *UnifiedConfig, log logger.Logger, resourceMonitor monitor.ResourceMonitor) {
	log.Info("Running in monitoring mode...")

	for {
		// Capture current state
		captureData(cfg, log, resourceMonitor)

		// Wait for next check
		log.Info("Waiting for next check...", "interval", cfg.CheckInterval)
		time.Sleep(cfg.CheckInterval)
	}
}

// Capture system data
func captureData(cfg *UnifiedConfig, log logger.Logger, resourceMonitor monitor.ResourceMonitor) {
	log.Info("Capturing system data...")

	// Create output file
	file, err := os.Create(cfg.OutputFile)
	if err != nil {
		log.Fatal("Failed to create output file", err)
	}
	defer file.Close()

	// Write header
	writeHeader(file, cfg)

	// Capture timestamp
	timestamp := time.Now()
	fmt.Fprintf(file, "TIMESTAMP: %s\n", timestamp.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(file, "===============================================\n\n")

	// Capture resources if requested
	if cfg.CaptureResources {
		log.Info("Capturing resource information...")
		writeResourceData(file, resourceMonitor, cfg.PrettyPrint)
	}

	// Capture health if requested
	if cfg.CaptureHealth {
		log.Info("Capturing health information...")
		writeHealthData(file, resourceMonitor, cfg.PrettyPrint)
	}

	// Write footer
	writeFooter(file, cfg)

	log.Info("Data captured successfully", "output_file", cfg.OutputFile)
}

// Write report header
func writeHeader(file *os.File, cfg *UnifiedConfig) {
	mode := "Capture"
	if cfg.MonitorMode {
		mode = "Monitor"
	}

	fmt.Fprintf(file, "LOCAL AGENT - UNIFIED %s REPORT\n", mode)
	fmt.Fprintf(file, "=====================================\n")
	fmt.Fprintf(file, "Generated: %s\n", time.Now().Format("Mon Jan 02 15:04:05 MST 2006"))
	fmt.Fprintf(file, "Mode: %s\n", mode)
	fmt.Fprintf(file, "Capture Type: %s\n", getCaptureType(cfg))
	fmt.Fprintf(file, "Output File: %s\n", cfg.OutputFile)
	fmt.Fprintf(file, "Log File: %s\n", cfg.LogFile)
	fmt.Fprintf(file, "\n")
}

// Write resource data
func writeResourceData(file *os.File, resourceMonitor monitor.ResourceMonitor, prettyPrint bool) {
	fmt.Fprintf(file, "RESOURCE INFORMATION\n")
	fmt.Fprintf(file, "====================\n\n")

	// Get all resource data
	data, err := resourceMonitor.GetAllResources()
	if err != nil {
		fmt.Fprintf(file, "ERROR: Failed to get resource data: %v\n\n", err)
		return
	}

	// Write JSON data
	encoder := json.NewEncoder(file)
	if prettyPrint {
		encoder.SetIndent("", "  ")
	}
	if err := encoder.Encode(data); err != nil {
		fmt.Fprintf(file, "ERROR: Failed to encode resource data: %v\n\n", err)
		return
	}
	fmt.Fprintf(file, "\n")
}

// Write health data
func writeHealthData(file *os.File, resourceMonitor monitor.ResourceMonitor, prettyPrint bool) {
	fmt.Fprintf(file, "HEALTH INFORMATION\n")
	fmt.Fprintf(file, "==================\n\n")

	// Get health data from resource monitor
	data, err := resourceMonitor.GetHealthInfo()
	if err != nil {
		fmt.Fprintf(file, "ERROR: Failed to get health data: %v\n\n", err)
		return
	}

	// Get VPN data
	vpnData, err := resourceMonitor.GetVPNInfo()
	if err != nil {
		fmt.Fprintf(file, "ERROR: Failed to get VPN data: %v\n\n", err)
		return
	}

	// Create combined health data
	healthData := map[string]interface{}{
		"is_healthy":   data.IsHealthy,
		"is_online":    data.IsOnline,
		"has_internet": data.HasInternet,
		"vpn_status": map[string]interface{}{
			"is_connected": vpnData.IsConnected,
			"ip_address":   vpnData.IPAddress,
			"interface":    vpnData.Interface,
			"timestamp":    vpnData.Timestamp,
		},
		"timestamp": time.Now(),
	}

	// Write JSON data
	encoder := json.NewEncoder(file)
	if prettyPrint {
		encoder.SetIndent("", "  ")
	}
	if err := encoder.Encode(healthData); err != nil {
		fmt.Fprintf(file, "ERROR: Failed to encode health data: %v\n\n", err)
		return
	}
	fmt.Fprintf(file, "\n")
}

// Write report footer
func writeFooter(file *os.File, cfg *UnifiedConfig) {
	mode := "Capture"
	if cfg.MonitorMode {
		mode = "Monitor"
	}

	fmt.Fprintf(file, "===============================================\n")
	fmt.Fprintf(file, "%s SUMMARY\n", mode)
	fmt.Fprintf(file, "===============================================\n")
	fmt.Fprintf(file, "Mode: %s\n", mode)
	fmt.Fprintf(file, "Capture Type: %s\n", getCaptureType(cfg))
	fmt.Fprintf(file, "Output File: %s\n", cfg.OutputFile)
	fmt.Fprintf(file, "Generated: %s\n", time.Now().Format("Mon Jan 02 15:04:05 MST 2006"))
}

// Get capture type description
func getCaptureType(cfg *UnifiedConfig) string {
	if cfg.CaptureHealth && cfg.CaptureResources {
		return "Full (Resources + Health)"
	} else if cfg.CaptureHealth {
		return "Health Only"
	} else if cfg.CaptureResources {
		return "Resources Only"
	}
	return "Unknown"
}

// Print help information
func printHelp() {
	fmt.Println("Local Agent Unified Tool")
	fmt.Println("========================")
	fmt.Println()
	fmt.Println("Usage: go run cmd/unified/main.go [flags]")
	fmt.Println()
	fmt.Println("This tool combines resource capture and VPN monitoring functionality.")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -output string")
	fmt.Println("        Output file path (default: reports/unified_YYYYMMDD_HHMMSS.txt)")
	fmt.Println("  -log string")
	fmt.Println("        Log file path (default: logs/unified.log)")
	fmt.Println("  -wait duration")
	fmt.Println("        Time to wait for agent to start (default: 5s)")
	fmt.Println("  -health-only")
	fmt.Println("        Capture only health information")
	fmt.Println("  -resources-only")
	fmt.Println("        Capture only resource information")
	fmt.Println("  -pretty")
	fmt.Println("        Pretty print JSON output")
	fmt.Println("  -monitor")
	fmt.Println("        Run in monitoring mode (continuous)")
	fmt.Println("  -interval duration")
	fmt.Println("        Check interval for monitoring mode (default: 30s)")
	fmt.Println("  -help")
	fmt.Println("        Show this help information")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  go run cmd/unified/main.go")
	fmt.Println("  go run cmd/unified/main.go -pretty")
	fmt.Println("  go run cmd/unified/main.go -monitor -interval 10s")
	fmt.Println("  go run cmd/unified/main.go -health-only -pretty")
}
