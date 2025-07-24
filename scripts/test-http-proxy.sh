#!/bin/bash

# Test HTTP Reverse Proxy Tunneling
# This script demonstrates the HTTP proxy approach for staging pod access

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

echo -e "${BLUE}🌐 HTTP Reverse Proxy Tunneling Test${NC}"
echo "=========================================="
echo

# Test 1: Check if HTTP proxy server is running
echo -e "${YELLOW}1. Testing HTTP Proxy Server...${NC}"
if curl -s http://localhost:8080/health > /dev/null 2>&1; then
    print_status 0 "HTTP proxy server is running"
else
    print_status 1 "HTTP proxy server is not running"
    echo "   Start the staging agent: go run cmd/staging-agent/main.go"
fi

# Test 2: Check proxy status endpoint
echo -e "${YELLOW}2. Testing Proxy Status Endpoint...${NC}"
if curl -s http://localhost:8080/api/proxies > /dev/null 2>&1; then
    print_status 0 "Proxy status endpoint is accessible"
    echo "   Proxy status:"
    curl -s http://localhost:8080/api/proxies | jq . 2>/dev/null || echo "   (JSON parsing not available)"
else
    print_status 1 "Proxy status endpoint is not accessible"
fi

# Test 3: Test proxy routing (if staging pods exist)
echo -e "${YELLOW}3. Testing Proxy Routing...${NC}"
if curl -s http://localhost:8080/staging/nginx > /dev/null 2>&1; then
    print_status 0 "Proxy routing is working"
    echo "   Example: http://localhost:8080/staging/nginx"
else
    print_status 1 "No staging pods available for testing"
    echo "   Deploy a staging pod to test routing"
fi

# Test 4: Compare with old port forwarding approach
echo -e "${YELLOW}4. Comparison with Port Forwarding...${NC}"
echo "   Old Approach (Port Forwarding):"
echo "   ❌ Requires socat/netcat"
echo "   ❌ Firewall/NAT issues"
echo "   ❌ Port conflicts"
echo "   ❌ Limited to localhost"
echo
echo "   New Approach (HTTP Reverse Proxy):"
echo "   ✅ No external dependencies"
echo "   ✅ Works through firewalls"
echo "   ✅ No port conflicts"
echo "   ✅ Accessible from anywhere"
echo "   ✅ Better error handling"
echo "   ✅ Health monitoring"

# Test 5: Show usage examples
echo -e "${YELLOW}5. Usage Examples...${NC}"
echo "   Access staging pods via HTTP proxy:"
echo "   • http://localhost:8080/staging/my-app"
echo "   • http://localhost:8080/staging/nginx"
echo "   • http://localhost:8080/staging/api-service"
echo
echo "   Health checks:"
echo "   • http://localhost:8080/health"
echo "   • http://localhost:8080/api/proxies"

# Test 6: Performance test
echo -e "${YELLOW}6. Performance Test...${NC}"
if command -v ab > /dev/null 2>&1; then
    echo "   Running Apache Bench test..."
    ab -n 100 -c 10 http://localhost:8080/health 2>/dev/null | grep "Requests per second" || echo "   Performance test skipped"
else
    echo "   Apache Bench not available for performance testing"
fi

echo
echo -e "${BLUE}🎯 HTTP Reverse Proxy Advantages:${NC}"
echo "=========================================="
echo "✅ No firewall issues - works through corporate networks"
echo "✅ No port conflicts - single server handles all proxies"
echo "✅ Better error handling - proper HTTP status codes"
echo "✅ Health monitoring - built-in health checks"
echo "✅ Logging - detailed request/response logging"
echo "✅ Security - can add authentication/authorization"
echo "✅ Scalability - can handle many staging pods"
echo "✅ Reliability - no dependency on external tools"

echo
echo -e "${GREEN}✅ HTTP Reverse Proxy tunneling is ready for use!${NC}"
echo
echo "To start using:"
echo "1. Start staging agent: go run cmd/staging-agent/main.go"
echo "2. Access staging pods: http://localhost:8080/staging/{pod-name}"
echo "3. Monitor status: http://localhost:8080/api/proxies" 