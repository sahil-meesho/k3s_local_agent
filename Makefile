# K3s Local Agent Makefile

# Variables
BINARY_NAME=k3s-local-agent
BUILD_DIR=build
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME}"

.PHONY: build clean test run k3s-agent all install-k3s setup-k3s help

# Build targets
all: build

build:
	@echo "Building K3s local agent..."
	go build -o build/k3s-agent cmd/k3s-agent/main.go
	@echo "Building unified tool..."
	go build -o build/unified-tool cmd/unified/main.go
	@echo "Building staging agent..."
	go build -o build/staging-agent cmd/staging-agent/main.go
	@echo "Build completed successfully!"

clean:
	@echo "Cleaning build artifacts..."
	rm -f build/k3s-agent build/unified-tool build/staging-agent
	@echo "Clean completed!"

# Test target
test:
	@echo "Running tests..."
	go test -v ./internal/config
	go test -v ./internal/monitor
	go test -v ./pkg/logger
	@echo "Tests completed!"

# Run targets
run: build
	@echo "Running K3s local agent..."
	./build/k3s-agent

k3s-agent: build
	@echo "Running K3s local agent..."
	./build/k3s-agent

unified: build
	@echo "Running unified tool..."
	./build/unified-tool

staging-agent: build
	@echo "Running staging agent..."
	./build/staging-agent

# K3s specific targets
k3s-monitor: build
	@echo "Running K3s agent in monitoring mode..."
	./build/k3s-agent -monitor -interval 30s

k3s-schedule: build
	@echo "Running K3s agent in scheduling mode..."
	./build/k3s-agent -schedule -pod-name test-app -image nginx:alpine

k3s-capture: build
	@echo "Running K3s agent in capture mode..."
	./build/k3s-agent -pretty

# Development targets
dev: build
	@echo "Running K3s agent in development mode..."
	./build/k3s-agent -pretty

dev-monitor: build
	@echo "Running K3s agent in monitoring mode..."
	./build/k3s-agent -monitor -interval 10s -pretty

dev-schedule: build
	@echo "Running K3s agent in scheduling mode..."
	./build/k3s-agent -schedule -pod-name dev-app -image nginx:latest -cpu 200m -memory 256Mi

# Staging agent targets
staging-monitor: build
	@echo "Running staging agent in monitoring mode..."
	./build/staging-agent -monitor -interval 30s

staging-capture: build
	@echo "Running staging agent in capture mode..."
	./build/staging-agent -pretty

staging-dev: build
	@echo "Running staging agent in development mode..."
	./build/staging-agent -pretty -control-plane-url http://localhost:8080

# Dependencies
deps:
	@echo "Installing dependencies..."
	go mod tidy
	go mod download

# Code formatting
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Linting
lint:
	@echo "Linting code..."
	golangci-lint run

# Generate report
report: build
	@echo "Generating K3s system report..."
	./build/k3s-agent -pretty -output reports/k3s_report_$(shell date +%Y%m%d_%H%M%S).txt
	@echo "Report generated in reports/ directory!"

# K3s installation and setup
install-k3s:
	@echo "Installing K3s..."
	curl -sfL https://get.k3s.io | sh -
	@echo "K3s installed successfully!"

setup-k3s:
	@echo "Setting up K3s cluster..."
	@echo "Starting K3s server..."
	sudo systemctl start k3s
	@echo "Waiting for K3s to be ready..."
	sleep 30
	@echo "Installing metrics server..."
	kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
	@echo "K3s setup completed!"

# K3s cluster management
k3s-status:
	@echo "Checking K3s cluster status..."
	kubectl get nodes
	kubectl get pods --all-namespaces

k3s-logs:
	@echo "Showing K3s logs..."
	sudo journalctl -u k3s -f

# Help
help:
	@echo "Available targets:"
	@echo "  build         - Build K3s agent and unified tool"
	@echo "  clean         - Remove build artifacts"
	@echo "  test          - Run tests"
	@echo "  run           - Build and run K3s agent"
	@echo "  k3s-agent     - Build and run K3s agent"
	@echo "  unified       - Build and run unified tool"
	@echo "  k3s-monitor   - Run K3s agent in monitoring mode"
	@echo "  k3s-schedule  - Run K3s agent in scheduling mode"
	@echo "  k3s-capture   - Run K3s agent in capture mode"
	@echo "  dev           - Build and run K3s agent in dev mode"
	@echo "  dev-monitor   - Build and run K3s agent in monitoring mode"
	@echo "  dev-schedule  - Build and run K3s agent in scheduling mode"
	@echo "  report        - Generate a new K3s system report"
	@echo "  deps          - Install dependencies"
	@echo "  fmt           - Format code"
	@echo "  lint          - Lint code"
	@echo "  install-k3s   - Install K3s"
	@echo "  setup-k3s     - Setup K3s cluster"
	@echo "  k3s-status    - Check K3s cluster status"
	@echo "  k3s-logs      - Show K3s logs"
	@echo "  staging-agent - Build and run staging agent"
	@echo "  staging-monitor - Run staging agent in monitoring mode"
	@echo "  staging-capture - Run staging agent in capture mode"
	@echo "  staging-dev    - Run staging agent in development mode"
	@echo "  help          - Show this help" 