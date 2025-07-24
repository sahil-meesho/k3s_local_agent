# External Server Integration Guide

## 🌐 How External Servers Can Call This Folder

### **Current System Status**
- **HTTP Proxy Server**: Running on port 8081
- **Staging Agent**: Running on port 8082
- **Status**: Ready for external server communication

---

## 📡 **Available Endpoints (No Deployment Required)**

### **1. Health Check Endpoints**

#### **HTTP Proxy Health**
```bash
# Check if HTTP proxy server is running
curl http://YOUR_IP:8081/health

# Response:
{
  "agent_id": "staging-agent-JHMH32WDGT-1753386304",
  "proxies": 0,
  "status": "healthy",
  "timestamp": "2025-07-25T02:12:37.037496+05:30"
}
```

#### **Staging Agent Health**
```bash
# Check if staging agent is running
curl http://YOUR_IP:8082/health

# Response:
{
  "agent_id": "staging-agent-JHMH32WDGT-1753386246",
  "pod_count": 0,
  "status": "healthy",
  "timestamp": "2025-07-25T02:12:40.385969+05:30"
}
```

### **2. Proxy Status Endpoints**

#### **Get All Proxy Status**
```bash
# Get current proxy status
curl http://YOUR_IP:8081/api/proxies

# Response:
{
  "active_proxies": 0,
  "failed_proxies": 0,
  "proxies": {},
  "timestamp": "2025-07-25T02:12:11.068797+05:30",
  "total_proxies": 0
}
```

### **3. Agent Status Endpoints**

#### **Get Agent Status**
```bash
# Get detailed agent status
curl http://YOUR_IP:8082/api/status

# Response includes:
# - Agent ID
# - Pod count
# - System status
# - Timestamp
```

---

## 🔧 **External Server Integration Examples**

### **Python Integration**
```python
import requests
import json

# Base URLs
PROXY_BASE = "http://YOUR_IP:8081"
AGENT_BASE = "http://YOUR_IP:8082"

# Health checks
def check_system_health():
    try:
        proxy_health = requests.get(f"{PROXY_BASE}/health").json()
        agent_health = requests.get(f"{AGENT_BASE}/health").json()
        
        return {
            "proxy_healthy": proxy_health["status"] == "healthy",
            "agent_healthy": agent_health["status"] == "healthy",
            "proxy_agent_id": proxy_health["agent_id"],
            "agent_agent_id": agent_health["agent_id"]
        }
    except Exception as e:
        return {"error": str(e)}

# Get proxy status
def get_proxy_status():
    try:
        response = requests.get(f"{PROXY_BASE}/api/proxies")
        return response.json()
    except Exception as e:
        return {"error": str(e)}

# Usage
health = check_system_health()
print(f"System Health: {health}")

proxy_status = get_proxy_status()
print(f"Proxy Status: {proxy_status}")
```

### **Node.js Integration**
```javascript
const axios = require('axios');

// Base URLs
const PROXY_BASE = 'http://YOUR_IP:8081';
const AGENT_BASE = 'http://YOUR_IP:8082';

// Health checks
async function checkSystemHealth() {
    try {
        const [proxyHealth, agentHealth] = await Promise.all([
            axios.get(`${PROXY_BASE}/health`),
            axios.get(`${AGENT_BASE}/health`)
        ]);
        
        return {
            proxy_healthy: proxyHealth.data.status === 'healthy',
            agent_healthy: agentHealth.data.status === 'healthy',
            proxy_agent_id: proxyHealth.data.agent_id,
            agent_agent_id: agentHealth.data.agent_id
        };
    } catch (error) {
        return { error: error.message };
    }
}

// Get proxy status
async function getProxyStatus() {
    try {
        const response = await axios.get(`${PROXY_BASE}/api/proxies`);
        return response.data;
    } catch (error) {
        return { error: error.message };
    }
}

// Usage
checkSystemHealth().then(health => {
    console.log('System Health:', health);
});

getProxyStatus().then(status => {
    console.log('Proxy Status:', status);
});
```

### **Bash/Shell Integration**
```bash
#!/bin/bash

# Configuration
PROXY_BASE="http://YOUR_IP:8081"
AGENT_BASE="http://YOUR_IP:8082"

# Health check function
check_health() {
    echo "Checking system health..."
    
    # Check proxy health
    if curl -s "${PROXY_BASE}/health" > /dev/null; then
        echo "✅ HTTP Proxy Server: HEALTHY"
        curl -s "${PROXY_BASE}/health" | jq .
    else
        echo "❌ HTTP Proxy Server: UNHEALTHY"
    fi
    
    # Check agent health
    if curl -s "${AGENT_BASE}/health" > /dev/null; then
        echo "✅ Staging Agent: HEALTHY"
        curl -s "${AGENT_BASE}/health" | jq .
    else
        echo "❌ Staging Agent: UNHEALTHY"
    fi
}

# Get proxy status
get_proxy_status() {
    echo "Getting proxy status..."
    curl -s "${PROXY_BASE}/api/proxies" | jq .
}

# Usage
check_health
get_proxy_status
```

---

## 🚀 **External Server Use Cases**

### **1. Monitoring & Health Checks**
- External monitoring systems can ping health endpoints
- Load balancers can use health checks for routing
- DevOps tools can monitor system status

### **2. Status Reporting**
- External dashboards can display proxy status
- Alerting systems can monitor proxy health
- Reporting tools can collect status data

### **3. Integration Points**
- CI/CD pipelines can check system readiness
- Deployment tools can verify system health
- Management consoles can display status

### **4. API Integration**
- External APIs can query system status
- Web applications can display proxy information
- Mobile apps can check system health

---

## 🔒 **Security Considerations**

### **Network Access**
- Ensure firewall allows access to ports 8081 and 8082
- Consider using HTTPS for production
- Implement authentication if needed

### **Rate Limiting**
- Current system has built-in rate limiting
- Monitor for excessive requests
- Implement additional rate limiting if needed

### **Authentication**
- Current endpoints are public (for health checks)
- Add authentication for sensitive operations
- Use API keys for external server access

---

## 📊 **Response Format**

All endpoints return JSON responses with:
- **status**: System status ("healthy", "unhealthy")
- **agent_id**: Unique agent identifier
- **timestamp**: ISO 8601 timestamp
- **Additional fields**: Varies by endpoint

---

## 🎯 **Next Steps**

1. **Replace `YOUR_IP`** with your actual server IP
2. **Test endpoints** from external servers
3. **Implement monitoring** in your external systems
4. **Add authentication** if needed for production
5. **Set up alerts** for system health

---

## ✅ **Current Status**

Your system is **ready for external server integration** without requiring any staging pod deployments. The HTTP proxy server and staging agent are both running and accessible via their respective endpoints. 