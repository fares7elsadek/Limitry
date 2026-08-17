<h1 align="center">
  <br>
  ⚡ Limitry
  <br>
</h1>

<p align="center">
  <b>A production-grade distributed rate limiter written in Go</b><br>
  <sub>Redis-backed · Dual-mode · Atomic Lua scripts · Per-route configuration · Kubernetes-ready</sub>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.21+-00ADD8?style=for-the-badge&logo=go&logoColor=white" />
  <img src="https://img.shields.io/badge/Redis-7.x-DC382D?style=for-the-badge&logo=redis&logoColor=white" />
  <img src="https://img.shields.io/badge/Kubernetes-ready-326CE5?style=for-the-badge&logo=kubernetes&logoColor=white" />
  <img src="https://img.shields.io/badge/License-MIT-green?style=for-the-badge" />
</p>

---

## Overview

**Limitry** is a lightweight, distributed rate limiter built for cloud-native environments. It protects your APIs from traffic spikes and abuse by enforcing per-client, per-route request quotas — backed by Redis for consistency across multiple service instances.

It runs in **two modes**, making it versatile enough to drop in front of any backend or integrate with any existing API gateway.

```
 Client → [ Limitry ] → Your Backend
               │
        Redis (shared state)
```

---

## Features

| Feature | Description |
|---|---|
| 🔄 **Two Rate-Limiting Algorithms** | Token Bucket and Sliding Window Counter |
| 🌐 **Dual Operation Modes** | Transparent reverse proxy or standalone check API |
| ⚛️ **Atomic Redis Scripts** | Lua scripts ensure correctness under high concurrency — no race conditions |
| 🗺️ **Per-Route Rules** | Different limits and algorithms for different API paths |
| 🛡️ **Fail-Mode Control** | Choose between fail-open (allow) or fail-closed (deny) when Redis is down |
| ⏱️ **Retry-After Headers** | Clients always know exactly when to retry |
| 📊 **Full Observability** | OpenTelemetry traces, Prometheus metrics, Grafana dashboards, structured logging |
| ☸️ **Kubernetes-Ready** | Production manifests with Kustomize — deploy the full stack in one command |
| 📦 **Single Binary** | Zero runtime dependencies beyond Redis |

---

## Algorithms

### Token Bucket

Tokens accumulate at a constant rate up to a maximum capacity. Each request consumes one token. Ideal for **bursty traffic** — clients can absorb short spikes while still being capped over time.

```
Tokens refill at: limit / window (tokens/sec)
```

### Sliding Window Counter

A hybrid of fixed-window counters. It blends the current and previous window's counts weighted by how far into the current window we are. Provides **smoother traffic shaping** than a fixed window while being far cheaper than a pure sliding log.

```
estimated = prev_count × (1 − elapsed_fraction) + current_count
```

---

## Architecture

```
Limitry/
├── cmd/
│   └── gateway/
│       └── main.go                    # Entrypoint — loads config, wires up mode handler
├── internal/
│   ├── config/
│   │   └── config.go                  # YAML config loader & validator
│   ├── limiter/
│   │   ├── limiter.go                 # Engine — Redis connection, routing to algorithms
│   │   ├── tokenbucket.go             # Token Bucket via atomic Lua script
│   │   └── sliding_window_counter.go  # Sliding Window Counter via atomic Lua script
│   ├── proxy/
│   │   └── proxy.go                   # Reverse proxy mode handler
│   ├── checkapi/
│   │   └── checkapi.go                # Standalone check API handler (POST /check)
│   └── telemetry/
│       ├── tracing.go                 # OpenTelemetry tracing setup
│       ├── metrics.go                 # Prometheus metrics definitions and server
│       ├── logging.go                 # Zerolog structured logging initialization
│       └── resource.go                # Telemetry resource attributes
├── k8s/                               # Kubernetes manifests (Kustomize)
│   ├── kustomization.yaml
│   ├── namespace.yaml
│   ├── limitry/                       # Limitry Deployment, Service, ConfigMap
│   ├── redis/                         # Redis Deployment, Service
│   ├── otel-collector/                # OTel Collector Deployment, Service, ConfigMap
│   ├── jaeger/                        # Jaeger Deployment, Service
│   ├── prometheus/                    # Prometheus Deployment, Service, ConfigMap
│   └── grafana/                       # Grafana Deployment, Service, ConfigMap
├── docker/
│   ├── grafana/                       # Pre-configured Grafana dashboard and datasources
│   ├── otel-collector.yaml            # Configuration for OpenTelemetry Collector
│   └── prometheus.yaml                # Scrape configuration for Prometheus
├── Dockerfile                         # Multi-stage build
├── docker-compose.yaml                # Full local dev stack
├── go.mod
└── config.yaml                        # Rate-limiting configuration
```

---

## Configuration

Limitry is configured with a single `config.yaml` file.

```yaml
# Operation mode: "proxy" or "check"
mode: proxy

# Only required in proxy mode — the backend to forward allowed requests to
backend:
  url: http://localhost:9090

# Redis connection
redis:
  addr: localhost:6379
  fail_mode: open   # "open" = allow requests if Redis is down, "closed" = deny

# Per-route rate-limiting rules
routes:
  - path: /api/login
    algorithm: sliding_window   # smoother enforcement
    limit: 5
    window: 60s

  - path: /api/search
    algorithm: token_bucket     # allow short bursts
    limit: 100
    window: 60s

# Telemetry (Tracing, Metrics, Logs)
telemetry:
  enabled: true
  service_name: limitry
  environment: production
  traces:
    enabled: true
    endpoint: otel-collector:4317
    insecure: true
    sampling_ratio: 1.0
  metrics:
    enabled: true
    port: 8000
  logs:
    enabled: true
    level: info
    format: json
```

### Configuration Reference

| Field | Type | Description |
|---|---|---|
| `mode` | `proxy` \| `check` | How Limitry operates |
| `backend.url` | string | Upstream to proxy to (proxy mode only) |
| `redis.addr` | string | Redis server address |
| `redis.fail_mode` | `open` \| `closed` | Behavior when Redis is unreachable |
| `routes[].path` | string | Exact URL path to match |
| `routes[].algorithm` | `token_bucket` \| `sliding_window` | Rate-limiting algorithm |
| `routes[].limit` | int | Max requests allowed per window |
| `routes[].window` | duration | Time window (e.g. `60s`, `1m`, `1h`) |
| `telemetry.enabled` | bool | Master toggle for all telemetry |
| `telemetry.traces.enabled` | bool | Enable OpenTelemetry tracing |
| `telemetry.metrics.enabled` | bool | Enable Prometheus metrics endpoint |
| `telemetry.logs.enabled` | bool | Enable structured JSON logging |

---

## Operation Modes

### Reverse Proxy

Limitry sits transparently in front of your backend. It checks each incoming request, forwards allowed requests upstream, and rejects throttled ones with `429 Too Many Requests`.

```
POST /api/login  →  Limitry  →  (if allowed) Your Backend
                             →  (if throttled) 429 + Retry-After
```

**Client Identity** is resolved from the `X-Client-Id` header, falling back to `RemoteAddr`.

### Check API

Limitry runs as a standalone decision service. Your application calls `POST /check` and receives a JSON response telling it whether to allow or deny the request.

```bash
curl -X POST http://localhost:8080/check \
  -H "Content-Type: application/json" \
  -d '{"client_id": "user-42", "route": "/api/login"}'
```

**Response (allowed):**
```json
{
  "allowed": true,
  "retry_after_seconds": 0
}
```

**Response (throttled):**
```json
{
  "allowed": false,
  "retry_after_seconds": 47
}
```

---

## Getting Started

### Prerequisites

- [Go 1.21+](https://go.dev/dl/)
- [Redis 7.x](https://redis.io/download) running locally or remotely

### Build from Source

```bash
git clone https://github.com/fares7elsadek/Limitry.git
cd Limitry

go mod download
go build -o limitry ./cmd/gateway
```

### Run

```bash
./limitry --config config.yaml
```

---

## Deployment

Limitry supports three deployment methods depending on your environment.

### Docker Compose (Local Development)

Spins up the full stack — Limitry, Redis, Jaeger, OTel Collector, Prometheus, and Grafana:

```bash
docker compose up -d
```

### Docker

```bash
docker build -t limitry .
docker run -v $(pwd)/config.yaml:/etc/limitry/config.yaml -p 8080:8080 limitry
```

### Kubernetes

Limitry ships with production-ready Kubernetes manifests using [Kustomize](https://kustomize.io/). A single command deploys the full stack into a dedicated `limitry` namespace:

| Component | Replicas | Purpose |
|---|---|---|
| **Limitry** | 2 | Rate limiter gateway |
| **Redis** | 1 | Shared state store |
| **OTel Collector** | 1 | Trace pipeline |
| **Jaeger** | 1 | Trace visualization |
| **Prometheus** | 1 | Metrics collection |
| **Grafana** | 1 | Dashboards & alerting |

**Deploy:**

```bash
kubectl apply -k k8s/
```

**Verify:**

```bash
kubectl get pods -n limitry
```

**Access services** via port-forward:

```bash
# Limitry API
kubectl port-forward -n limitry svc/limitry 8080:8080

# Grafana (admin/admin)
kubectl port-forward -n limitry svc/grafana 3000:3000

# Jaeger UI
kubectl port-forward -n limitry svc/jaeger 16686:16686

# Prometheus
kubectl port-forward -n limitry svc/prometheus 9090:9090
```

**Tear down:**

```bash
kubectl delete -k k8s/
```

> **Note:** The manifests reference `ghcr.io/fares7elsadek/limitry:1.0.0` by default. Update the image in `k8s/limitry/deployment.yaml` if using a different registry.

---

## Observability

Limitry provides full observability out of the box. All telemetry is available when deploying via Docker Compose or Kubernetes.

| Tool | URL | Credentials | What it shows |
|---|---|---|---|
| **Grafana** | [localhost:3000](http://localhost:3000) | `admin` / `admin` | Pre-built Limitry traffic dashboard |
| **Prometheus** | [localhost:9090](http://localhost:9090) | — | Raw metrics (`limitry_requests_total`, latency histograms) |
| **Jaeger** | [localhost:16686](http://localhost:16686) | — | Distributed traces for every rate-limit decision |

### Exposed Metrics

| Metric | Type | Labels | Description |
|---|---|---|---|
| `limitry_requests_total` | Counter | `route`, `allowed`, `mode` | Total rate-limit decisions |
| `limitry_ratelimit_duration_seconds` | Histogram | `route`, `algorithm` | Redis round-trip latency |
| `limitry_redis_errors_total` | Counter | `route`, `failmode` | Redis errors during evaluation |
| `limitry_request_duration_seconds` | Histogram | `route`, `status_code`, `mode` | End-to-end HTTP request duration |

---

## Testing

### Proxy Mode

```bash
# Fire 10 rapid requests and watch for 429s
for i in $(seq 1 10); do
  curl -s -o /dev/null -w "%{http_code}\n" \
    -H "X-Client-Id: test-user" \
    http://localhost:8080/api/login
done
```

### Check API Mode

```bash
curl -X POST http://localhost:8080/check \
  -H "Content-Type: application/json" \
  -d '{"client_id": "user-1", "route": "/api/login"}'
```

---

## Contributing

Contributions are welcome! Feel free to open an issue or a pull request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feat/my-feature`)
3. Commit your changes (`git commit -m 'feat: add my feature'`)
4. Push to the branch (`git push origin feat/my-feature`)
5. Open a Pull Request

---

## License

This project is licensed under the MIT License — see the [LICENSE](LICENSE) file for details.

---

<p align="center">
  Built with ❤️ in Go by <a href="https://github.com/fares7elsadek">fares7elsadek</a>
</p>
