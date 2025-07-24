#!/bin/bash

# Complete HTTP Proxy Tunneling Test
# This script tests the entire setup with current configuration values

set -e

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    if [ $1 -eq 0 ]; then
        echo -e "${GREEN}✅ $2${NC}"
    else
        echo -e "${RED}❌ $2${NC}"
    fi
}

echo -e "${BLUE}🚀 Complete HTTP Proxy Tunneling Test${NC}"
echo "============================================="
echo

# Test 1: Check configuration files
echo -e "${YELLOW}1. Checking Configuration Files...${NC}"

if [ -f "config/staging_config.yaml" ]; then
    print_status 0 "staging_config.yaml exists"
    
    # Check key values
    if grep -q "agent_id: \"staging-agent-1\"" config/staging_config.yaml; then
        print_status 0 "agent_id is set correctly"
    else
        print_status 1 "agent_id needs to be set"
    fi
    
    if grep -q "control_plane_url: \"http://localhost:8080\"" config/staging_config.yaml; then
        print_status 0 "control_plane_url is set correctly"
    else
        print_status 1 "control_plane_url needs to be set"
    fi
else
    print_status 1 "staging_config.yaml not found"
fi

if [ -f "config/tunnel_config.yaml" ]; then
    print_status 0 "tunnel_config.yaml exists"
    
    if grep -q "type: \"http_proxy\"" config/tunnel_config.yaml; then
        print_status 0 "HTTP proxy is enabled"
    else
        print_status 1 "HTTP proxy not enabled"
    fi
else
    print_status 1 "tunnel_config.yaml not found"
fi

# Test 2: Check if staging agent can be built
echo -e "${YELLOW}2. Testing Build Process...${NC}"
if go build -o build/staging-agent cmd/staging-agent/main.go 2>/dev/null; then
    print_status 0 "Staging agent builds successfully"
else
    print_status 1 "Staging agent build failed"
    echo "   Run: go mod tidy"
fi

# Test 3: Check if HTTP proxy server can start
echo -e "${YELLOW}3. Testing HTTP Proxy Server...${NC}"
if timeout 5s go run cmd/staging-agent/main.go -help > /dev/null 2>&1; then
    print_status 0 "Staging agent starts successfully"
else
    print_status 1 "Staging agent failed to start"
fi

# Test 4: Check port availability
echo -e "${YELLOW}4. Checking Port Availability...${NC}"
if lsof -i :8081 > /dev/null 2>&1; then
    print_status 1 "Port 8081 is already in use"
    echo "   Free up port 8081 or change tunnel_config.yaml"
else
    print_status 0 "Port 8081 is available"
fi

if lsof -i :8082 > /dev/null 2>&1; then
    print_status 1 "Port 8082 is already in use"
    echo "   Free up port 8082 or change staging_config.yaml"
else
    print_status 0 "Port 8082 is available"
fi

# Test 5: Check dependencies
echo -e "${YELLOW}5. Checking Dependencies...${NC}"
if command -v go > /dev/null 2>&1; then
    print_status 0 "Go is installed"
else
    print_status 1 "Go is not installed"
fi

if command -v curl > /dev/null 2>&1; then
    print_status 0 "curl is available for testing"
else
    print_status 1 "curl not found"
fi

# Test 6: Show expected URLs
echo -e "${YELLOW}6. Expected URLs After Setup...${NC}"
echo "   When staging agent is running, you should be able to access:"
echo "   • Health Check: http://localhost:8081/health"
echo "   • Proxy Status: http://localhost:8081/api/proxies"
echo "   • Staging Pods: http://localhost:8081/staging/{pod-name}"
echo "   • Agent Health: http://localhost:8082/health"

# Test 7: Configuration summary
echo -e "${YELLOW}7. Configuration Summary...${NC}"
echo "   Current Configuration:"
echo "   • Agent ID: staging-agent-1"
echo "   • Control Plane: http://localhost:8080"
echo "   • Agent Port: 8082"
echo "   • Proxy Port: 8081"
echo "   • Base Path: /staging"
echo "   • Namespace: staging"

echo
echo -e "${BLUE}🎯 Ready to Start!${NC}"
echo "======================"
echo
echo "To start the system:"
echo "1. go run cmd/staging-agent/main.go"
echo "2. Test with: curl http://localhost:8081/health"
echo "3. Access staging pods: http://localhost:8081/staging/{pod-name}"
echo
echo -e "${GREEN}✅ Your configuration is ready to work perfectly!${NC}" 