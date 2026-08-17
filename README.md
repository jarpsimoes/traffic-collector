# Traffic Collector

A lightweight, production-grade eBPF-based network traffic collector for Kubernetes clusters. Collects and exports traces to Grafana Tempo and Jaeger with minimal resource footprint.

## Features

- **eBPF-based Traffic Capture**: Low-overhead network packet inspection using kernel-level eBPF programs
- **Dual Exporter Support**: Compatible with both Grafana Tempo (OTLP) and Jaeger
- **Pod Metadata Enrichment**: Automatically enriches traces with pod namespace, name, and node information
- **IPv4 & IPv6 Support**: Handles both protocol versions seamlessly
- **Minimal Footprint**: Go-based collector with ~20-40MB memory usage per DaemonSet instance
- **Production Ready**: Health checks, graceful shutdown, comprehensive error handling
- **Kubernetes Native**: DaemonSet deployment with RBAC, tolerations, and node selectors

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                        │
├─────────────────────────────────────────────────────────────┤
│  Node 1              │  Node 2              │  Node 3        │
│ ┌─────────────────┐ │ ┌─────────────────┐ │ ┌────────────┐ │
│ │ Traffic         │ │ │ Traffic         │ │ │ Traffic    │ │
│ │ Collector Pod   │ │ │ Collector Pod   │ │ │ Collector  │ │
│ │ (DaemonSet)     │ │ │ (DaemonSet)     │ │ │ Pod        │ │
│ │                 │ │ │                 │ │ │            │ │
│ │ eBPF Program    │ │ │ eBPF Program    │ │ │ eBPF       │ │
│ └────────┬────────┘ │ └────────┬────────┘ │ └─────┬──────┘ │
│          │          │          │          │       │         │
└──────────┼──────────┴──────────┼──────────┴───────┼─────────┘
           │                     │                   │
           │                     ▼                   │
           │            ┌─────────────────┐         │
           │            │  Tempo Cluster  │         │
           │            │  (OTLP Endpoint)│         │
           │            └─────────────────┘         │
           │                                        │
           └────────────────┬───────────────────────┘
                            ▼
                    ┌──────────────────┐
                    │  Jaeger Collector│
                    └──────────────────┘
```

## Quick Start

### Prerequisites

- Kubernetes 1.16+
- Helm 3.0+
- Grafana Tempo or Jaeger backend (optional for deployment, required for trace collection)

### Installation

1. **Clone the repository**:
```bash
git clone https://github.com/jarpsimoes/traffic-collector.git
cd traffic-collector
```

2. **Build the Docker image**:
```bash
make docker-build REGISTRY=your-registry VERSION=latest
```

3. **Deploy with Helm**:
```bash
helm install traffic-collector ./charts/traffic-collector \
  --namespace monitoring \
  --set image.repository=your-registry/traffic-collector \
  --set tempo.endpoint=http://tempo:4317 \
  --set jaeger.endpoint=http://jaeger-collector:14268
```

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `NODE_NAME` | Kubernetes API | Node name from downward API |
| `TEMPO_ENDPOINT` | `http://tempo:4317` | Grafana Tempo OTLP gRPC endpoint |
| `JAEGER_ENDPOINT` | `http://jaeger-collector:14268` | Jaeger collector HTTP endpoint |
| `LOG_LEVEL` | `info` | Logging level (debug, info, warn, error) |
| `HEALTH_CHECK_PORT` | `8080` | Health check HTTP port |

### Helm Values

See `charts/traffic-collector/values.yaml` for all available configuration options.

Key values:
- `image.repository`: Docker image repository
- `image.tag`: Docker image tag
- `resources.limits`: CPU and memory limits
- `resources.requests`: CPU and memory requests
- `nodeSelector`: Node selection criteria
- `tolerations`: Node tolerations
- `tempo.enabled`: Enable Tempo exporter
- `tempo.endpoint`: Tempo OTLP endpoint
- `jaeger.enabled`: Enable Jaeger exporter
- `jaeger.endpoint`: Jaeger collector endpoint

## Development

### Project Structure

```
traffic-collector/
├── cmd/
│   └── collector/
│       └── main.go           # Main application entry point
├── pkg/
│   ├── collector/
│   │   └── collector.go      # Core collector logic
│   ├── ebpf/
│   │   ├── program.c         # eBPF kernel program
│   │   └── loader.go         # eBPF program loader
│   ├── exporter/
│   │   ├── tempo.go          # Tempo/OTLP exporter
│   │   └── jaeger.go         # Jaeger exporter
│   └── k8s/
│       └── metadata.go       # Kubernetes metadata enrichment
├── charts/
│   └── traffic-collector/    # Helm chart
├── Dockerfile                # Multi-stage Docker build
├── Makefile                  # Build automation
├── go.mod                    # Go module definition
└── go.sum                    # Go dependencies
```

### Building Locally

```bash
# Build binary
make build

# Run with debug logging
make run

# Build Docker image
make docker-build

# Run tests
make test

# Format code
make fmt

# Lint code
make lint
```

### eBPF Program Development

The eBPF program is written in C and compiled to bytecode. Modify `pkg/ebpf/program.c` and rebuild:

```bash
make ebpf-build
make build
```

## Deployment Examples

### Development Environment

```bash
helm install traffic-collector ./charts/traffic-collector \
  -f charts/traffic-collector/values-dev.yaml \
  --namespace monitoring
```

### Production with Tempo

```bash
helm install traffic-collector ./charts/traffic-collector \
  -f charts/traffic-collector/values-prod.yaml \
  --set tempo.endpoint=tempo.observability.svc.cluster.local:4317 \
  --namespace monitoring
```

### Production with Jaeger

```bash
helm install traffic-collector ./charts/traffic-collector \
  -f charts/traffic-collector/values-prod.yaml \
  --set jaeger.endpoint=http://jaeger-collector.observability:14268 \
  --namespace monitoring
```

## Performance

- **Memory Usage**: ~20-40 MB per node (DaemonSet instance)
- **CPU Usage**: <1% typical load
- **Network Overhead**: Minimal, batched OTLP exports
- **Kernel Impact**: Negligible with eBPF (no syscall overhead)

## Monitoring & Health Checks

The collector exposes health check endpoint on port 8080:

```bash
curl http://pod-ip:8080/health
```

Response:
```json
{
  "status": "healthy",
  "uptime": "2h30m15s",
  "traces_exported": 15234,
  "errors": 0
}
```

## Troubleshooting

### Collector pod not starting

Check logs:
```bash
kubectl logs -n monitoring -l app=traffic-collector
```

### No traces appearing in Tempo/Jaeger

1. Verify exporter endpoints are reachable:
```bash
kubectl exec -n monitoring <pod-name> -- curl http://tempo:4317
```

2. Check collector permissions:
```bash
kubectl describe clusterrole traffic-collector
```

3. Verify eBPF program loaded:
```bash
kubectl logs -n monitoring <pod-name> | grep "eBPF program loaded"
```

### High memory usage

Increase batch size or reduce trace sampling in values.yaml

## Contributing

Contributions are welcome! Please:
1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Submit a pull request

## License

MIT License - see LICENSE file for details

## Support

For issues, questions, or suggestions, please open an issue on GitHub.

---

**Built for Kubernetes observability at scale.**
