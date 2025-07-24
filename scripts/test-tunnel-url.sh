#!/bin/bash

echo "Starting tunnel test..."

# Start cloudflared tunnel and capture output
cloudflared tunnel --url http://localhost:8082 2>&1 | tee tunnel.log &
TUNNEL_PID=$!

echo "Tunnel started with PID: $TUNNEL_PID"

# Wait for tunnel to establish
echo "Waiting for tunnel to establish..."
sleep 10

# Look for the tunnel URL in the log
TUNNEL_URL=$(grep -o 'https://[^[:space:]]*\.trycloudflare\.com' tunnel.log | head -1)

if [ -n "$TUNNEL_URL" ]; then
    echo "Tunnel URL found: $TUNNEL_URL"
    echo "Testing tunnel..."
    curl -I "$TUNNEL_URL" 2>/dev/null | head -5
else
    echo "No tunnel URL found in logs"
    echo "Log contents:"
    cat tunnel.log
fi

# Cleanup
echo "Cleaning up..."
kill $TUNNEL_PID 2>/dev/null
rm -f tunnel.log

echo "Test completed" 