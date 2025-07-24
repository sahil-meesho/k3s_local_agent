#!/bin/bash

# K3s Installation and Setup Script
# This script installs K3s and sets up the cluster for the local agent

set -e

echo "=== K3s Local Agent Installation Script ==="
echo "This script will install K3s and configure it for local agent monitoring"
echo

# Check if running as root
if [[ $EUID -eq 0 ]]; then
   echo "This script should not be run as root"
   exit 1
fi

# Check if K3s is already installed
if command -v k3s &> /dev/null; then
    echo "K3s is already installed. Checking status..."
    if sudo systemctl is-active --quiet k3s; then
        echo "K3s is running. Skipping installation."
    else
        echo "K3s is installed but not running. Starting K3s..."
        sudo systemctl start k3s
    fi
else
    echo "Installing K3s..."
    curl -sfL https://get.k3s.io | sh -
    
    echo "Starting K3s service..."
    sudo systemctl enable k3s
    sudo systemctl start k3s
    
    echo "Waiting for K3s to be ready..."
    sleep 30
fi

# Check if kubectl is available
if ! command -v kubectl &> /dev/null; then
    echo "Installing kubectl..."
    # For macOS
    if [[ "$OSTYPE" == "darwin"* ]]; then
        brew install kubectl
    else
        # For Linux
        curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
        chmod +x kubectl
        sudo mv kubectl /usr/local/bin/
    fi
fi

# Configure kubectl to use K3s
echo "Configuring kubectl for K3s..."
mkdir -p ~/.kube
sudo cp /etc/rancher/k3s/k3s.yaml ~/.kube/config
sudo chown $USER:$USER ~/.kube/config

# Wait for cluster to be ready
echo "Waiting for cluster to be ready..."
kubectl wait --for=condition=Ready nodes --all --timeout=300s

# Install metrics server
echo "Installing metrics server..."
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml

# Wait for metrics server to be ready
echo "Waiting for metrics server to be ready..."
kubectl wait --for=condition=Ready pod -l k8s-app=metrics-server -n kube-system --timeout=300s

# Create namespace for the local agent
echo "Creating namespace for local agent..."
kubectl create namespace local-agent --dry-run=client -o yaml | kubectl apply -f -

# Show cluster status
echo
echo "=== Cluster Status ==="
kubectl get nodes
echo
echo "=== Pods in kube-system ==="
kubectl get pods -n kube-system
echo
echo "=== Metrics Server Status ==="
kubectl get pods -n kube-system -l k8s-app=metrics-server

echo
echo "=== K3s Installation Complete ==="
echo "Cluster is ready for local agent monitoring!"
echo
echo "Next steps:"
echo "1. Run: make build"
echo "2. Run: make k3s-agent"
echo "3. Run: make k3s-monitor (for continuous monitoring)"
echo "4. Run: make k3s-schedule (to test pod scheduling)"
echo
echo "Useful commands:"
echo "- Check cluster status: make k3s-status"
echo "- View K3s logs: make k3s-logs"
echo "- Generate report: make report" 