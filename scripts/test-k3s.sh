#!/bin/bash

# K3s Integration Test Script
# This script tests the K3s local agent functionality

set -e

echo "=== K3s Local Agent Test Script ==="
echo "Testing K3s integration and functionality"
echo

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    local status=$1
    local message=$2
    if [ "$status" = "PASS" ]; then
        echo -e "${GREEN}✓ PASS${NC}: $message"
    elif [ "$status" = "FAIL" ]; then
        echo -e "${RED}✗ FAIL${NC}: $message"
    elif [ "$status" = "INFO" ]; then
        echo -e "${YELLOW}ℹ INFO${NC}: $message"
    fi
}

# Function to check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Function to check if service is running
service_running() {
    systemctl is-active --quiet "$1" 2>/dev/null
}

echo "1. Checking prerequisites..."

# Check if Go is installed
if command_exists go; then
    print_status "PASS" "Go is installed"
else
    print_status "FAIL" "Go is not installed"
    exit 1
fi

# Check if K3s is installed
if command_exists k3s; then
    print_status "PASS" "K3s is installed"
else
    print_status "FAIL" "K3s is not installed. Run ./scripts/install-k3s.sh first"
    exit 1
fi

# Check if kubectl is installed
if command_exists kubectl; then
    print_status "PASS" "kubectl is installed"
else
    print_status "FAIL" "kubectl is not installed"
    exit 1
fi

echo
echo "2. Checking K3s cluster status..."

# Check if K3s is running
if service_running k3s; then
    print_status "PASS" "K3s service is running"
else
    print_status "FAIL" "K3s service is not running"
    echo "Starting K3s..."
    sudo systemctl start k3s
    sleep 10
fi

# Check cluster connectivity
if kubectl cluster-info >/dev/null 2>&1; then
    print_status "PASS" "K3s cluster is accessible"
else
    print_status "FAIL" "Cannot connect to K3s cluster"
    exit 1
fi

# Check nodes
NODE_COUNT=$(kubectl get nodes --no-headers | wc -l)
if [ "$NODE_COUNT" -gt 0 ]; then
    print_status "PASS" "Found $NODE_COUNT node(s) in cluster"
else
    print_status "FAIL" "No nodes found in cluster"
    exit 1
fi

echo
echo "3. Checking metrics server..."

# Check if metrics server is installed
if kubectl get pods -n kube-system -l k8s-app=metrics-server --no-headers | grep -q Running; then
    print_status "PASS" "Metrics server is running"
else
    print_status "INFO" "Metrics server not found, installing..."
    kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
    sleep 30
    if kubectl get pods -n kube-system -l k8s-app=metrics-server --no-headers | grep -q Running; then
        print_status "PASS" "Metrics server installed and running"
    else
        print_status "FAIL" "Failed to install metrics server"
    fi
fi

echo
echo "4. Building K3s local agent..."

# Build the application
if make build >/dev/null 2>&1; then
    print_status "PASS" "K3s local agent built successfully"
else
    print_status "FAIL" "Failed to build K3s local agent"
    exit 1
fi

echo
echo "5. Testing K3s agent functionality..."

# Test capture mode
print_status "INFO" "Testing capture mode..."
if timeout 30s ./build/k3s-agent -output /tmp/test_capture.txt >/dev/null 2>&1; then
    print_status "PASS" "Capture mode test passed"
else
    print_status "FAIL" "Capture mode test failed"
fi

# Test scheduling mode (if cluster is ready)
print_status "INFO" "Testing scheduling mode..."
if timeout 60s ./build/k3s-agent -schedule -pod-name test-pod-$(date +%s) -image nginx:alpine -cpu 50m -memory 64Mi >/dev/null 2>&1; then
    print_status "PASS" "Scheduling mode test passed"
else
    print_status "FAIL" "Scheduling mode test failed"
fi

echo
echo "6. Checking generated reports..."

# Check if reports were generated
if [ -f /tmp/test_capture.txt ]; then
    print_status "PASS" "Report file generated"
    echo "Report preview:"
    head -20 /tmp/test_capture.txt
else
    print_status "FAIL" "No report file generated"
fi

echo
echo "7. Testing cluster metrics..."

# Test metrics collection
if kubectl top nodes >/dev/null 2>&1; then
    print_status "PASS" "Node metrics collection working"
else
    print_status "FAIL" "Node metrics collection failed"
fi

if kubectl top pods --all-namespaces >/dev/null 2>&1; then
    print_status "PASS" "Pod metrics collection working"
else
    print_status "FAIL" "Pod metrics collection failed"
fi

echo
echo "8. Final cluster status check..."

# Show final cluster status
echo "Current cluster status:"
kubectl get nodes
echo
echo "Pods in default namespace:"
kubectl get pods -n default
echo
echo "Pods in kube-system namespace:"
kubectl get pods -n kube-system

echo
echo "=== Test Summary ==="
print_status "INFO" "K3s local agent integration test completed"
print_status "INFO" "Check the generated reports in /tmp/test_capture.txt"
print_status "INFO" "Run 'make k3s-monitor' for continuous monitoring"
print_status "INFO" "Run 'make k3s-schedule' to test pod scheduling"

# Cleanup test pod
kubectl delete pod test-pod-* -n default --ignore-not-found=true >/dev/null 2>&1

echo
echo "Test completed successfully! 🎉" 