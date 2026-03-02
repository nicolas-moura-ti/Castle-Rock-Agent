# 🏰 Castle Rock Agent

*Read this in other languages: [English](README.md), [Português](README.pt-BR.md).*

> A Go-native observability agent for monitoring Docker containers with an interactive dashboard, Prometheus metrics, and configurable alerts.

### ⛰️ Why "Castle Rock"?
Inspired by medieval watchtowers built on rocky peaks (Castle Rocks), which offered an **absolute panoramic view** of everything happening around the castle. Just like these towers, this agent sits at a privileged observation point (the Docker Socket) to monitor, watch, and alert on the health of your entire container infrastructure.

---

## 🧠 How It Works — Simple Explanation

Imagine you have several Docker containers running (database, API, nginx...). You want to know: **How much CPU is each one using? Is it consuming too much memory? Is the network normal?**

The problem is that Docker alone has the memory of a goldfish. It knows what's happening **now**, but has no idea what happened 5 minutes ago. And it also has no way of generating beautiful charts to show off in your Friday meetings.

To solve this, we use **3 pieces** working together:

### 🏰 Castle Rock Agent (this project)
It's the **collector**. It connects to Docker, asks "how is each container doing?", and translates this information into a standardized format. Without it, Prometheus would have no way to access Docker data.

**Analogy:** It's like a thermometer that measures the temperature and displays the reading on a screen.

### 📊 Prometheus
It's the **metrics database**. Every 5 seconds it visits the agent (`http://agent:9110/metrics`), collects the numbers, and **saves them with a timestamp**. This way you can ask: "What was the postgres CPU at 2:30 PM?"

**Analogy:** It's like a notebook where someone writes down the temperature every 5 seconds. After a week, you have a complete history.

### 📈 Grafana
It's the **visualization panel**. It reads data from Prometheus and generates **charts, gauges, and tables** in real-time. It's where you actually "see" what is going on.

**Analogy:** It's like taking that notebook and turning it into a beautiful chart on the screen.

### Data Flow

```
Docker Containers → Castle Rock Agent → Prometheus → Grafana
(generate metrics)  (collects & trans)  (stores)     (visualizes)
```

**Why not use a single tool?** Because in practice, each piece does ONE thing very well. This separation is the industry standard — that's how companies like Google, Netflix, and Uber monitor their systems.

---

## ✨ Features

| Feature | Description |
|---|---|
| **Interactive TUI** | Fullscreen dashboard with [Bubble Tea](https://github.com/charmbracelet/bubbletea) — container table, metrics, events |
| **Real-time Metrics** | CPU%, Memory%, Network I/O, Disk I/O via Docker Stats API |
| **Prometheus Exporter** | Exposes 9 metrics at `/metrics` for scraping (port 9110) |
| **Grafana Dashboard** | Pre-configured dashboard with 6 panels (CPU, Memory, Network, Gauges) |
| **Configurable Alerts** | Customizable rules with threshold + duration (similar to Alertmanager) |
| **Container Actions** | Stop and restart containers straight from the TUI with confirmation |
| **Streaming Logs** | View real-time container logs (like `docker logs -f`) |
| **Docker Events** | Lifecycle events (start, stop, die) with icons and colors |
| **Cluster Mode 🌐** | Multi-Host Architecture (Leader/Worker) to aggregate metrics from multiple servers |
| **SQLite Historian 🗄️** | Pure-Go SQLite local database to persist historical events and alerts (because your memory shouldn't be as volatile as RAM). |
| **Auto Prune 🧹** | Built-in smart garbage collector for Docker. It watches your host disk and natively calls `docker system prune` before your server goes boom. |
| **Service Map 🕸️** | Press `M` to visually inspect your Docker Network topology and see who is talking to whom. |
| **Security Auditing 🛡️** | Real-time Shift-Left security scanning. Detects 9 critical anti-patterns (e.g. root user, privileged mode, wildcard DB ports) right from the `docker inspect` data. |
| **i18n & Localization 🌍** | The agent natively speaks English (`en`) and Portuguese (`pt`). Change it via `config.yaml` or `CASTLE_ROCK_LANGUAGE=pt`. |
| **Config YAML + ENV** | Configuration via `config.yaml` with environment variable overrides |

---

## Architecture

```
┌────────────────────────────────────────────────────────────────┐
│                    Castle Rock Agent v0.3.0                    │
│                                                                │
│  ┌─────────────┐   ┌──────────────┐   ┌─────────────────────┐  │
│  │     TUI     │   │  Prometheus  │   │    Alert Engine     │  │
│  │ (bubbletea) │   │  HTTP :9110  │   │   (rules + state)   │  │
│  └──────┬──────┘   └──────┬───────┘   └──────────┬──────────┘  │
│         │                 │                      │             │
│         │                 │  ┌────────────────┐  │             │
│         │                 │◄─┤ Cluster (Push) │  │             │
│         │                 │  └───────▲────────┘  │             │
│         └─────────┬───────┴──────────┼───────────┘             │
│                   ▼                  │ (HTTP POST)             │
│         ┌──────────────────┐         │  ┌───────────────────┐  │
│         │  Docker Client   │         └──┤ Worker Node (Ag.) │  │
│         │  (Official SDK)  │            └───────────────────┘  │
│         └─────────┬────────┘                                   │
│                   │                                            │
└───────────────────┼────────────────────────────────────────────┘
                    ▼
            Docker Engine API
      (unix:///var/run/docker.sock)
```

### Directory Structure

```
castle-rock-agent/
├── cmd/agent/main.go              # Entrypoint — bootstrapping and modes (TUI/headless)
├── internal/
│   ├── docker/client.go           # Docker SDK: containers, stats, events, logs, actions
│   ├── tui/tui.go                 # Interactive dashboard (Bubble Tea + lipgloss)
│   ├── metrics/prometheus.go      # Prometheus exporter with 9 GaugeVecs
│   ├── alerts/alerts.go           # Alert engine (rules, pending→firing→resolved)
│   ├── config/config.go           # YAML loader + env vars (12-Factor App)
│   ├── logger/logger.go           # Custom slog with ANSI colors
│   └── collector/container.go     # Collector interface (extensible)
├── pkg/models/container.go        # DTOs: ContainerInfo, ContainerMetrics, ContainerDisplay
├── configs/config.yaml            # Documented YAML config
├── deploy/
│   ├── prometheus/
│   │   ├── prometheus.yml         # Scrape config
│   │   └── alert_rules.yml        # Prometheus alert rules
│   └── grafana/provisioning/
│       ├── datasources/           # Auto-config Prometheus as datasource
│       └── dashboards/            # Pre-configured JSON dashboards
├── docker-compose.yml             # Full stack: Agent + Prometheus + Grafana
├── Dockerfile                     # Multi-stage build (Go 1.24 → Alpine)
├── Makefile                       # Standardized targets
└── go.mod / go.sum
```

---

## Prerequisites

| Dependency | Min Version | Verification |
|---|---|---|
| **Go** | 1.24+ | `go version` |
| **Docker** | 20.10+ | `docker --version` |
| **Docker Desktop** or **Engine** | Running | `docker info` |
| **Make** | Any | `make --version` |

### macOS — Xcode Command Line Tools

```bash
# Install (if required)
xcode-select --install

# Accept license (REQUIRED after install/update)
sudo xcodebuild -license accept
```

> ⚠️ **Without accepting the license, `go build` will fail** with `"please accept the Xcode license as the root user"`.

---

## Quick Start

### Mode 1: Local TUI (Development)

```bash
# Clone
git clone https://github.com/nicolas-moura-ti/castle-rock-agent.git
cd castle-rock-agent

# Install dependencies
make tidy

# Run (opens interactive dashboard)
make run
```

### Mode 2: Docker Compose (Full Observability Stack)

```bash
# Spin up Agent + Prometheus + Grafana
docker compose up -d

# Access:
# - Grafana:    http://localhost:3000  (admin / castlerock)
# - Prometheus: http://localhost:9090
# - Metrics:    http://localhost:9110/metrics
```

To stop:

```bash
docker compose down
```

### Mode Comparison

| | `make run` | `docker compose up` |
|---|---|---|
| **Where it runs** | Right on your machine | Inside Docker containers |
| **TUI (terminal dashboard)** | ✅ Yes | ❌ No (headless) |
| **Prometheus collecting** | ❌ No | ✅ Yes |
| **Grafana with charts** | ❌ No | ✅ Yes |
| **Why use it** | Quick analysis and dev | 24/7 monitoring and visual alerts |

> 💡 **PARALLEL USE:** You can (and should!) run both modes at the same time!
> Keep `docker compose up -d` running in the background collecting metrics for Grafana, and whenever you want to inspect deeply or test loads ("Stress Test"), open a terminal and run `make run`. One does not interfere with the other, and both cooperatively monitor the same environment.

### Mode 3: Multi-Host Cluster 🌐

The agent supports running in a distributed way to monitor multiple servers at once.
You spin up **one Leader** and multiple **Workers** (remote agents that send data to the Leader).

```bash
# Main Server (Leader)
CASTLE_ROCK_CLUSTER_MODE=leader CASTLE_ROCK_CLUSTER_HOST_ID=hq make run

# On another server (Worker) - No TUI, running in the background
CASTLE_ROCK_CLUSTER_MODE=worker \
CASTLE_ROCK_CLUSTER_HOST_ID=branch-nyc \
CASTLE_ROCK_CLUSTER_LEADER_URL=http://<LEADER_IP>:9110 \
make run
```

The Worker's containers will automatically appear in the Leader's TUI with the "HOST" column indicating their origin, and the Leader's Prometheus will expose the metrics using the `host_id` tag for Grafana.

---

## 🖥️ TUI — Interactive Dashboard

When running `make run`, the agent opens a fullscreen dashboard:

```
 🏰 Castle Rock Agent    v0.3.0 │ Docker 29.2.1 │ ⏱ 2m30s │ 📡 5 events

    ID             NOME                 CPU%    MEM%    MEM       NET ↓/↑     ESTADO
 ▸  9b157cd0abf6   postgres             2.3%    45.2%   350MB     12M/5M      ● up
    abc123def456   redis                0.1%    12.8%   98MB      3M/1M       ● up
    fed987654321   nginx                0.5%    5.1%    42MB      50M/45M     ● up

 📋 Events
  ╭──────────────────────────────────────────────────────╮
  │  16:05:32 🟢 start      nginx                       │
  │  16:05:30 📦 create     nginx                       │
  │  16:04:15 🔴 die        old-container               │
  ╰──────────────────────────────────────────────────────╯

  ↑↓ nav │ space select │ enter details │ l log 1 │ L log N │ x shell │ C prune │ s stop │ R restart │ S stress │ M map │ ? help │ q quit
```

> 💡 **Context-Sensitive Help Bar:** The bottom bar automatically changes its shortcuts depending on the current screen. For example, when viewing logs it shows `↑↓ scroll │ / grep │ f tail │ E export │ Esc back`. When on the Prune Dashboard it shows `[i] images │ [v] volumes │ Esc back`.

### Keyboard Shortcuts

| Key | Action |
|---|---|
| `↑` / `k` | Navigate up (or scroll up logs) |
| `↓` / `j` | Navigate down (or scroll down logs) |
| `Enter` | Expand container details (metrics, labels, networks, ports) |
| `l` | Toggle real-time logs for the **selected** container |
| `Space` | Select/Deselect container for Multi-Tailing |
| `Shift+L` | Toggle aggregated logs for **all selected** containers |
| `/` | Apply Live Grep (filter) while viewing logs |
| `f` | Return to auto-tailing at the bottom of the logs |
| `E` | Export current log view to `/tmp/castle-rock-logs-*.txt` |
| `x` | Open Interactive Shell inside container (`/bin/sh` or `/bin/bash`) |
| `C` | Open Interactive Prune Dashboard (Cleanup images/volumes) |
| `s` | **Stop** container (asks for `y` confirmation) |
| `R` | **Restart** container (asks for `y` confirmation) |
| `S` | **Stress Test Mode** (CPU/Memory to simulate Noisy Neighbor) |
| `M` | **Service Map** (Visual network topology) |
| `r` | Manual list refresh |
| `?` | Show/hide detailed help |
| `Esc` | Close open panels |
| `q` / `Ctrl+C` | Quit the agent |

### Indicator Colors

| Color | Meaning |
|---|---|
| 🟢 Green | Normal (CPU < 40%, MEM < 50%) |
| 🟡 Yellow | Warning (CPU 40-80%, MEM 50-80%) |
| 🔴 Red | Critical (CPU > 80%, MEM > 80%) |

---

## ⚡ Stress Test Mode (Noisy Neighbor)

The agent has a built-in didactic feature to stress the machine and watch metrics/alerts trigger in Grafana in real-time.

By pressing `S` in the TUI, you create a temporary container built via code (`alpine` + native `stress-ng`) that injects load into the host:
- `c` **CPU:** Stresses 2 cores at 100%
- `m` **Memory:** Allocates and locks exactly 256MB (without burning CPU)
- `b` **Both:** Applies double load

*The container lasts exactly 30 seconds and self-destructs (`AutoRemove: true`), so you can see the alert firing curve and, right after, it recovering (resolved).*

### ⚠️ Stress Test Limitations
- **Virtual Machine vs Host:** On macOS and Windows (Docker Desktop), the maximum load pointed out by the agent reflects the resource limits of the allocated Docker Virtual Machine, not the entire physical machine.
- **Initial Connectivity:** Requires internet access on the 1st run to download the official `alpine` image (only ~5MB).
- **No Docker Proxy (Read-Only):** The test does not work in headless mode attached to `docker-socket-proxy`. The proxy has the `POST=0` flag locked for security (doesn't allow creating containers). Because of this, we suggest using the TUI via `make run` directly on the local real OS for success.

---

## 📜 Advanced Logs Viewer

The TUI includes a native **Advanced Logs Viewer** packed with modern CLI features, reducing the need to leave the dashboard to inspect container output.

### 📖 How to read the Logs (`l`, `L`, `f`)

- **`l` (Log 1):** Use the arrow keys to place the `▸` cursor over a single container and press `l` (lowercase L) to tail its logs.
- **`L` (Log N / Multi-Tailing):** Navigate the table and press `Space` on multiple containers to select them. Then press `Shift+L`. The agent will aggregate their streams in real-time, placing colored `[container-name]` tags on each line.
- **`f` (Follow):** When a log screen is open, the view automatically scrolls (tails) to the bottom. If you press `↑` (Up), the scrolling pauses so you can read a stack trace. When you are done investigating, press `f` to collapse the view back to the live tail edge.

### Key Features
1. **🔍 Live Grep (Fuzzy Search):** Press `/` to open the search bar. The view will instantly filter logs as you type, matching case-insensitively. Press `Enter` or `Esc` to close the search.
2. **🔀 Multi-Tailing (Aggregated Logs):** Aggregate logs side-by-side using `Space` and `Shift+L`.
3. **⏪ History Pagination:** Use the `↑` and `↓` (or `k` and `j`) keys to scroll back in the log history without losing the context of new incoming logs. Press `f` to resume live tailing.
4. **🎨 JSON Highlighting:** The viewer automatically detects common log levels (`error`, `warn`, `info`) in structured JSON outputs and colorizes them (e.g., bright red for `error`).
5. **⏱️ Timestamps:** It requests logs from the Docker API with the `Timestamps: true` flag enabled, rendering exact ISO8601 timestamps in muted gray to help you align metrics spikes with log events.
6. **📤 Quick Export:** Press `E` while viewing a log panel to save a snapshot of the current view (including any active Grep filters) directly to `/tmp/castle-rock-logs-[name]-[timestamp].txt`. A confirmation event (`📤 export`) appears in the Events panel showing the exact file path.

> ⚠️ **Important:** The `E` shortcut only works when you are **inside the log view** (after pressing `l` or `L`). It does not work from the main dashboard — there are no logs on screen to export. Steps: `l` (open logs) → `E` (export) → check `/tmp/` folder.

---

## 💼 Enterprise UX Tools

For advanced debugging and operations, the agent includes **Enterprise UX Tools** designed to feel like a high-end command line dashboard.

### 1. 🖥️ Host Health Metrics
The agent automatically reports the physical **Node / Host Operating System** CPU and Memory percentage in the top status bar. Instead of guessing why a cluster is struggling due to pure Docker usage, you can see physical RAM limits right from the Agent.

### 2. 💻 Interactive Shell (Exec)
If you spot an error in the logs, selecting a container and pressing `x` suspends the Agent's UI and drops you directly into an **Interactive TTY Shell** (`/bin/sh` or `/bin/bash`) running inside the container. After typing `exit`, you smoothly return to the TUI exactly where you left off.

### 3. 🧹 Interactive Prune Dashboard
Running low on disk space? Press **`C`** (Cleanup) to open the interactive prune dashboard. The engine analyzes `docker system df` metrics and displays precisely how much memory dangling images and idle local volumes are occupying. Press `i` to prune images or `v` to prune volumes, and instantly view the reclaimed space.

---

## 🔎 Debug & Diagnostics

The container detail panel (`Enter`) now includes two powerful diagnostic tools:

### 1. 🔎 Environment Variables Inspector
When you press `Enter` on a container, the detail panel now displays all environment variables configured inside it. Keys are color-coded for readability, and values longer than 50 characters are truncated. Up to 15 variables are shown, with a `+N more` indicator if there are additional ones. This is invaluable for catching misconfigurations like a wrong `DATABASE_URL` or an incorrect `NODE_ENV`.

### 2. 🏥 Health Check Status
Containers that define a `HEALTHCHECK` in their Dockerfile now show a health badge directly in the main table:
- ❤️ **healthy** — the container’s health check is passing
- 🩺 **unhealthy** — the health check is failing (investigate immediately!)
- ⏳ **starting** — the container is still initializing

In the detail panel, you also see the **last health check output** (stdout), making it easy to understand *why* a container is unhealthy without running `docker inspect` manually.

### 3. ⚙️ Entrypoint / Command
Shows the exact command being executed by the container (e.g., `node server.js`, `postgres -c shared_buffers=256MB`). No more guessing what the process is actually doing.

### 4. 📂 Volumes & Bind Mounts
Lists all mounted volumes and bind paths in the format `source → destination (type)`. Instantly spot missing mounts or wrong paths — the #1 cause of "my data disappeared" incidents.

### 5. 🔄 Restart Policy & Crash Count
Shows the restart policy (`always`, `on-failure:5`, `no`) and how many times the container has crashed. A red `(crashed 3x)` alert makes restart loops impossible to miss.

### 6. 🚧 Resource Limits
Displays CPU and Memory limits. **Containers with no limits show a red `unlimited ⚠` warning**, making it trivial to identify containers that could take down the entire host.

## 📊 Prometheus — Exported Metrics

The agent exposes metrics in Prometheus format at `http://localhost:9110/metrics`.

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

All metrics have the following labels: `container_id`, `container_name`, `image`.

### PromQL Query Example

```promql
# CPU for a specific container
castle_rock_container_cpu_percent{container_name="postgres"}

# Top 5 containers by memory
topk(5, castle_rock_container_memory_percent)

# Total network traffic
sum(castle_rock_container_network_rx_bytes) by (container_name)
```

### Test Locally

```bash
# While the agent is running (make run or docker compose up)
curl -s http://localhost:9110/metrics | grep castle_rock

# Health check
curl http://localhost:9110/health
```

---

## 📈 Grafana — 5 Pre-configured Dashboards

Docker Compose automatically provisions **5 dashboards** in Grafana. They all update in real-time (5s).

**Access:** http://localhost:3000 → Login: `admin` / `castlerock`

---

### Dashboard 1: Overview

A broad overview of all containers on a single screen.

| Panel | Type | Description |
|---|---|---|
| Active Containers | Stat | Total monitored containers count |
| Average CPU | Stat | Average CPU% across all containers |
| Average Memory | Stat | Average Memory% across all containers |
| Total Memory Used | Stat | Sum of RAM used by all (in bytes) |
| Total Network Traffic | Stat | Sum of RX + TX from all containers |
| CPU % per Container | Time Series | CPU history with smooth lines and gradients |
| CPU Gauge | Gauge | Speedometers with green/yellow/red thresholds |
| Memory % | Time Series | Memory history with thresholds |
| Memory Usage (stacked) | Time Series | Stacked usage — shows each container's contribution |
| Memory Bar Gauge | Bar Gauge | Horizontal bars with gradients per container |
| Network RX | Time Series | Bytes received per container |
| Network TX | Time Series | Bytes transmitted per container |
| Disk Read | Time Series | Disk reads per container |
| Disk Write | Time Series | Disk writes per container |
| Monitored Containers | Table | Filterable table with ID, name, and image |

---

### Dashboard 2: Container Detail

A deep dive into **a specific container**. A dropdown at the top allows you to select which container to analyze.

| Panel | Type | Description |
|---|---|---|
| Current CPU | Gauge | Real-time CPU% with thresholds |
| Current Memory | Gauge | Real-time Memory% with thresholds |
| RAM Used | Stat | Memory used in bytes |
| RAM Limit | Stat | Configured memory limit |
| Network Total | Stat | Total traffic (RX + TX) |
| CPU % History | Time Series | CPU over time with warning/critical zones |
| Memory % History | Time Series | Memory over time with alert zones |
| Memory Usage vs Limit | Time Series | Two lines: actual usage vs configured limit |
| Network I/O | Time Series | Download (RX) and Upload (TX) on the same chart |
| Disk I/O | Time Series | Disk read and write |

---

### Dashboard 3: Network Analysis

Focused analysis on network traffic.

| Panel | Type | Description |
|---|---|---|
| Total Download ↓ | Stat | Sum of received bytes |
| Total Upload ↑ | Stat | Sum of transmitted bytes |
| Total Traffic | Stat | RX + TX combined |
| Containers with Network | Stat | How many containers have > 0 traffic |
| Download Time Series | Time Series | RX bytes over time |
| Download Top Containers | Bar Gauge | Ranking of highest downloaders |
| Upload Time Series | Time Series | TX bytes over time |
| Upload Top Containers | Bar Gauge | Ranking of highest uploaders |
| Stacked Traffic | Time Series | All RX/TX stacked to see the total |

---

### Dashboard 4: Memory Deep Dive

Deep memory analysis — useful to spot leaks and risk of OOM kills.

| Panel | Type | Description |
|---|---|---|
| Total RAM Used | Stat | Sum of memory used by all containers |
| Total RAM Limit | Stat | Sum of configured memory limits |
| Average Memory % | Stat | Average memory utilization |
| Max Memory % | Stat | Container with the highest memory usage |
| Memory % All | Time Series | History of all containers with threshold lines |
| Stacked Usage | Time Series | Stacked memory usage (who consumes more) |
| Memory % Ranking | Bar Gauge | Horizontal bars with green→red gradients |
| Usage vs Limit | Time Series | For each container: actual usage vs limit (as they approach, risk of OOM) |

---

### Dashboard 5: Alerts & Health

Health monitoring and threshold violations.

| Panel | Type | Description |
|---|---|---|
| Containers CPU > 80% | Stat | How many containers are in critical CPU state |
| Containers MEM > 85% | Stat | How many containers are in critical memory state |
| Containers CPU > 50% | Stat | How many containers are in CPU warning |
| Healthy Containers | Stat | How many are below 50% in CPU and memory |
| CPU Ranking | Bar Gauge | Bars from highest to lowest CPU usage |
| Memory Ranking | Bar Gauge | Bars from highest to lowest memory usage |
| Max CPU | Time Series | CPU peak over time with alert zones |
| Max Memory | Time Series | Memory peak with OOM risk |
| Average vs Max CPU | Time Series | Compares average with peak — points out outliers |
| Average vs Max Memory | Time Series | Same comparison for memory |

---

## ⚠️ Configurable Alerts

The alert system works in **two layers**:

### Layer 1: TUI Alerts (Internal)

Defined in `configs/config.yaml`, evaluated by the agent's internal engine:

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

    - name: "High Memory"
      metric: "memory_percent"
      operator: ">"
      threshold: 70.0
      duration: 5m
      severity: "warning"
```

When an alert triggers, it appears:
- 🚨 On the TUI status bar
- 📋 In the event log with details
- With a visual indicator on the container in the table

### Layer 2: Prometheus Alerts (External)

Defined in `deploy/prometheus/alert_rules.yml`, evaluated by Prometheus:

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

## ⚙️ Configuration

### `configs/config.yaml` File

```yaml
log_level: "info"           # debug, info, warn, error

prometheus:
  enabled: true
  port: 9110                # HTTP server port

stats:
  interval: 5s              # Metrics collection interval

alerts:
  enabled: true
  rules:                    # Alert rules (see Alerts section)
    - name: "Critical CPU"
      metric: "cpu_percent"
      operator: ">"
      threshold: 80.0
      duration: 2m
      severity: "critical"
```

### Environment Variables (Overrides)

Environment variables have **maximum priority** over the YAML file:

| Variable | Description | Default |
|---|---|---|
| `CASTLE_ROCK_LOG_LEVEL` | Log level | `info` |
| `CASTLE_ROCK_PROMETHEUS_PORT` | Prometheus port | `9110` |
| `CASTLE_ROCK_PROMETHEUS_ENABLED` | Enable/disable Prometheus | `true` |
| `CASTLE_ROCK_STATS_INTERVAL` | Collection interval | `5s` |
| `CASTLE_ROCK_ALERTS_ENABLED` | Enable/disable alerts | `true` |
| `CASTLE_ROCK_MODE` | `headless` = no TUI (Docker/K8s) | `` (TUI) |
| `CASTLE_ROCK_CLUSTER_MODE` | `standalone`, `leader`, `worker` | `standalone` |
| `CASTLE_ROCK_CLUSTER_HOST_ID` | Identifier in TUI/Grafana | Host native hostname |
| `CASTLE_ROCK_CLUSTER_LEADER_URL` | Target URL (worker mode) | `http://127.0.0.1:9110` |

### Default Docker Variables

| Variable | Description | Default |
|---|---|---|
| `DOCKER_HOST` | Docker daemon address | `unix:///var/run/docker.sock` |
| `DOCKER_API_VERSION` | API version | Auto-negotiated |

### Order of Precedence (12-Factor App)

```
1. Hardcoded defaults (Go code)
2. configs/config.yaml (partial overrides)
3. CASTLE_ROCK_* Environment variables (final overrides)
```

---

## 🐳 Docker Compose — Observability Stack

The `docker-compose.yml` sets up 3 services:

```
┌────────────────────────────────────────────────────────┐
│                  Docker Compose Stack                  │
│                                                        │
│  ┌──────────────┐   ┌────────────┐   ┌──────────────┐  │
│  │ Castle Rock  │ → │ Prometheus │ → │   Grafana    │  │
│  │ Agent :9110  │   │   :9090    │   │    :3000     │  │
│  │  (headless)  │   │ (scraping) │   │ (dashboards) │  │
│  └──────┬───────┘   └────────────┘   └──────────────┘  │
│         │                                              │
│         ▼                                              │
│   Docker Socket                                        │
│  (monitoring)                                          │
└────────────────────────────────────────────────────────┘
```

### Docker Socket and Permissions

The agent needs access to the Docker socket to monitor containers:

```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock:ro  # Read-only
```

> ⚠️ **Why `user: root` in docker-compose?**
>
> The Docker socket (`/var/run/docker.sock`) is owned by `root`. Monitoring agents such as **cAdvisor** (Google), **node-exporter** (Prometheus), and the **Datadog Agent** also run as root to access the socket.
>
> The volume is mounted as `:ro` (read-only) for security. The agent DOES NOT modify anything in the Docker daemon — it just reads information.
>
> In production, consider alternatives:
> - Docker rootless mode
> - Socket proxy like [tecnativa/docker-socket-proxy](https://github.com/Tecnativa/docker-socket-proxy)

### Headless Mode

In Docker Compose, the agent runs in **headless mode** (without TUI):

```bash
CASTLE_ROCK_MODE=headless  # Defined in docker-compose.yml
```

In this mode, the agent acts solely as a Prometheus metrics server, without attempting to open the interactive dashboard (which requires a TTY terminal).

---

## Makefile Targets

| Target | Description |
|---|---|
| `make build` | Compiles the optimized binary |
| `make run` | Compiles and runs (interactive TUI) |
| `make test` | Runs tests with `-race` and coverage |
| `make lint` | Static analysis (`go vet`) |
| `make clean` | Removes binaries |
| `make tidy` | Organizes dependencies |
| `make docker-build` | Builds Docker image |
| `make docker-run` | Runs via Docker |

---

## 🧪 Testing

```bash
# Run all tests
go test ./... -v

# With coverage
go test ./... -cover

# With race detector
go test ./... -race
```

### Test Coverage

| Package | Tests | Coverage |
|---|---|---|
| `internal/tui` | formatBytes, truncate, min | Metrics formatting |
| `internal/alerts` | evaluateCondition, metrics, fire/resolve | Complete alert engine |
| `internal/config` | defaults, YAML loading, env overrides | Config loader |

---

## Troubleshooting

### ❌ `Agreeing to the Xcode and Apple SDKs license requires admin privileges`

```bash
sudo xcodebuild -license accept
```

If Xcode is not installed: `xcode-select --install`

### ❌ `Cannot connect to the Docker daemon`

- **macOS**: Open **Docker Desktop** and wait for the icon to turn green
- **Linux**: `sudo systemctl start docker`
- **Verify**: `docker info`

### ❌ `unable to get image [...]: Cannot connect to the Docker daemon`

This common error (especially on macOS) means the tool couldn't find the Docker socket at the expected path (e.g., `~/.docker/run/docker.sock`).
- Verify if **Docker Desktop** (or OrbStack/Colima) is open and fully loaded.
- In Docker Desktop, go to `Settings` > `Advanced` and check **"Allow the default Docker socket to be used"** (may require password).
- If using **OrbStack**, run in your terminal: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock"`

### ❌ `permission denied` on Docker socket

**On host (Linux):**
```bash
sudo usermod -aG docker $USER
newgrp docker  # or re-login
```

**In Docker Compose:**
Already addressed with `user: root` in `docker-compose.yml`.

### ❌ `go: command not found`

```bash
# macOS
brew install go

# Linux
wget https://go.dev/dl/go1.24.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.24.0.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
```

### ⚠️ `make: *** [run] Error 1` after Ctrl+C

**Normal behavior.** Go exits with exit code 130 (SIGINT) and `make` interprets it as an error. The agent shut down correctly (graceful shutdown).

### ⚠️ Containers don't show up

1. Check if containers are running: `docker ps`
2. Check Docker context: `docker context ls`
3. Restart agent: `make run`

### ⚠️ Grafana shows no data

1. Verify agent is running: `docker compose ps`
2. Check metrics: `curl http://localhost:9110/metrics`
3. Check Prometheus: http://localhost:9090/targets (status should be "UP")

---

## Technical Concepts

### Bubble Tea (Elm Architecture)

The dashboard uses the [Bubble Tea](https://github.com/charmbracelet/bubbletea) framework which follows the Elm architecture:

```
Model → Update → View (unidirectional cycle)
```

- **Model**: immutable struct with all state
- **Update**: pure function that processes messages and returns new state
- **View**: pure function that renders state as a string
- **Cmd**: async operations (Docker API, timers) that produce messages

### Docker Stats API — CPU% Calculation

The CPU% calculation uses the official Docker CLI formula:

```
cpuDelta = cpu_usage.total - pre_cpu_usage.total
systemDelta = system_cpu_usage - pre_system_cpu_usage
cpu% = (cpuDelta / systemDelta) × numCPUs × 100
```

### Context and Graceful Shutdown

All goroutines share a cancellable `context.Context`:

```
Ctrl+C → SIGINT → context cancelled → all goroutines exit
                                    → Docker client closes
                                    → HTTP server stops
                                    → resources freed via defer
```

### 12-Factor App — Configuration

The agent follows factor III (Config) of the [12-Factor App](https://12factor.net/config):
configuration separated from code, with precedence: defaults → YAML → env vars.

---

## License

MIT
