#!/bin/bash

# Single Resource Capture Script for Local Agent

set -e

# Configuration
AGENT_BINARY="./local-agent"
OUTPUT_FILE="./logs/resources_$(date +%Y%m%d_%H%M%S).txt"
LOG_FILE="./logs/capture_single.log"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    if [ $1 -eq 0 ]; then
        echo -e "${GREEN}✅ $2${NC}"
    else
        echo -e "${RED}❌ $2${NC}"
    fi
}

# Function to log messages
log_message() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') - $1" | tee -a "$LOG_FILE"
}

# Create necessary directories
mkdir -p logs

echo "🚀 Starting Single Resource Capture..."
log_message "Single resource capture started"

# Check if agent binary exists
if [ ! -f "$AGENT_BINARY" ]; then
    echo "❌ Agent binary not found. Building..."
    go build -o local-agent main.go
    if [ ! -f "$AGENT_BINARY" ]; then
        echo "❌ Failed to build agent binary"
        exit 1
    fi
    print_status 0 "Agent binary built successfully"
fi

# Start the agent in background
echo "🔧 Starting local agent..."
nohup "$AGENT_BINARY" > "$LOG_FILE" 2>&1 &
AGENT_PID=$!

# Wait for agent to start
echo "⏳ Waiting for agent to start..."
sleep 3

# Check if agent is running
if ! pgrep -f "local-agent" > /dev/null; then
    echo "❌ Agent failed to start"
    exit 1
fi

print_status 0 "Agent started successfully (PID: $AGENT_PID)"

# Wait for agent to be ready
echo "⏳ Waiting for agent to be ready..."
for i in {1..5}; do
    if curl -s http://localhost:8081/health > /dev/null 2>&1; then
        print_status 0 "Agent is ready"
        break
    fi
    if [ $i -eq 5 ]; then
        echo "❌ Agent is not responding"
        kill $AGENT_PID 2>/dev/null || true
        exit 1
    fi
    sleep 1
done

# Create output file header
echo "LOCAL AGENT - SINGLE RESOURCE CAPTURE" > "$OUTPUT_FILE"
echo "=====================================" >> "$OUTPUT_FILE"
echo "Generated: $(date)" >> "$OUTPUT_FILE"
echo "System: $(uname -a)" >> "$OUTPUT_FILE"
echo "Capture Type: Single snapshot" >> "$OUTPUT_FILE"
echo "" >> "$OUTPUT_FILE"

# Capture all resources
echo "📊 Capturing system resources..." >> "$OUTPUT_FILE"
echo "===============================================" >> "$OUTPUT_FILE"
echo "TIMESTAMP: $(date '+%Y-%m-%d %H:%M:%S')" >> "$OUTPUT_FILE"
echo "===============================================" >> "$OUTPUT_FILE"
echo "" >> "$OUTPUT_FILE"

# Capture all resources
echo "ALL RESOURCES:" >> "$OUTPUT_FILE"
curl -s http://localhost:8081/api/resources | jq . >> "$OUTPUT_FILE" 2>/dev/null || echo "Failed to get resources" >> "$OUTPUT_FILE"
echo "" >> "$OUTPUT_FILE"

# Capture individual resources
echo "INDIVIDUAL RESOURCE DETAILS:" >> "$OUTPUT_FILE"
echo "" >> "$OUTPUT_FILE"

# System info
echo "SYSTEM INFORMATION:" >> "$OUTPUT_FILE"
curl -s http://localhost:8081/api/resources/system | jq . >> "$OUTPUT_FILE" 2>/dev/null || echo "Failed to get system info" >> "$OUTPUT_FILE"
echo "" >> "$OUTPUT_FILE"

# CPU info
echo "CPU INFORMATION:" >> "$OUTPUT_FILE"
curl -s http://localhost:8081/api/resources/cpu | jq . >> "$OUTPUT_FILE" 2>/dev/null || echo "Failed to get CPU info" >> "$OUTPUT_FILE"
echo "" >> "$OUTPUT_FILE"

# Memory info
echo "MEMORY INFORMATION:" >> "$OUTPUT_FILE"
curl -s http://localhost:8081/api/resources/memory | jq . >> "$OUTPUT_FILE" 2>/dev/null || echo "Failed to get memory info" >> "$OUTPUT_FILE"
echo "" >> "$OUTPUT_FILE"

# Disk info
echo "DISK INFORMATION:" >> "$OUTPUT_FILE"
curl -s http://localhost:8081/api/resources/disk | jq . >> "$OUTPUT_FILE" 2>/dev/null || echo "Failed to get disk info" >> "$OUTPUT_FILE"
echo "" >> "$OUTPUT_FILE"

# Network info
echo "NETWORK INFORMATION:" >> "$OUTPUT_FILE"
curl -s http://localhost:8081/api/resources/network | jq . >> "$OUTPUT_FILE" 2>/dev/null || echo "Failed to get network info" >> "$OUTPUT_FILE"
echo "" >> "$OUTPUT_FILE"

echo "===============================================" >> "$OUTPUT_FILE"
echo "CAPTURE COMPLETE" >> "$OUTPUT_FILE"
echo "===============================================" >> "$OUTPUT_FILE"

# Stop the agent
echo "🛑 Stopping agent..."
kill $AGENT_PID 2>/dev/null || true
sleep 2

if ! pgrep -f "local-agent" > /dev/null; then
    print_status 0 "Agent stopped successfully"
else
    print_status 1 "Failed to stop agent"
fi

# Display results
echo ""
echo "🎉 Single resource capture completed!"
echo ""
echo "📁 Output Files:"
echo "   📄 Resource Data: $OUTPUT_FILE"
echo "   📋 Agent Logs: $LOG_FILE"
echo ""
echo "💡 View the captured data:"
echo "   cat $OUTPUT_FILE"
echo "   less $OUTPUT_FILE"
echo ""

log_message "Single resource capture completed successfully"
print_status 0 "Single resource capture completed" 