# K3s Local Agent

A comprehensive local agent system that integrates with K3s for resource monitoring and intelligent pod scheduling. This system combines local machine monitoring with K3s cluster management to provide real-time resource visibility and automated workload scheduling.

## Features

### 🔍 **Resource Monitoring**
- **Local System Monitoring**: CPU, memory, disk, and network usage
- **K3s Cluster Monitoring**: Node metrics, pod metrics, and cluster health
- **Real-time Metrics**: Continuous monitoring with configurable intervals
- **Health Checks**: System and network connectivity monitoring

### 🚀 **Intelligent Pod Scheduling**
- **Resource-aware Scheduling**: Automatically selects the best node based on available resources
- **Load Balancing**: Distributes workloads across healthy nodes
- **Capacity Planning**: Monitors resource availability and prevents overloading
- **Scheduling Decisions**: Provides detailed reasoning for pod placement

### 📊 **Comprehensive Reporting**
- **Human-readable Reports**: Detailed system and cluster summaries
- **JSON Export**: Machine-readable data for integration
- **Historical Data**: Track resource usage over time
- **Scheduling Analytics**: Monitor pod placement decisions

## Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Local Agent   │    │   K3s Client    │    │   K3s Cluster   │
│                 │    │                 │    │                 │
│ • System Monitor│◄──►│ • Node Metrics  │◄──►│ • Control Plane │
│ • Health Checks │    │ • Pod Metrics   │    │ • Worker Nodes  │
│ • Resource Data │    │ • Scheduling    │    │ • Metrics Server│
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

## How It Works

### 1. **Resource Monitoring**
The system continuously monitors both local machine resources and K3s cluster metrics:

- **Local Monitoring**: CPU usage, memory consumption, disk I/O, network status
- **Cluster Monitoring**: Node availability, pod resource usage, cluster health
- **Metrics Collection**: Real-time data from metrics-server and local system calls

### 2. **Intelligent Scheduling**
When scheduling pods, the system:

1. **Analyzes Available Resources**: Checks CPU and memory availability across all nodes
2. **Evaluates Node Health**: Considers node readiness and current load
3. **Calculates Best Fit**: Uses scoring algorithm to find optimal placement
4. **Executes Scheduling**: Creates pods with proper resource requests and limits

### 3. **Load Distribution**
- **Resource Pressure Detection**: Avoids scheduling on overloaded nodes
- **Health-based Routing**: Prioritizes healthy nodes with available capacity
- **Automatic Failover**: Distributes workloads when nodes become unavailable

## Installation

### Prerequisites
- Go 1.21 or later
- K3s (will be installed automatically)
- kubectl (will be installed automatically)

### Quick Start

1. **Clone the repository**:
   ```bash
   git clone <repository-url>
   cd k3s_local_agent
   ```

2. **Install K3s and setup cluster**:
   ```bash
   ./scripts/install-k3s.sh
   ```

3. **Build the application**:
   ```bash
   make build
   ```

4. **Run the K3s agent**:
   ```bash
   make k3s-agent
   ```

## Usage

### Basic Commands

#### **Capture Mode** (Single snapshot)
```bash
# Capture current system and cluster state
make k3s-capture

# With pretty JSON output
./build/k3s-agent -pretty
```

#### **Monitoring Mode** (Continuous monitoring)
```bash
# Monitor with 30-second intervals
make k3s-monitor

# Monitor with custom interval
./build/k3s-agent -monitor -interval 10s
```

#### **Scheduling Mode** (Test pod scheduling)
```bash
# Schedule a test pod
make k3s-schedule

# Schedule with custom resources
./build/k3s-agent -schedule -pod-name my-app -image nginx:latest -cpu 500m -memory 512Mi
```

### Advanced Usage

#### **Custom Namespace**
```bash
./build/k3s-agent -namespace my-namespace -pretty
```

#### **Custom Output Location**
```bash
./build/k3s-agent -output /path/to/report.txt -log /path/to/agent.log
```

#### **Development Mode**
```bash
# Development with detailed logging
make dev

# Development monitoring
make dev-monitor

# Development scheduling
make dev-schedule
```

## Configuration

### Environment Variables
- `K3S_NAMESPACE`: Default namespace for pod scheduling (default: "default")
- `K3S_LOG_LEVEL`: Logging level (default: "info")

### Configuration File
The system uses `config/config.yaml` for application settings:

```yaml
monitor:
  interval: 30s
  enabled: true

k3s:
  namespace: default
  metrics_enabled: true

logging:
  level: info
  format: json
```

## Monitoring Features

### **Local System Metrics**
- CPU usage percentage and core count
- Memory usage (total, available, used, free)
- Disk I/O and space utilization
- Network connectivity and VPN status
- System health and uptime

### **K3s Cluster Metrics**
- Node availability and readiness
- Pod resource consumption
- Cluster health percentage
- Resource allocation and capacity
- Scheduling decisions and reasoning

### **Health Monitoring**
- Network connectivity checks
- Internet connectivity verification
- VPN connection status
- System health indicators

## Scheduling Algorithm

The system uses a sophisticated scheduling algorithm that considers:

1. **Resource Availability**: CPU and memory capacity on each node
2. **Current Load**: Existing pod resource usage
3. **Node Health**: Readiness and condition status
4. **Scoring**: Calculates optimal placement based on available resources
5. **Constraints**: Respects node selectors and affinity rules

### Scheduling Decision Process
```
1. Get current node metrics
2. Filter nodes with sufficient resources
3. Calculate resource availability score
4. Select node with highest score
5. Create pod with resource requests
6. Monitor pod placement success
```

## Reports and Output

### **Human-readable Reports**
```
=== K3s Resource Summary ===
Timestamp: 2024-01-15 10:30:45

Local System:
  Hostname: my-machine
  CPU Usage: 25.50%
  Memory Usage: 45.20%
  Online: true
  Internet: true

Cluster Health:
  Total Nodes: 3
  Ready Nodes: 3
  Total Pods: 12
  Running Pods: 12
  Node Health: 100.00%
  Pod Health: 100.00%

Node Metrics:
  k3s-node-1:
    CPU Available: 2000m / 4000m
    Memory Available: 4Gi / 8Gi
```

### **JSON Export**
```json
{
  "local_system": {
    "system": {
      "hostname": "my-machine",
      "platform": "darwin",
      "os": "macOS"
    },
    "cpu": {
      "usage_percent": 25.5,
      "core_count": 8
    },
    "memory": {
      "used_percent": 45.2,
      "total": 17179869184
    }
  },
  "cluster_health": {
    "total_nodes": 3,
    "ready_nodes": 3,
    "node_health": 100.0
  },
  "node_metrics": [
    {
      "name": "k3s-node-1",
      "cpu_available": "2000m",
      "memory_available": "4Gi"
    }
  ]
}
```

## Troubleshooting

### **Common Issues**

#### K3s Connection Issues
```bash
# Check K3s status
make k3s-status

# View K3s logs
make k3s-logs

# Restart K3s
sudo systemctl restart k3s
```

#### Metrics Server Issues
```bash
# Check metrics server status
kubectl get pods -n kube-system -l k8s-app=metrics-server

# Reinstall metrics server
kubectl delete -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
```

#### Permission Issues
```bash
# Fix kubectl permissions
sudo cp /etc/rancher/k3s/k3s.yaml ~/.kube/config
sudo chown $USER:$USER ~/.kube/config
```

### **Debug Mode**
```bash
# Run with debug logging
./build/k3s-agent -log-level debug

# Check cluster connectivity
kubectl cluster-info
```

## Development

### **Building from Source**
```bash
# Install dependencies
make deps

# Build application
make build

# Run tests
make test

# Format code
make fmt

# Lint code
make lint
```

### **Adding New Features**
1. **Resource Monitoring**: Add new metrics in `internal/monitor/`
2. **Scheduling Logic**: Modify algorithms in `internal/k3s/`
3. **Reporting**: Extend output formats in `cmd/k3s-agent/`

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Submit a pull request

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Support

For issues and questions:
- Create an issue in the repository
- Check the troubleshooting section
- Review the logs in `logs/` directory

---

**Built with ❤️ for K3s and Kubernetes enthusiasts** 