package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"k3s-local-agent/internal/config"
	"k3s-local-agent/internal/staging"
	"k3s-local-agent/pkg/logger"
)

// StagingAgentConfig holds configuration for the staging agent
type StagingAgentConfig struct {
	OutputFile       string
	LogFile          string
	AgentID          string
	ControlPlaneURL  string
	ControlPlanePort int
	KindClusterName  string
	LocalNamespace   string
	AgentPort        int
	SyncInterval     time.Duration
	PrettyPrint      bool
	MonitorMode      bool
	CheckInterval    time.Duration
}

func main() {
	// Parse command line flags
	var (
		outputFile       = flag.String("output", "", "Output file path (default: reports/staging_agent_YYYYMMDD_HHMMSS.txt)")
		logFile          = flag.String("log", "", "Log file path (default: logs/staging_agent.log)")
		agentID          = flag.String("agent-id", "", "Agent ID (default: auto-generated)")
		controlPlaneURL  = flag.String("control-plane-url", "http://localhost:8080", "Control plane URL")
		controlPlanePort = flag.Int("control-plane-port", 8080, "Control plane port")
		kindClusterName  = flag.String("kind-cluster", "staging-cluster", "Kind cluster name")
		localNamespace   = flag.String("namespace", "staging", "Local namespace for staging pods")
		agentPort        = flag.Int("agent-port", 8082, "Agent port for receiving pod data")
		syncInterval     = flag.Duration("sync-interval", 30*time.Second, "Sync interval for staging pods")
		prettyPrint      = flag.Bool("pretty", false, "Pretty print JSON output")
		monitorMode      = flag.Bool("monitor", false, "Run in monitoring mode")
		checkInterval    = flag.Duration("interval", 60*time.Second, "Check interval for monitoring mode")
		help             = flag.Bool("help", false, "Show help information")
	)
	flag.Parse()

	if *help {
		printStagingHelp()
		return
	}

	// Setup configuration
	cfg := setupStagingAgentConfig(*outputFile, *logFile, *agentID, *controlPlaneURL, *controlPlanePort, *kindClusterName, *localNamespace, *agentPort, *syncInterval, *prettyPrint, *monitorMode, *checkInterval)

	// Setup logger
	log := logger.New()
	log.Info("Starting staging agent...")

	// Create output directory
	if err := os.MkdirAll(filepath.Dir(cfg.OutputFile), 0755); err != nil {
		log.Fatal("Failed to create output directory", err)
	}

	// Load application config
	_, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load configuration", err)
	}

	// Create staging agent
	stagingConfig := &staging.StagingConfig{
		AgentID:          cfg.AgentID,
		ControlPlaneURL:  cfg.ControlPlaneURL,
		ControlPlanePort: cfg.ControlPlanePort,
		KindClusterName:  cfg.KindClusterName,
		LocalNamespace:   cfg.LocalNamespace,
		AgentPort:        cfg.AgentPort,
		SyncInterval:     cfg.SyncInterval,
	}

	stagingAgent, err := staging.NewLocalStagingAgent(stagingConfig, log)
	if err != nil {
		log.Fatal("Failed to create staging agent", err)
	}

	if cfg.MonitorMode {
		// Run in monitoring mode
		runStagingMonitoringMode(cfg, log, stagingAgent)
	} else {
		// Run in capture mode
		runStagingCaptureMode(cfg, log, stagingAgent)
	}
}

// Setup staging agent configuration
func setupStagingAgentConfig(outputFile, logFile, agentID, controlPlaneURL string, controlPlanePort int, kindClusterName, localNamespace string, agentPort int, syncInterval time.Duration, prettyPrint, monitorMode bool, checkInterval time.Duration) *StagingAgentConfig {
	// Generate default output file name if not provided
	if outputFile == "" {
		timestamp := time.Now().Format("20060102_150405")
		if monitorMode {
			outputFile = fmt.Sprintf("reports/staging_monitor_%s.txt", timestamp)
		} else {
			outputFile = fmt.Sprintf("reports/staging_agent_%s.txt", timestamp)
		}
	}

	// Generate default log file name if not provided
	if logFile == "" {
		if monitorMode {
			logFile = "logs/staging_monitor.log"
		} else {
			logFile = "logs/staging_agent.log"
		}
	}

	// Generate default agent ID if not provided
	if agentID == "" {
		hostname, _ := os.Hostname()
		agentID = fmt.Sprintf("staging-agent-%s-%d", hostname, time.Now().Unix())
	}

	return &StagingAgentConfig{
		OutputFile:       outputFile,
		LogFile:          logFile,
		AgentID:          agentID,
		ControlPlaneURL:  controlPlaneURL,
		ControlPlanePort: controlPlanePort,
		KindClusterName:  kindClusterName,
		LocalNamespace:   localNamespace,
		AgentPort:        agentPort,
		SyncInterval:     syncInterval,
		PrettyPrint:      prettyPrint,
		MonitorMode:      monitorMode,
		CheckInterval:    checkInterval,
	}
}

// Run staging capture mode
func runStagingCaptureMode(cfg *StagingAgentConfig, log logger.Logger, stagingAgent *staging.LocalStagingAgent) {
	log.Info("Running staging agent in capture mode...")

	// Start the staging agent
	if err := stagingAgent.Start(); err != nil {
		log.Fatal("Failed to start staging agent", err)
	}

	// Wait for agent to be ready
	log.Info("Waiting for staging agent to be ready...")
	time.Sleep(10 * time.Second)

	// Capture staging data
	captureStagingData(cfg, log, stagingAgent)

	// Stop the staging agent
	if err := stagingAgent.Stop(); err != nil {
		log.Error("Failed to stop staging agent", err)
	}

	log.Info("Staging capture mode completed successfully")
}

// Run staging monitoring mode
func runStagingMonitoringMode(cfg *StagingAgentConfig, log logger.Logger, stagingAgent *staging.LocalStagingAgent) {
	log.Info("Running staging agent in monitoring mode...")

	// Start the staging agent
	if err := stagingAgent.Start(); err != nil {
		log.Fatal("Failed to start staging agent", err)
	}

	for {
		// Capture current staging state
		captureStagingData(cfg, log, stagingAgent)

		// Wait for next check
		log.Info("Waiting for next staging check...", "interval", cfg.CheckInterval)
		time.Sleep(cfg.CheckInterval)
	}
}

// Capture staging system data
func captureStagingData(cfg *StagingAgentConfig, log logger.Logger, stagingAgent *staging.LocalStagingAgent) {
	log.Info("Capturing staging system data...")

	// Create output file
	file, err := os.Create(cfg.OutputFile)
	if err != nil {
		log.Fatal("Failed to create output file", err)
	}
	defer file.Close()

	// Write staging header
	writeStagingHeader(file, cfg)

	// Capture timestamp
	timestamp := time.Now()
	fmt.Fprintf(file, "TIMESTAMP: %s\n", timestamp.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(file, "===============================================\n\n")

	// Get staging status
	status := stagingAgent.GetStagingStatus()

	// Write staging status
	writeStagingStatus(file, status, cfg.PrettyPrint)

	// Get staging pods
	stagingPods := stagingAgent.GetStagingPods()

	// Write staging pods
	writeStagingPods(file, stagingPods, cfg.PrettyPrint)

	// Get and write HTTP proxies
	proxies := stagingAgent.GetHTTPProxies()
	writeHTTPProxies(file, proxies, cfg.PrettyPrint)

	// Get and write proxy status
	proxyStatus := stagingAgent.GetProxyStatus()
	writeProxyStatus(file, proxyStatus, cfg.PrettyPrint)

	// Write staging footer
	writeStagingFooter(file, cfg)

	log.Info("Staging data captured successfully", "output_file", cfg.OutputFile)
}

// Write staging report header
func writeStagingHeader(file *os.File, cfg *StagingAgentConfig) {
	mode := "Staging Capture"
	if cfg.MonitorMode {
		mode = "Staging Monitor"
	}

	fmt.Fprintf(file, "STAGING AGENT - %s REPORT\n", mode)
	fmt.Fprintf(file, "=====================================\n")
	fmt.Fprintf(file, "Generated: %s\n", time.Now().Format("Mon Jan 02 15:04:05 MST 2006"))
	fmt.Fprintf(file, "Mode: %s\n", mode)
	fmt.Fprintf(file, "Agent ID: %s\n", cfg.AgentID)
	fmt.Fprintf(file, "Control Plane URL: %s\n", cfg.ControlPlaneURL)
	fmt.Fprintf(file, "Kind Cluster: %s\n", cfg.KindClusterName)
	fmt.Fprintf(file, "Local Namespace: %s\n", cfg.LocalNamespace)
	fmt.Fprintf(file, "Agent Port: %d\n", cfg.AgentPort)
	fmt.Fprintf(file, "Output File: %s\n", cfg.OutputFile)
	fmt.Fprintf(file, "Log File: %s\n", cfg.LogFile)
	fmt.Fprintf(file, "\n")
}

// Write staging status
func writeStagingStatus(file *os.File, status *staging.StagingStatus, prettyPrint bool) {
	fmt.Fprintf(file, "STAGING STATUS\n")
	fmt.Fprintf(file, "==============\n\n")

	// Write JSON data
	encoder := json.NewEncoder(file)
	if prettyPrint {
		encoder.SetIndent("", "  ")
	}
	if err := encoder.Encode(status); err != nil {
		fmt.Fprintf(file, "ERROR: Failed to encode staging status: %v\n\n", err)
		return
	}
	fmt.Fprintf(file, "\n")
}

// Write staging pods
func writeStagingPods(file *os.File, stagingPods map[string]staging.StagingPodInfo, prettyPrint bool) {
	fmt.Fprintf(file, "STAGING PODS\n")
	fmt.Fprintf(file, "============\n\n")

	podData := map[string]interface{}{
		"total_pods": len(stagingPods),
		"pods":       stagingPods,
		"timestamp":  time.Now(),
	}

	// Write JSON data
	encoder := json.NewEncoder(file)
	if prettyPrint {
		encoder.SetIndent("", "  ")
	}
	if err := encoder.Encode(podData); err != nil {
		fmt.Fprintf(file, "ERROR: Failed to encode staging pods: %v\n\n", err)
		return
	}
	fmt.Fprintf(file, "\n")
}

// Write HTTP proxies
func writeHTTPProxies(file *os.File, proxies map[string]staging.HTTPProxy, prettyPrint bool) {
	fmt.Fprintf(file, "HTTP PROXIES\n")
	fmt.Fprintf(file, "============\n\n")

	proxyData := map[string]interface{}{
		"total_proxies": len(proxies),
		"proxies":       proxies,
		"timestamp":     time.Now(),
	}

	// Write JSON data
	encoder := json.NewEncoder(file)
	if prettyPrint {
		encoder.SetIndent("", "  ")
	}
	if err := encoder.Encode(proxyData); err != nil {
		fmt.Fprintf(file, "ERROR: Failed to encode HTTP proxies: %v\n\n", err)
		return
	}
	fmt.Fprintf(file, "\n")
}

// Write proxy status
func writeProxyStatus(file *os.File, proxyStatus map[string]interface{}, prettyPrint bool) {
	fmt.Fprintf(file, "PROXY STATUS\n")
	fmt.Fprintf(file, "============\n\n")

	// Write JSON data
	encoder := json.NewEncoder(file)
	if prettyPrint {
		encoder.SetIndent("", "  ")
	}
	if err := encoder.Encode(proxyStatus); err != nil {
		fmt.Fprintf(file, "ERROR: Failed to encode proxy status: %v\n\n", err)
		return
	}
	fmt.Fprintf(file, "\n")
}

// Write staging report footer
func writeStagingFooter(file *os.File, cfg *StagingAgentConfig) {
	mode := "Staging Capture"
	if cfg.MonitorMode {
		mode = "Staging Monitor"
	}

	fmt.Fprintf(file, "===============================================\n")
	fmt.Fprintf(file, "%s SUMMARY\n", mode)
	fmt.Fprintf(file, "===============================================\n")
	fmt.Fprintf(file, "Mode: %s\n", mode)
	fmt.Fprintf(file, "Agent ID: %s\n", cfg.AgentID)
	fmt.Fprintf(file, "Control Plane URL: %s\n", cfg.ControlPlaneURL)
	fmt.Fprintf(file, "Kind Cluster: %s\n", cfg.KindClusterName)
	fmt.Fprintf(file, "Output File: %s\n", cfg.OutputFile)
	fmt.Fprintf(file, "Generated: %s\n", time.Now().Format("Mon Jan 02 15:04:05 MST 2006"))
}

// Print staging help information
func printStagingHelp() {
	fmt.Println("Staging Agent - Local Staging Pod Manager")
	fmt.Println("==========================================")
	fmt.Println()
	fmt.Println("Usage: go run cmd/staging-agent/main.go [flags]")
	fmt.Println()
	fmt.Println("This agent receives staging pod data from control plane and")
	fmt.Println("hosts them locally in a kind cluster for local development.")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -output string")
	fmt.Println("        Output file path (default: reports/staging_agent_YYYYMMDD_HHMMSS.txt)")
	fmt.Println("  -log string")
	fmt.Println("        Log file path (default: logs/staging_agent.log)")
	fmt.Println("  -agent-id string")
	fmt.Println("        Agent ID (default: auto-generated)")
	fmt.Println("  -control-plane-url string")
	fmt.Println("        Control plane URL (default: http://localhost:8080)")
	fmt.Println("  -control-plane-port int")
	fmt.Println("        Control plane port (default: 8080)")
	fmt.Println("  -kind-cluster string")
	fmt.Println("        Kind cluster name (default: staging-cluster)")
	fmt.Println("  -namespace string")
	fmt.Println("        Local namespace for staging pods (default: staging)")
	fmt.Println("  -agent-port int")
	fmt.Println("        Agent port for receiving pod data (default: 8082)")
	fmt.Println("  -sync-interval duration")
	fmt.Println("        Sync interval for staging pods (default: 30s)")
	fmt.Println("  -pretty")
	fmt.Println("        Pretty print JSON output")
	fmt.Println("  -monitor")
	fmt.Println("        Run in monitoring mode (continuous)")
	fmt.Println("  -interval duration")
	fmt.Println("        Check interval for monitoring mode (default: 60s)")
	fmt.Println("  -help")
	fmt.Println("        Show this help information")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  go run cmd/staging-agent/main.go")
	fmt.Println("  go run cmd/staging-agent/main.go -pretty")
	fmt.Println("  go run cmd/staging-agent/main.go -monitor -interval 30s")
	fmt.Println("  go run cmd/staging-agent/main.go -control-plane-url http://staging-control.example.com")
	fmt.Println("  go run cmd/staging-agent/main.go -kind-cluster my-staging-cluster")
}
