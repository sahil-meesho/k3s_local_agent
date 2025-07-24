#!/bin/bash

# Auto-register laptop public endpoint script
# This script monitors cloudflared logs and automatically registers the laptop's public endpoint

CONTROL_PLANE_URL="https://7ab044fdb22100.lhr.life"
LOG_FILE="logs/cloudflared.log"
REGISTRATION_ENDPOINT="/api/v1/register-local-agent"

echo "Starting auto-registration script..."
echo "Monitoring cloudflared logs for new tunnel URLs..."

# Function to register the laptop's public endpoint
register_endpoint() {
    local tunnel_url="$1"
    echo "Registering laptop endpoint: $tunnel_url"
    
    # Send registration request to control plane
    response=$(curl -s -X POST "$CONTROL_PLANE_URL$REGISTRATION_ENDPOINT" \
        -H "Content-Type: application/json" \
        -d "{\"host\": \"$tunnel_url\"}")
    
    if [[ $? -eq 0 ]]; then
        echo "Registration successful: $response"
        echo "$(date): Registered $tunnel_url" >> logs/registration_history.log
    else
        echo "Registration failed for $tunnel_url"
        echo "$(date): Failed to register $tunnel_url" >> logs/registration_history.log
    fi
}

# Monitor the log file for new tunnel URLs
tail -f "$LOG_FILE" | while read line; do
    if echo "$line" | grep -q "Your quick Tunnel has been created"; then
        # Get the next line which contains the URL
        read url_line
        tunnel_url=$(echo "$url_line" | grep -o 'https://[^[:space:]]*\.trycloudflare\.com')
        
        if [[ -n "$tunnel_url" ]]; then
            echo "New tunnel detected: $tunnel_url"
            register_endpoint "$tunnel_url"
        fi
    fi
done 