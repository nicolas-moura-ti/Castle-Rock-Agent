# ⚙️ Configuration & Setup

Castle Rock Agent follows the **12-Factor App** methodology. Configuration is strictly separated from the code and heavily dependent on environment variables.

All configuration is loaded via Viper and supports both a YAML config file and ENV variables.

---

## The `configs/config.yaml` File

By default, the agent ships with a `configs/config.yaml` file defining the baseline defaults.

```yaml
log_level: "info"           # debug, info, warn, error

prometheus:
  enabled: true
  port: 9110                # HTTP server port

stats:
  interval: 5s              # Metrics collection interval

alerts:
  enabled: true
  rules:                    # Internal alert rules
    - name: "Critical CPU"
      metric: "cpu_percent"
      operator: ">"
      threshold: 80.0
      duration: 2m
      severity: "critical"
```

---

## Environment Variables (Overrides)

Environment variables have **maximum priority** and always override what is specified in the YAML file.
This means you don't even need a `config.yaml` if you inject everything through the environment.

| Variable | Description | Default |
|---|---|---|
| `CASTLE_ROCK_LOG_LEVEL` | Log level (`debug`, `info`, `warn`, `error`) | `info` |
| `CASTLE_ROCK_PROMETHEUS_PORT` | Prometheus HTTP exporter port | `9110` |
| `CASTLE_ROCK_PROMETHEUS_ENABLED` | Enable/disable Prometheus exporter | `true` |
| `CASTLE_ROCK_STATS_INTERVAL` | Collection frequency interval (e.g., `5s`) | `5s` |
| `CASTLE_ROCK_STATS_INCLUDE_CONTAINERS` | Comma-separated list of container names to monitor | `""` (all) |
| `CASTLE_ROCK_ALERTS_ENABLED` | Enable/disable internal TUI alerts | `true` |
| `CASTLE_ROCK_MODE` | Set to `headless` to disable TUI (used in Docker) | `""` (TUI enabled) |
| `CASTLE_ROCK_CLUSTER_MODE` | Role: `standalone`, `leader`, `worker` | `standalone` |
| `CASTLE_ROCK_CLUSTER_HOST_ID` | Identifier shown in TUI/Grafana | Host OS hostname |
| `CASTLE_ROCK_CLUSTER_LEADER_URL` | Target URL to push stats to (worker mode) | `http://127.0.0.1:9110` |
| `CASTLE_ROCK_CLUSTER_AUTH_TOKEN` | Bearer Token for API Authentication | `""` |
| `CASTLE_ROCK_CLUSTER_SHARED_SECRET` | Secret Key for AES-GCM (Argon2id) encryption | `""` |

---

## Default Docker Variables

Because Castle Rock Agent talks directly to the Docker daemon, it honors the official Docker SDK environment variables:

| Variable | Description | Default |
|---|---|---|
| `DOCKER_HOST` | Docker daemon address | `unix:///var/run/docker.sock` |
| `DOCKER_API_VERSION` | Minimum API version | Auto-negotiated |

---

## Order of Precedence 

The final configuration is computed following this strict precedence order:

1. **Hardcoded Defaults** inside the Go code structures.
2. Values parsed from **`configs/config.yaml`** (applies partial overrides over defaults).
3. **`CASTLE_ROCK_*` Environment variables** (final overrides over configuration file).

---

## 🎯 Selective Container Monitoring (Filtering)

By default, the agent connects to the Docker socket and tracks **every** running container on the host. 
However, you can explicitly filter which containers the agent will monitor. 

This is highly recommended in production to avoid polluting your Prometheus metrics and Grafana with sidecars or other noisy infrastructure-level containers.

To monitor specific containers, define a list of names or substrings (like `postgres` or `api`). Only containers whose names contain at least one of these strings will be monitored, tracked, and displayed on the TUI.

**Via config.yaml**:
```yaml
stats:
  interval: 5s
  include_containers: ["postgres", "redis", "my-api"]
```

**Via Environment Variable (Comma-Separated)**:
```bash
# Only tracks the postgres database and the redis cache
CASTLE_ROCK_STATS_INCLUDE_CONTAINERS="postgres,redis"
```
