# 📈 Observability & Metrics

Castle Rock Agent is designed to seamlessly integrate with the standard Cloud Native observability stack: **Prometheus** and **Grafana**. 

If you use the `docker-compose.yml` file, this entire stack is bootstrapped automatically.

---

## 📊 Prometheus — Exported Metrics

The agent acts as an exporter, exposing container metrics in Prometheus format at `http://localhost:9110/metrics` (configurable).

### Available Metrics

| Metric | Type | Description |
|---|---|---|
| `castle_rock_container_cpu_percent` | Gauge | CPU usage percentage |
| `castle_rock_container_memory_usage_bytes` | Gauge | Used memory (bytes) |
| `castle_rock_container_memory_limit_bytes` | Gauge | Memory limit (bytes) |
| `castle_rock_container_memory_percent` | Gauge | Memory usage percentage |
| `castle_rock_container_network_rx_bytes` | Gauge | Bytes received from the network |
| `castle_rock_container_network_tx_bytes` | Gauge | Bytes transmitted to the network |
| `castle_rock_container_block_read_bytes` | Gauge | Bytes read from disk |
| `castle_rock_container_block_write_bytes` | Gauge | Bytes written to disk |
| `castle_rock_container_info` | Gauge | Container metadata (labels) |

*All metrics have the following labels:* `container_id`, `container_name`, `image`, `host_id`.

### PromQL Query Example

```promql
# CPU for a specific container
castle_rock_container_cpu_percent{container_name="postgres"}

# Top 5 containers by memory
topk(5, castle_rock_container_memory_percent)

# Total network traffic
sum(castle_rock_container_network_rx_bytes) by (container_name)
```

---

## 📈 Grafana — 5 Pre-configured Dashboards

Docker Compose automatically provisions **5 dashboards** in Grafana. They all update in real-time.

**Access:** http://localhost:3000 → Login with the credentials defined in your `.env` file (default: `admin` / `castlerock`).

1. **Dashboard 1: Overview:** A broad overview of all containers (Average CPU/Mem, Top traffic, Speedometers, Bar Gauges).
2. **Dashboard 2: Container Detail:** A deep dive into a specific container selected via a dropdown. Shows history over time.
3. **Dashboard 3: Network Analysis:** Focused analysis on network traffic (RX / TX / Top downloaders).
4. **Dashboard 4: Memory Deep Dive:** Deep memory analysis — useful to spot leaks and risk of OOM kills by viewing Usage vs. Limits.
5. **Dashboard 5: Alerts & Health:** Health monitoring, threshold violations, and Maximum (Peak) vs. Average usage analysis.

---

## ⚠️ Configurable Alerts

The alert system works in **two layers**:

### Layer 1: TUI Alerts (Internal / Integrated)

Defined in `configs/config.yaml`, evaluated by the agent's internal engine in real-time.

```yaml
alerts:
  enabled: true
  rules:
    - name: "Critical CPU"
      metric: "cpu_percent"
      operator: ">"
      threshold: 80.0
      duration: 2m           # Only triggers after 2 mins above the threshold
      severity: "critical"
```

When an alert triggers, it appears:
- 🚨 On the TUI status bar
- 📋 In the event log with details
- With a visual indicator on the container in the table (`🚨` or `⚠️`)

### Layer 2: Prometheus Alerts (External)

Defined in `deploy/prometheus/alert_rules.yml`, evaluated by Prometheus itself based on scraped metrics:

```yaml
- alert: ContainerHighCPU
  expr: castle_rock_container_cpu_percent > 80
  for: 2m
  labels:
    severity: critical
  annotations:
    summary: "High CPU on container {{ $labels.container_name }}"
```

These can be connected to **Alertmanager** for notifications via Slack, email, PagerDuty, etc.

---

## 🐳 Docker Compose Stack

The `docker-compose.yml` sets up 3 services:

```
┌────────────────────────────────────────────────────────┐
│                  Docker Compose Stack                  │
│                                                        │
│  ┌──────────────┐   ┌────────────┐   ┌──────────────┐  │
│  │ Castle Rock  │ → │ Prometheus │ → │   Grafana    │  │
│  │  Agent :9110 │   │   :9090    │   │    :3000     │  │
│  │  (headless)  │   │ (scraping) │   │ (dashboards) │  │
│  └──────┬───────┘   └────────────┘   └──────────────┘  │
│         │                                              │
│         ▼                                              │
│   Docker Socket                                        │
│  (monitoring)                                          │
└────────────────────────────────────────────────────────┘
```

### Headless Mode
In Docker Compose, the agent runs in **headless mode** (`CASTLE_ROCK_MODE=headless`). 
It acts solely as a Prometheus metrics server, without attempting to open the interactive dashboard since it is running detached in the background.

---

## 🌐 Cluster Mode (Multi-Host Monitoring)

The agent supports running in a distributed way to monitor multiple servers at once through a single dashboard/scraper.

You spin up **one Leader** and multiple **Workers** (remote agents that push data to the Leader via HTTP).

```bash
# Main Server (Leader)
CASTLE_ROCK_CLUSTER_MODE=leader \
CASTLE_ROCK_CLUSTER_HOST_ID=hq \
CASTLE_ROCK_CLUSTER_AUTH_TOKEN="my-secret-token" \
CASTLE_ROCK_CLUSTER_SHARED_SECRET="my-aes-key" \
make run

# On another server (Worker) - No TUI, running in the background
CASTLE_ROCK_CLUSTER_MODE=worker \
CASTLE_ROCK_CLUSTER_HOST_ID=branch-nyc \
CASTLE_ROCK_CLUSTER_LEADER_URL=http://<LEADER_IP>:9110 \
CASTLE_ROCK_CLUSTER_AUTH_TOKEN="my-secret-token" \
CASTLE_ROCK_CLUSTER_SHARED_SECRET="my-aes-key" \
make run
```

The Worker's containers will automatically appear in the Leader's TUI with the "HOST" column indicating their origin, and the Leader's Prometheus will expose the metrics using the `host_id` label so Grafana can filter by node.
