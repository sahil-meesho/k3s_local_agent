package agent

import (
	"k3s-local-agent/internal/config"
	"k3s-local-agent/internal/monitor"
	"k3s-local-agent/pkg/logger"
	"time"
)

type Agent interface {
	Start() error
	Stop() error
	PollResources() error
}

type agent struct {
	config  *config.Config
	monitor monitor.ResourceMonitor
	logger  logger.Logger
	stopCh  chan struct{}
}

func New(cfg *config.Config, monitor monitor.ResourceMonitor, log logger.Logger) Agent {
	return &agent{
		config:  cfg,
		monitor: monitor,
		logger:  log,
		stopCh:  make(chan struct{}),
	}
}

func (a *agent) Start() error {
	a.logger.Info("Starting local agent...")

	// Start polling in a goroutine
	go a.pollLoop()

	a.logger.Info("Local agent started successfully")
	return nil
}

func (a *agent) Stop() error {
	a.logger.Info("Stopping local agent...")
	close(a.stopCh)
	return nil
}

func (a *agent) pollLoop() {
	// Poll every 30 seconds
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Initial poll
	if err := a.PollResources(); err != nil {
		a.logger.Debug("Initial resource polling failed (non-critical)", err)
	}

	for {
		select {
		case <-ticker.C:
			if err := a.PollResources(); err != nil {
				a.logger.Debug("Resource polling failed (non-critical)", err)
			}
		case <-a.stopCh:
			a.logger.Info("Polling stopped")
			return
		}
	}
}

func (a *agent) PollResources() error {
	a.logger.Debug("Polling system resources...")

	// Get all resource data
	data, err := a.monitor.GetAllResources()
	if err != nil {
		return err
	}

	a.logger.Debug("Resource polling completed successfully", "timestamp", data.Timestamp)
	return nil
}
