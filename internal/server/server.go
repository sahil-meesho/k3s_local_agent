package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"k3s-local-agent/internal/agent"
	"k3s-local-agent/internal/config"
	"k3s-local-agent/internal/monitor"
	"k3s-local-agent/pkg/logger"

	"github.com/gorilla/mux"
)

type Server interface {
	Start() error
	Shutdown(ctx context.Context) error
}

type server struct {
	config *config.Config
	agent  agent.Agent
	logger logger.Logger
	router *mux.Router
	server *http.Server
}

func New(cfg *config.Config, agent agent.Agent, log logger.Logger) Server {
	router := mux.NewRouter()

	s := &server{
		config: cfg,
		agent:  agent,
		logger: log,
		router: router,
	}

	s.setupRoutes()

	s.server = &http.Server{
		Addr:    cfg.Server.Host + ":" + cfg.Server.Port,
		Handler: router,
	}

	return s
}

// Setup HTTP routes
func (s *server) setupRoutes() {
	// Health check endpoint
	s.router.HandleFunc("/health", s.healthHandler).Methods("GET")

	// Agent registration endpoint
	s.router.HandleFunc("/register-local-agent", s.registerLocalAgentHandler).Methods("POST")

	// Essential resource endpoints
	s.router.HandleFunc("/api/resources", s.getResourcesHandler).Methods("GET")
	s.router.HandleFunc("/api/resources/system", s.getSystemInfoHandler).Methods("GET")
	s.router.HandleFunc("/api/resources/cpu", s.getCPUInfoHandler).Methods("GET")
	s.router.HandleFunc("/api/resources/memory", s.getMemoryInfoHandler).Methods("GET")
	s.router.HandleFunc("/api/resources/vpn", s.getVPNInfoHandler).Methods("GET")
	s.router.HandleFunc("/api/resources/health", s.getHealthInfoHandler).Methods("GET")

	// Manual polling endpoint
	s.router.HandleFunc("/api/poll", s.pollResourcesHandler).Methods("POST")
}

func (s *server) Start() error {
	s.logger.Info("Starting HTTP server on", s.server.Addr)
	return s.server.ListenAndServe()
}

func (s *server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down HTTP server...")
	return s.server.Shutdown(ctx)
}

// Health check handler
func (s *server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now(),
		"service":   "local-agent",
	}

	json.NewEncoder(w).Encode(response)
}

// Get all resources handler
func (s *server) getResourcesHandler(w http.ResponseWriter, r *http.Request) {
	monitor := s.getMonitor()

	data, err := monitor.GetAllResources()
	if err != nil {
		s.logger.Error("Failed to get resources", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// Get system info handler
func (s *server) getSystemInfoHandler(w http.ResponseWriter, r *http.Request) {
	monitor := s.getMonitor()

	data, err := monitor.GetSystemInfo()
	if err != nil {
		s.logger.Error("Failed to get system info", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// Get CPU info handler
func (s *server) getCPUInfoHandler(w http.ResponseWriter, r *http.Request) {
	monitor := s.getMonitor()

	data, err := monitor.GetCPUInfo()
	if err != nil {
		s.logger.Error("Failed to get CPU info", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// Get memory info handler
func (s *server) getMemoryInfoHandler(w http.ResponseWriter, r *http.Request) {
	monitor := s.getMonitor()

	data, err := monitor.GetMemoryInfo()
	if err != nil {
		s.logger.Error("Failed to get memory info", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// Get VPN info handler
func (s *server) getVPNInfoHandler(w http.ResponseWriter, r *http.Request) {
	monitor := s.getMonitor()

	data, err := monitor.GetVPNInfo()
	if err != nil {
		s.logger.Error("Failed to get VPN info", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// Get health info handler
func (s *server) getHealthInfoHandler(w http.ResponseWriter, r *http.Request) {
	monitor := s.getMonitor()

	data, err := monitor.GetHealthInfo()
	if err != nil {
		s.logger.Error("Failed to get health info", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// Poll resources handler
func (s *server) pollResourcesHandler(w http.ResponseWriter, r *http.Request) {
	if err := s.agent.PollResources(); err != nil {
		s.logger.Error("Failed to poll resources", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := map[string]interface{}{
		"status":    "success",
		"message":   "Resources polled successfully",
		"timestamp": time.Now(),
	}

	json.NewEncoder(w).Encode(response)
}

// Register local agent handler
func (s *server) registerLocalAgentHandler(w http.ResponseWriter, r *http.Request) {
	// Parse the request body
	var requestBody map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
		s.logger.Error("Failed to decode request body", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Extract host from request
	host, ok := requestBody["host"].(string)
	if !ok {
		s.logger.Error("Missing or invalid host in request")
		http.Error(w, "Missing or invalid host", http.StatusBadRequest)
		return
	}

	s.logger.Info("Agent registration request received", "host", host)

	// Generate a unique agent ID if not provided
	agentID := fmt.Sprintf("agent-%s-%d", host, time.Now().Unix())

	// Create registration response
	response := map[string]interface{}{
		"status":    "success",
		"message":   "Agent registered successfully",
		"agent_id":  agentID,
		"host":      host,
		"timestamp": time.Now(),
		"endpoints": map[string]string{
			"health":           "/health",
			"resources":        "/api/resources",
			"system_info":      "/api/resources/system",
			"cpu_info":         "/api/resources/cpu",
			"memory_info":      "/api/resources/memory",
			"vpn_info":         "/api/resources/vpn",
			"health_info":      "/api/resources/health",
			"poll_resources":   "/api/poll",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// Helper function to get monitor from agent
func (s *server) getMonitor() monitor.ResourceMonitor {
	// We need to access the monitor from the agent
	// For now, create a new monitor instance
	cfg, err := config.Load()
	if err != nil {
		s.logger.Error("Failed to load config for monitor", err)
		return nil
	}
	return monitor.New(cfg, s.logger)
}
