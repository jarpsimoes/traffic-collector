# Traffic Collector Helm Chart

A production-ready Helm chart for deploying the lightweight eBPF-based network traffic collector on Kubernetes clusters.

## Prerequisites

- Kubernetes 1.16+
- Helm 3.0+
- Linux kernel 4.9+ (for eBPF support)
- One of the following for trace collection:
  - Grafana Tempo (OTLP endpoint)
  - Jaeger (collector endpoint)

## Installation

### Add the Helm repository (when available)

```bash
helm repo add traffic-collector https://example.com/charts
helm repo update
```

### Install from local chart

```bash
helm install traffic-collector ./charts/traffic-collector \
  --namespace monitoring \
  --create-namespace
```

### Install with custom values

```bash
helm install traffic-collector ./charts/traffic-collector \
  --namespace monitoring \
  --create-namespace \
  -f values-prod.yaml \
  --set image.repository=your-registry/traffic-collector
```

## Configuration

### Basic Configuration

| Parameter | Description | Default |
|-----------|-------------|----------|
| `image.repository` | Docker image repository | `jarpsimoes/traffic-collector` |
| `image.tag` | Docker image tag | `latest` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `collector.logLevel` | Log level (debug, info, warn, error) | `info` |
| `collector.healthCheckPort` | Health check HTTP port | `8080` |

### Tempo Configuration

| Parameter | Description | Default |
|-----------|-------------|----------|
| `tempo.enabled` | Enable Tempo exporter | `true` |
| `tempo.endpoint` | Tempo OTLP gRPC endpoint | `http://tempo:4317` |
| `tempo.insecure` | Use insecure connection | `true` |

### Jaeger Configuration

| Parameter | Description | Default |
|-----------|-------------|----------|
| `jaeger.enabled` | Enable Jaeger exporter | `true` |
| `jaeger.endpoint` | Jaeger collector HTTP endpoint | `http://jaeger-collector:14268/api/traces` |
| `jaeger.agentHost` | Jaeger agent hostname | `jaeger-agent` |
| `jaeger.agentPort` | Jaeger agent port | `6831` |

### Resource Configuration

| Parameter | Description | Default |
|-----------|-------------|----------|
| `resources.requests.cpu` | CPU request | `100m` |
| `resources.requests.memory` | Memory request | `128Mi` |
| `resources.limits.cpu` | CPU limit | `500m` |
| `resources.limits.memory` | Memory limit | `256Mi` |

## Examples

### Development Deployment

```bash
helm install traffic-collector ./charts/traffic-collector \
  -f values-dev.yaml \
  --namespace monitoring \
  --create-namespace
```

### Production Deployment with Tempo

```bash
helm install traffic-collector ./charts/traffic-collector \
  -f values-prod.yaml \
  --set tempo.endpoint=tempo.observability.svc.cluster.local:4317 \
  --set jaeger.enabled=false \
  --namespace monitoring \
  --create-namespace
```

### Production Deployment with Jaeger

```bash
helm install traffic-collector ./charts/traffic-collector \
  -f values-prod.yaml \
  --set jaeger.endpoint=http://jaeger-collector.observability:14268/api/traces \
  --set tempo.enabled=false \
  --namespace monitoring \
  --create-namespace
```

## Verification

### Check DaemonSet Status

```bash
kubectl get daemonset -n monitoring traffic-collector
kubectl get pods -n monitoring -l app=traffic-collector
```

### Check Logs

```bash
kubectl logs -n monitoring -l app=traffic-collector -f
```

### Health Check

```bash
kubectl port-forward -n monitoring daemonset/traffic-collector 8080:8080
curl http://localhost:8080/health
curl http://localhost:8080/ready
```

## Troubleshooting

### Pod Not Starting

```bash
kubectl describe pod -n monitoring <pod-name>
kubectl logs -n monitoring <pod-name>
```

### No Traces Appearing

1. Verify exporter endpoints are reachable:
```bash
kubectl exec -n monitoring <pod-name> -- curl http://tempo:4317
```

2. Check collector permissions:
```bash
kubectl describe clusterrole traffic-collector
```

3. Verify eBPF support:
```bash
kubectl exec -n monitoring <pod-name> -- uname -r
```

### High Memory Usage

Adjust in values.yaml:
- Reduce `collector.batchSize`
- Increase `collector.batchTimeout`
- Lower `resources.limits.memory`

## Uninstall

```bash
helm uninstall traffic-collector -n monitoring
```

## Support

For issues and questions, visit: https://github.com/jarpsimoes/traffic-collector/issues
