package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"k3s-local-agent/internal/config"
	"k3s-local-agent/internal/controlplane"
	"k3s-local-agent/internal/k3s"
	"k3s-local-agent/internal/monitor"
	"k3s-local-agent/pkg/logger"

	"k8s.io/apimachinery/pkg/api/resource"
)

// K3sAgentConfig holds configuration for the K3s agent
type K3sAgentConfig struct {
	OutputFile       string
	LogFile          string
	Namespace        string
	MonitorMode      bool
	CheckInterval    time.Duration
	ScheduleWorkload bool
	PodName          string
	Image            string
	CPURequest       string
	MemoryRequest    string
	PrettyPrint      bool
	// Control plane configuration
	ControlPlaneURL    string
	ControlPlaneKey    string
	AgentID            string
	SendToControlPlane bool
}

func main() {
	// Parse command line flags
	var (
		outputFile    = flag.String("output", "", "Output file path (default: reports/k3s_agent_YYYYMMDD_HHMMSS.txt)")
		logFile       = flag.String("log", "", "Log file path (default: logs/k3s_agent.log)")
		namespace     = flag.String("namespace", "default", "Kubernetes namespace to use")
		monitorMode   = flag.Bool("monitor", false, "Run in monitoring mode")
		checkInterval = flag.Duration("interval", 30*time.Second, "Check interval for monitoring mode")
		schedulePod   = flag.Bool("schedule", false, "Schedule a test pod")
		podName       = flag.String("pod-name", "test-pod", "Name for the pod to schedule")
		image         = flag.String("image", "nginx:alpine", "Container image for the pod")
		cpuRequest    = flag.String("cpu", "100m", "CPU request for the pod")
		memoryRequest = flag.String("memory", "128Mi", "Memory request for the pod")
		prettyPrint   = flag.Bool("pretty", false, "Pretty print JSON output")
		// Control plane flags
		controlPlaneURL    = flag.String("control-plane-url", "", "Control plane URL")
		controlPlaneKey    = flag.String("control-plane-key", "", "Control plane API key")
		agentID            = flag.String("agent-id", "", "Agent ID for control plane")
		sendToControlPlane = flag.Bool("send-to-control-plane", false, "Send data to control plane")
		help               = flag.Bool("help", false, "Show help information")
	)
	flag.Parse()

	if *help {
		printHelp()
		return
	}

	// Setup configuration
	cfg := setupK3sAgentConfig(*outputFile, *logFile, *namespace, *monitorMode, *checkInterval, *schedulePod, *podName, *image, *cpuRequest, *memoryRequest, *prettyPrint, *controlPlaneURL, *controlPlaneKey, *agentID, *sendToControlPlane)

	// Setup logger
	log := logger.New()
	log.Info("Starting K3s local agent...")

	// Create output directory
	if err := os.MkdirAll(filepath.Dir(cfg.OutputFile), 0755); err != nil {
		log.Fatal("Failed to create output directory", err)
	}

	// Load application config
	appConfig, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load configuration", err)
	}

	// Create K3s resource monitor
	k3sMonitor, err := monitor.NewK3sResourceMonitor(appConfig, log, cfg.Namespace)
	if err != nil {
		log.Fatal("Failed to create K3s resource monitor", err)
	}

	// Create control plane client if configured
	var controlPlaneClient *controlplane.ControlPlaneClient
	if cfg.SendToControlPlane && cfg.ControlPlaneURL != "" {
		controlPlaneConfig := &controlplane.ControlPlaneConfig{
			BaseURL: cfg.ControlPlaneURL,
			APIKey:  cfg.ControlPlaneKey,
			AgentID: cfg.AgentID,
			Timeout: 30 * time.Second,
		}

		controlPlaneClient = controlplane.NewControlPlaneClient(controlPlaneConfig, log)

		// Test connection to control plane
		if err := controlPlaneClient.TestConnection(); err != nil {
			log.Warn("Failed to connect to control plane", "error", err)
		} else {
			log.Info("Successfully connected to control plane")
		}
	}

	if cfg.MonitorMode {
		// Run in monitoring mode
		runK3sMonitoringMode(cfg, log, k3sMonitor, controlPlaneClient)
	} else if cfg.ScheduleWorkload {
		// Run in scheduling mode
		runK3sSchedulingMode(cfg, log, k3sMonitor, controlPlaneClient)
	} else {
		// Run in capture mode
		runK3sCaptureMode(cfg, log, k3sMonitor, controlPlaneClient)
	}
}

// Setup K3s agent configuration
func setupK3sAgentConfig(outputFile, logFile, namespace string, monitorMode bool, checkInterval time.Duration, scheduleWorkload bool, podName, image, cpuRequest, memoryRequest string, prettyPrint bool, controlPlaneURL, controlPlaneKey, agentID string, sendToControlPlane bool) *K3sAgentConfig {
	// Generate default output file name if not provided
	if outputFile == "" {
		timestamp := time.Now().Format("20060102_150405")
		if monitorMode {
			outputFile = fmt.Sprintf("reports/k3s_monitor_%s.txt", timestamp)
		} else if scheduleWorkload {
			outputFile = fmt.Sprintf("reports/k3s_schedule_%s.txt", timestamp)
		} else {
			outputFile = fmt.Sprintf("reports/k3s_agent_%s.txt", timestamp)
		}
	}

	// Generate default log file name if not provided
	if logFile == "" {
		if monitorMode {
			logFile = "logs/k3s_monitor.log"
		} else if scheduleWorkload {
			logFile = "logs/k3s_schedule.log"
		} else {
			logFile = "logs/k3s_agent.log"
		}
	}

	// Generate default agent ID if not provided
	if agentID == "" {
		hostname, _ := os.Hostname()
		agentID = fmt.Sprintf("%s-%d", hostname, time.Now().Unix())
	}

	return &K3sAgentConfig{
		OutputFile:         outputFile,
		LogFile:            logFile,
		Namespace:          namespace,
		MonitorMode:        monitorMode,
		CheckInterval:      checkInterval,
		ScheduleWorkload:   scheduleWorkload,
		PodName:            podName,
		Image:              image,
		CPURequest:         cpuRequest,
		MemoryRequest:      memoryRequest,
		PrettyPrint:        prettyPrint,
		ControlPlaneURL:    controlPlaneURL,
		ControlPlaneKey:    controlPlaneKey,
		AgentID:            agentID,
		SendToControlPlane: sendToControlPlane,
	}
}

// Run in K3s monitoring mode - continuous monitoring
func runK3sMonitoringMode(cfg *K3sAgentConfig, log logger.Logger, k3sMonitor *monitor.K3sResourceMonitor, controlPlaneClient *controlplane.ControlPlaneClient) {
	log.Info("Running K3s agent in monitoring mode...")

	stopCh := make(chan struct{})
	defer close(stopCh)

	// Start cluster monitoring in background
	go k3sMonitor.MonitorCluster(cfg.CheckInterval, stopCh)

	// Main monitoring loop
	for {
		// Capture current state
		captureK3sData(cfg, log, k3sMonitor, controlPlaneClient)

		// Wait for next check
		log.Info("Waiting for next check...", "interval", cfg.CheckInterval)
		time.Sleep(cfg.CheckInterval)
	}
}

// Run in K3s scheduling mode - schedule a test workload
func runK3sSchedulingMode(cfg *K3sAgentConfig, log logger.Logger, k3sMonitor *monitor.K3sResourceMonitor, controlPlaneClient *controlplane.ControlPlaneClient) {
	log.Info("Running K3s agent in scheduling mode...")

	// Parse resource requests
	cpuRequest, err := resource.ParseQuantity(cfg.CPURequest)
	if err != nil {
		log.Fatal("Invalid CPU request", "error", err)
	}

	memoryRequest, err := resource.ParseQuantity(cfg.MemoryRequest)
	if err != nil {
		log.Fatal("Invalid memory request", "error", err)
	}

	// Get scheduling recommendation
	recommendation, err := k3sMonitor.GetSchedulingRecommendation()
	if err != nil {
		log.Error("Failed to get scheduling recommendation", "error", err)
	} else {
		log.Info("Scheduling recommendation",
			"best_node", recommendation.BestNode,
			"available_cpu", recommendation.AvailableCPU.String(),
			"available_memory", recommendation.AvailableMemory.String(),
			"recommendation", recommendation.Recommendation)
	}

	// Schedule the pod
	decision, err := k3sMonitor.ScheduleWorkload(cfg.PodName, cfg.Image, cpuRequest, memoryRequest)
	if err != nil {
		log.Fatal("Failed to schedule workload", "error", err)
	}

	// Send scheduling decision to control plane
	if controlPlaneClient != nil {
		if err := controlPlaneClient.SendSchedulingDecision(decision); err != nil {
			log.Error("Failed to send scheduling decision to control plane", "error", err)
		}
	}

	// Capture scheduling result
	captureK3sSchedulingResult(cfg, log, k3sMonitor, decision, controlPlaneClient)

	log.Info("Scheduling mode completed successfully")
}

// Run in K3s capture mode - single capture
func runK3sCaptureMode(cfg *K3sAgentConfig, log logger.Logger, k3sMonitor *monitor.K3sResourceMonitor, controlPlaneClient *controlplane.ControlPlaneClient) {
	log.Info("Running K3s agent in capture mode...")

	// Capture data
	captureK3sData(cfg, log, k3sMonitor, controlPlaneClient)

	log.Info("Capture mode completed successfully")
}

// Capture K3s system data
func captureK3sData(cfg *K3sAgentConfig, log logger.Logger, k3sMonitor *monitor.K3sResourceMonitor, controlPlaneClient *controlplane.ControlPlaneClient) {
	log.Info("Capturing K3s system data...")

	// Create output file
	file, err := os.Create(cfg.OutputFile)
	if err != nil {
		log.Fatal("Failed to create output file", err)
	}
	defer file.Close()

	// Write header
	writeK3sHeader(file, cfg)

	// Capture timestamp
	timestamp := time.Now()
	fmt.Fprintf(file, "TIMESTAMP: %s\n", timestamp.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(file, "===============================================\n\n")

	// Get resource summary
	summary, err := k3sMonitor.GetResourceSummary()
	if err != nil {
		fmt.Fprintf(file, "ERROR: Failed to get resource summary: %v\n\n", err)
	} else {
		fmt.Fprintf(file, "%s\n", summary)
	}

	// Get JSON data
	jsonData, err := k3sMonitor.ExportToJSON()
	if err != nil {
		fmt.Fprintf(file, "ERROR: Failed to export JSON data: %v\n\n", err)
	} else {
		fmt.Fprintf(file, "JSON DATA:\n")
		fmt.Fprintf(file, "==========\n")
		if cfg.PrettyPrint {
			var prettyJSON map[string]interface{}
			if err := json.Unmarshal(jsonData, &prettyJSON); err == nil {
				encoder := json.NewEncoder(file)
				encoder.SetIndent("", "  ")
				encoder.Encode(prettyJSON)
			} else {
				file.Write(jsonData)
			}
		} else {
			file.Write(jsonData)
		}
		fmt.Fprintf(file, "\n")
	}

	// Send to control plane if configured
	if controlPlaneClient != nil {
		k3sData, err := k3sMonitor.GetAllK3sResources()
		if err != nil {
			log.Error("Failed to get K3s data for control plane", "error", err)
		} else {
			if err := controlPlaneClient.SendMonitoringData(k3sData); err != nil {
				log.Error("Failed to send data to control plane", "error", err)
			} else {
				log.Info("Data sent to control plane successfully")
			}
		}
	}

	// Write footer
	writeK3sFooter(file, cfg)

	log.Info("K3s data captured successfully", "output_file", cfg.OutputFile)
}

// Capture K3s scheduling result
func captureK3sSchedulingResult(cfg *K3sAgentConfig, log logger.Logger, k3sMonitor *monitor.K3sResourceMonitor, decision *k3s.SchedulingDecision, controlPlaneClient *controlplane.ControlPlaneClient) {
	log.Info("Capturing K3s scheduling result...")

	// Create output file
	file, err := os.Create(cfg.OutputFile)
	if err != nil {
		log.Fatal("Failed to create output file", err)
	}
	defer file.Close()

	// Write header
	writeK3sHeader(file, cfg)

	// Capture timestamp
	timestamp := time.Now()
	fmt.Fprintf(file, "TIMESTAMP: %s\n", timestamp.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(file, "===============================================\n\n")

	// Write scheduling decision
	fmt.Fprintf(file, "SCHEDULING DECISION:\n")
	fmt.Fprintf(file, "====================\n")
	fmt.Fprintf(file, "Pod Name: %s\n", decision.PodName)
	fmt.Fprintf(file, "Target Node: %s\n", decision.TargetNode)
	fmt.Fprintf(file, "Reason: %s\n", decision.Reason)
	fmt.Fprintf(file, "CPU Request: %s\n", decision.CPURequest.String())
	fmt.Fprintf(file, "Memory Request: %s\n", decision.MemoryRequest.String())
	fmt.Fprintf(file, "Timestamp: %s\n\n", decision.Timestamp.Format("2006-01-02 15:04:05"))

	// Get current resource summary
	summary, err := k3sMonitor.GetResourceSummary()
	if err != nil {
		fmt.Fprintf(file, "ERROR: Failed to get resource summary: %v\n\n", err)
	} else {
		fmt.Fprintf(file, "CURRENT RESOURCE SUMMARY:\n")
		fmt.Fprintf(file, "=========================\n")
		fmt.Fprintf(file, "%s\n", summary)
	}

	// Write footer
	writeK3sFooter(file, cfg)

	log.Info("K3s scheduling result captured successfully", "output_file", cfg.OutputFile)
}

// Write K3s report header
func writeK3sHeader(file *os.File, cfg *K3sAgentConfig) {
	mode := "Capture"
	if cfg.MonitorMode {
		mode = "Monitor"
	} else if cfg.ScheduleWorkload {
		mode = "Schedule"
	}

	fmt.Fprintf(file, "K3s LOCAL AGENT - %s REPORT\n", mode)
	fmt.Fprintf(file, "=====================================\n")
	fmt.Fprintf(file, "Generated: %s\n", time.Now().Format("Mon Jan 02 15:04:05 MST 2006"))
	fmt.Fprintf(file, "Mode: %s\n", mode)
	fmt.Fprintf(file, "Namespace: %s\n", cfg.Namespace)
	if cfg.ScheduleWorkload {
		fmt.Fprintf(file, "Pod Name: %s\n", cfg.PodName)
		fmt.Fprintf(file, "Image: %s\n", cfg.Image)
		fmt.Fprintf(file, "CPU Request: %s\n", cfg.CPURequest)
		fmt.Fprintf(file, "Memory Request: %s\n", cfg.MemoryRequest)
	}
	if cfg.SendToControlPlane {
		fmt.Fprintf(file, "Control Plane: %s\n", cfg.ControlPlaneURL)
		fmt.Fprintf(file, "Agent ID: %s\n", cfg.AgentID)
	}
	fmt.Fprintf(file, "Output File: %s\n", cfg.OutputFile)
	fmt.Fprintf(file, "Log File: %s\n", cfg.LogFile)
	fmt.Fprintf(file, "\n")
}

// Write K3s report footer
func writeK3sFooter(file *os.File, cfg *K3sAgentConfig) {
	mode := "Capture"
	if cfg.MonitorMode {
		mode = "Monitor"
	} else if cfg.ScheduleWorkload {
		mode = "Schedule"
	}

	fmt.Fprintf(file, "===============================================\n")
	fmt.Fprintf(file, "%s SUMMARY\n", mode)
	fmt.Fprintf(file, "===============================================\n")
	fmt.Fprintf(file, "Mode: %s\n", mode)
	fmt.Fprintf(file, "Namespace: %s\n", cfg.Namespace)
	fmt.Fprintf(file, "Output File: %s\n", cfg.OutputFile)
	fmt.Fprintf(file, "Generated: %s\n", time.Now().Format("Mon Jan 02 15:04:05 MST 2006"))
}

// Print help information
func printHelp() {
	fmt.Println("K3s Local Agent")
	fmt.Println("===============")
	fmt.Println()
	fmt.Println("Usage: go run cmd/k3s-agent/main.go [flags]")
	fmt.Println()
	fmt.Println("This tool integrates local system monitoring with K3s cluster management.")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -output string")
	fmt.Println("        Output file path (default: reports/k3s_agent_YYYYMMDD_HHMMSS.txt)")
	fmt.Println("  -log string")
	fmt.Println("        Log file path (default: logs/k3s_agent.log)")
	fmt.Println("  -namespace string")
	fmt.Println("        Kubernetes namespace to use (default: default)")
	fmt.Println("  -monitor")
	fmt.Println("        Run in monitoring mode (continuous)")
	fmt.Println("  -interval duration")
	fmt.Println("        Check interval for monitoring mode (default: 30s)")
	fmt.Println("  -schedule")
	fmt.Println("        Schedule a test pod")
	fmt.Println("  -pod-name string")
	fmt.Println("        Name for the pod to schedule (default: test-pod)")
	fmt.Println("  -image string")
	fmt.Println("        Container image for the pod (default: nginx:alpine)")
	fmt.Println("  -cpu string")
	fmt.Println("        CPU request for the pod (default: 100m)")
	fmt.Println("  -memory string")
	fmt.Println("        Memory request for the pod (default: 128Mi)")
	fmt.Println("  -pretty")
	fmt.Println("        Pretty print JSON output")
	fmt.Println("  -control-plane-url string")
	fmt.Println("        Control plane URL for sending data")
	fmt.Println("  -control-plane-key string")
	fmt.Println("        Control plane API key")
	fmt.Println("  -agent-id string")
	fmt.Println("        Agent ID for control plane (auto-generated if not provided)")
	fmt.Println("  -send-to-control-plane")
	fmt.Println("        Send monitoring data to control plane")
	fmt.Println("  -help")
	fmt.Println("        Show this help information")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  go run cmd/k3s-agent/main.go")
	fmt.Println("  go run cmd/k3s-agent/main.go -monitor -interval 10s")
	fmt.Println("  go run cmd/k3s-agent/main.go -schedule -pod-name my-app -image nginx:latest")
	fmt.Println("  go run cmd/k3s-agent/main.go -pretty -namespace my-namespace")
	fmt.Println("  go run cmd/k3s-agent/main.go -send-to-control-plane -control-plane-url https://api.example.com -control-plane-key my-api-key")
}
