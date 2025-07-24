package staging

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"k3s-local-agent/pkg/logger"
)

// HTTPProxyManager handles HTTP reverse proxy for staging pods
type HTTPProxyManager struct {
	logger    logger.Logger
	proxies   map[string]HTTPProxy
	mutex     sync.RWMutex
	agentID   string
	server    *http.Server
	proxyPort int
}

// HTTPProxy represents an HTTP proxy configuration
type HTTPProxy struct {
	ProxyID      string    `json:"proxy_id"`
	PodName      string    `json:"pod_name"`
	StagingPodIP string    `json:"staging_pod_ip"`
	StagingPort  int       `json:"staging_port"`
	LocalPath    string    `json:"local_path"` // e.g., "/my-app"
	ProxyURL     string    `json:"proxy_url"`  // e.g., "http://localhost:8080/my-app"
	Status       string    `json:"status"`     // "active", "failed", "pending"
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ProxyConfig holds configuration for HTTP proxy
type ProxyConfig struct {
	AgentID   string
	ProxyPort int
	BasePath  string
	EnableSSL bool
}

// NewHTTPProxyManager creates a new HTTP proxy manager
func NewHTTPProxyManager(config *ProxyConfig, log logger.Logger) *HTTPProxyManager {
	return &HTTPProxyManager{
		logger:    log,
		proxies:   make(map[string]HTTPProxy),
		agentID:   config.AgentID,
		proxyPort: config.ProxyPort,
	}
}

// SetupProxy creates an HTTP proxy for a staging pod
func (hpm *HTTPProxyManager) SetupProxy(stagingPod StagingPodInfo, localPodIP string) (*HTTPProxy, error) {
	hpm.mutex.Lock()
	defer hpm.mutex.Unlock()

	// Check if proxy already exists
	if existing, exists := hpm.proxies[stagingPod.ID]; exists {
		hpm.logger.Info("HTTP proxy already exists",
			"pod", stagingPod.Name,
			"proxy_id", existing.ProxyID,
			"proxy_url", existing.ProxyURL)
		return &existing, nil
	}

	// Create proxy ID
	proxyID := fmt.Sprintf("%s-%s-%s", hpm.agentID, stagingPod.Name, time.Now().Format("20060102"))

	// Create local path
	localPath := fmt.Sprintf("/%s", stagingPod.Name)

	// Create proxy
	proxy := &HTTPProxy{
		ProxyID:      proxyID,
		PodName:      stagingPod.Name,
		StagingPodIP: stagingPod.IP,
		StagingPort:  80, // Default port
		LocalPath:    localPath,
		ProxyURL:     fmt.Sprintf("http://localhost:%d%s", hpm.proxyPort, localPath),
		Status:       "pending",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// Setup proxy routing
	if err := hpm.setupProxyRouting(proxy); err != nil {
		proxy.Status = "failed"
		hpm.logger.Error("Failed to setup proxy routing",
			"pod", stagingPod.Name,
			"error", err)
	} else {
		proxy.Status = "active"
	}

	// Store proxy
	hpm.proxies[stagingPod.ID] = *proxy

	hpm.logger.Info("HTTP proxy setup completed",
		"pod", stagingPod.Name,
		"proxy_id", proxy.ProxyID,
		"proxy_url", proxy.ProxyURL,
		"status", proxy.Status)

	return proxy, nil
}

// setupProxyRouting sets up the HTTP proxy routing
func (hpm *HTTPProxyManager) setupProxyRouting(proxy *HTTPProxy) error {
	// Create target URL
	targetURL, err := url.Parse(fmt.Sprintf("http://%s:%d", proxy.StagingPodIP, proxy.StagingPort))
	if err != nil {
		return fmt.Errorf("failed to parse target URL: %w", err)
	}

	// Create reverse proxy
	reverseProxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Customize the proxy director
	originalDirector := reverseProxy.Director
	reverseProxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Header.Set("X-Proxy-By", "k3s-local-agent")
		req.Header.Set("X-Original-Host", req.Host)
		req.Host = targetURL.Host
	}

	// Add error handling
	reverseProxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		hpm.logger.Error("Proxy error",
			"pod", proxy.PodName,
			"target", targetURL.String(),
			"error", err)
		http.Error(w, "Proxy Error", http.StatusBadGateway)
	}

	// Register the proxy handler
	http.HandleFunc(proxy.LocalPath, func(w http.ResponseWriter, r *http.Request) {
		hpm.logger.Debug("Proxying request",
			"pod", proxy.PodName,
			"path", r.URL.Path,
			"target", targetURL.String())
		reverseProxy.ServeHTTP(w, r)
	})

	// Start HTTP server if not already running
	if hpm.server == nil {
		go hpm.startHTTPServer()
	}

	return nil
}

// startHTTPServer starts the HTTP proxy server
func (hpm *HTTPProxyManager) startHTTPServer() error {
	mux := http.NewServeMux()

	// Add health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "healthy",
			"agent_id":  hpm.agentID,
			"timestamp": time.Now(),
			"proxies":   len(hpm.proxies),
		})
	})

	// Add proxy status endpoint
	mux.HandleFunc("/api/proxies", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(hpm.GetProxyStatus())
	})

	hpm.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", hpm.proxyPort),
		Handler: mux,
	}

	hpm.logger.Info("HTTP proxy server started", "port", hpm.proxyPort)
	return hpm.server.ListenAndServe()
}

// RemoveProxy removes an HTTP proxy
func (hpm *HTTPProxyManager) RemoveProxy(podID string) error {
	hpm.mutex.Lock()
	defer hpm.mutex.Unlock()

	proxy, exists := hpm.proxies[podID]
	if !exists {
		return fmt.Errorf("proxy not found for pod %s", podID)
	}

	// Remove from proxies map
	delete(hpm.proxies, podID)

	hpm.logger.Info("HTTP proxy removed",
		"pod", proxy.PodName,
		"proxy_id", proxy.ProxyID,
		"proxy_url", proxy.ProxyURL)

	return nil
}

// GetProxies returns all active proxies
func (hpm *HTTPProxyManager) GetProxies() map[string]HTTPProxy {
	hpm.mutex.RLock()
	defer hpm.mutex.RUnlock()

	result := make(map[string]HTTPProxy)
	for k, v := range hpm.proxies {
		result[k] = v
	}
	return result
}

// GetProxyStatus returns the status of all proxies
func (hpm *HTTPProxyManager) GetProxyStatus() map[string]interface{} {
	hpm.mutex.RLock()
	defer hpm.mutex.RUnlock()

	status := map[string]interface{}{
		"total_proxies":  len(hpm.proxies),
		"active_proxies": 0,
		"failed_proxies": 0,
		"proxies":        hpm.proxies,
		"timestamp":      time.Now(),
	}

	for _, proxy := range hpm.proxies {
		if proxy.Status == "active" {
			status["active_proxies"] = status["active_proxies"].(int) + 1
		} else if proxy.Status == "failed" {
			status["failed_proxies"] = status["failed_proxies"].(int) + 1
		}
	}

	return status
}

// HealthCheck performs a health check on all proxies
func (hpm *HTTPProxyManager) HealthCheck() map[string]interface{} {
	status := hpm.GetProxyStatus()

	// Check each proxy's health
	for podID, proxy := range hpm.proxies {
		isHealthy := hpm.isProxyHealthy(&proxy)
		status["proxies"].(map[string]HTTPProxy)[podID] = proxy

		if !isHealthy && proxy.Status == "active" {
			hpm.logger.Warn("Proxy health check failed",
				"proxy_id", proxy.ProxyID,
				"pod", proxy.PodName)
		}
	}

	return status
}

// isProxyHealthy checks if a proxy is healthy
func (hpm *HTTPProxyManager) isProxyHealthy(proxy *HTTPProxy) bool {
	// Test the proxy endpoint
	resp, err := http.Get(proxy.ProxyURL)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 500
}
