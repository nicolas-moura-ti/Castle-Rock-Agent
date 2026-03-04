<div align="center">

# 🏰 Castle Rock Agent

[![Go Report Card](https://goreportcard.com/badge/github.com/nicolas-moura-ti/castle-rock-agent?style=flat-square)](https://goreportcard.com/report/github.com/nicolas-moura-ti/castle-rock-agent)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg?style=flat-square)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-v1.24.2-blue?style=flat-square&logo=go)](https://github.com/nicolas-moura-ti/castle-rock-agent)

\
*Read this in other languages: [English](README.md) · [Português](README.pt-BR.md)*

> A Go-native observability agent for monitoring Docker containers with an interactive dashboard, Prometheus metrics, and configurable alerts.

</div>

---

### ⛰️ Why "Castle Rock"?
Inspired by medieval watchtowers built on rocky peaks (Castle Rocks), which offered an **absolute panoramic view** of everything happening around the castle. Just like these towers, this agent sits at a privileged observation point (the Docker Socket) to monitor, watch, and alert on the health of your entire container infrastructure.

---

## 📖 Complete Documentation Setup

For deep dives into modules and features, please reference our documentation directory:

- 🖥️ **[Interactive Dashboard (TUI) & Features Reference](docs/TUI.md)** 
  *(Stress Testing, Shell execution, Pruning, Logs Tailing, Security Scans)*
- 📈 **[Observability (Prometheus & Grafana) & Alerts Setup](docs/OBSERVABILITY.md)** 
  *(Metrics List, Grafana dashboards, Cluster Mode, Alert Configurations)*
- ⚙️ **[Configuration & Environment Variables](docs/CONFIGURATION.md)**
  *(Selective filtering, Precedence, Defaults)*

---

## 🧠 How It Works — Simple Explanation

The observability workflow leverages 3 main pieces working together:

1. **Castle Rock Agent (this project):** It connects to Docker, retrieves metrics/logs using the SDK, calculates CPU and RAM footprints manually, and acts as the **collector**.
2. **Prometheus:** Acts as the **time-series database**. Every 5 seconds it scrapes metrics from the Castle Rock Agent (`http://agent:9110/metrics`) and records them historically.
3. **Grafana:** The **visualization panel**. It queries Prometheus and renders visually pleasing CPU/Memory metrics and network graphs.

```
Docker Containers → Castle Rock Agent → Prometheus → Grafana
(generate metrics)  (collects & trans)  (stores)     (visualizes)
```

---

## ✨ Top Features

| Feature | Description |
|---|---|
| **Interactive TUI** | Fullscreen dashboard with container tables, real-time metrics, events and live logs |
| **Real-time Metrics** | CPU%, Memory%, Network I/O, Disk I/O exported continuously |
| **Grafana Dashboard** | 5 pre-configured dashboards shipped natively (CPU, Mem, Network, Alerts) |
| **Alerts Engine** | Double-layer alerts (Internal on TUI -> Notification via Alertmanager) |
| **Cluster Mode 🌐** | Leader/Worker remote architecture to track several host nodes simultaneously |
| **Security Auditing 🛡️** | Scan for Privileged mode, ROOT privileges, public ports instantly |
| **Auto Prune 🧹** | Smart GARBAGE COLLECTOR to evict orphaned Docker objects natively |

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

---

## Prerequisites

| Dependency | Min Version | 
|---|---|
| **Go** | 1.24+ | 
| **Docker** | 20.10+ | 
| **Make** | Any | 

*(macOS note: if running locally, ensure you accept the Xcode license: `sudo xcodebuild -license accept`)*

---

## Quick Start

### Mode 1: Local TUI (Development)

Runs directly on your machine and attaches to the local docker daemon via a beautiful terminal dashboard.

```bash
# Clone
git clone https://github.com/nicolas-moura-ti/castle-rock-agent.git
cd castle-rock-agent

# Run (opens interactive dashboard)
make run
```

### Mode 2: Docker Compose (Full Observability Stack)

Spins up the **Headless Agent** + **Prometheus** + **Grafana** in isolated containers. Best for 24/7 background monitoring.

```bash
docker compose up -d

# Access:
# - Grafana:    http://localhost:3000  (admin / castlerock)
# - Prometheus: http://localhost:9090
# - Metrics:    http://localhost:9110/metrics
```

> 💡 **PARALLEL USE:** Keep `docker compose up -d` running in the background collecting metrics for Grafana. Whenever you want to inspect deeply or test loads ("Stress Test"), open a terminal and run `make run`. One does not interfere with the other.

---

## 🧪 Development & Testing

```bash
make test          # Tests with -race and coverage
make lint          # Static analysis (go vet)
make build         # Compiles optimized binary
make docker-build  # Builds alpine Docker image
```

### Coverage (Key Modules Tested):
- `internal/tui`: Metrics formatting 
- `internal/alerts`: Complete alert engine evaluation
- `internal/config`: Defaults, YAML loading, and Environment override logic

---

## License

MIT
