#!/bin/bash

# Test script for Cloudflare tunnel setup

echo "Starting test HTTP server on port 8082..."

# Start a simple HTTP server in the background
python3 -m http.server 8082 &
SERVER_PID=$!

echo "HTTP server started with PID: $SERVER_PID"

# Wait a moment for server to start
sleep 2

# Test the tunnel
echo "Testing tunnel setup..."
echo "You can now test the tunnel by visiting:"
echo "1. Check if tunnel is working: curl -I http://localhost:8082"
echo "2. Once tunnel is established, visit the tunnel URL"

# Keep the script running
echo "Press Ctrl+C to stop the test server"
trap "echo 'Stopping test server...'; kill $SERVER_PID; exit" INT

# Wait for user to stop
wait 