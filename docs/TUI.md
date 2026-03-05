# 🖥️ TUI — Interactive Dashboard

When running `make run`, the agent opens a fullscreen dashboard designed for seamless Docker monitoring and operations.

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

  ↑↓ nav │ space select │ enter details │ l log 1 │ L log N │ a all │ x shell │ C prune │ s stop │ R restart │ S stress │ M map │ ? help │ q quit
```

> 💡 **Context-Sensitive Help Bar:** The bottom bar automatically changes its shortcuts depending on the current screen. For example, when viewing logs it shows `↑↓ scroll │ / grep │ f tail │ E export │ Esc back`. When on the Prune Dashboard it shows `[i] images │ [v] volumes │ Esc back`.

---

## Keyboard Shortcuts

| Key | Action |
|---|---|
| `↑` / `k` | Navigate up (or scroll up logs) |
| `↓` / `j` | Navigate down (or scroll down logs) |
| `Enter` | Expand container details (metrics, labels, networks, ports) |
| `a` / `A` | Toggle visibility of stopped/inactive containers |
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

---

## Indicator Colors & Icons

### Resource Usage Colors
| Color | Meaning |
|---|---|
| 🟢 Green | Normal (CPU < 40%, MEM < 50%) |
| 🟡 Yellow | Warning (CPU 40-80%, MEM 50-80%) |
| 🔴 Red | Critical (CPU > 80%, MEM > 80%) |

### Status Icons
| Icon | Meaning |
|---|---|
| 🚨 | **Critical Alert:** Resource usage exceeded critical thresholds |
| ⚠️ | **Warning Alert:** Resource usage exceeded warning thresholds |
| 🛡️ | **Security Issue:** Anti-pattern detected (e.g., privileged mode, root user) |
| ❤️ | **Healthy:** Container health check is passing |
| 🩺 | **Unhealthy:** Container health check is failing |
| ⏳ | **Starting:** Container health check is initializing |

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
- **No Docker Proxy (Read-Only):** The test does not work in headless mode attached to `docker-socket-proxy`. The proxy has the `POST=0` flag locked for security.

---

## 📜 Advanced Logs Viewer

The TUI includes a native **Advanced Logs Viewer** packed with modern CLI features, reducing the need to leave the dashboard to inspect container output.

### 📖 How to read the Logs (`l`, `L`, `f`)

- **`l` (Log 1):** Use the arrow keys to place the `▸` cursor over a single container and press `l` (lowercase L) to tail its logs.
- **`L` (Log N / Multi-Tailing):** Navigate the table and press `Space` on multiple containers to select them. Then press `Shift+L`. The agent will aggregate their streams in real-time, placing colored `[container-name]` tags on each line.
- **`f` (Follow):** When a log screen is open, the view automatically scrolls (tails) to the bottom. If you press `↑`, scrolling pauses so you can read. Press `f` to collapse back to the live tail edge.

### Key Features
1. **🔍 Live Grep (Fuzzy Search):** Press `/` to open the search bar. The view will instantly filter logs as you type.
2. **🔀 Multi-Tailing:** Aggregate logs side-by-side using `Space` and `Shift+L`.
3. **⏪ History Pagination:** Scroll back without losing context.
4. **🎨 JSON Highlighting:** Automatically colorizes standard JSON log levels (`error`, `warn`, `info`).
5. **⏱️ Timestamps:** Renders exact ISO8601 timestamps in muted gray.
6. **📤 Quick Export:** Press `E` while viewing a log panel to save a snapshot directly to `/tmp/castle-rock-logs-[name]-[timestamp].txt`.

---

## 💼 Enterprise UX Tools

For advanced debugging, the agent includes tools designed like a high-end dashboard:

### 1. 🖥️ Host Health Metrics
The agent automatically reports the physical **Node / Host Operating System** CPU and Memory percentage in the top status bar.

### 2. 💻 Interactive Shell (Exec)
Selecting a container and pressing `x` suspends the Agent's UI and drops you directly into an **Interactive TTY Shell** (`/bin/sh` or `/bin/bash`) running inside the container. After typing `exit`, you smoothly return to the TUI.

### 3. 🧹 Interactive Prune Dashboard
Press **`C`** (Cleanup) to open the interactive prune dashboard. The engine displays exactly how much memory dangling images and idle local volumes occupy. Press `i` to prune images or `v` to prune volumes instantly.

---

## 🔎 Debug & Diagnostics

The container detail panel (`Enter`) includes advanced diagnostic tools:

### 1. 🔎 Environment Variables Inspector
Displays all environment variables configured inside the container (truncated at 50 characters). Invaluable for catching misconfigurations like a wrong `DATABASE_URL`.

### 2. 🏥 Health Check Status
Shows the container's formal Docker health badge + the **last health check output** (stdout), making it easy to understand *why* a container is unhealthy without running `docker inspect` manually.

### 3. ⚙️ Entrypoint / Command
Shows the exact command being executed by the container.

### 4. 📂 Volumes & Bind Mounts
Lists all mounted volumes and bind paths in the format `source → destination (type)`.

### 5. 🔄 Restart Policy & Crash Count
Shows the restart policy and how many times the container has crashed.

### 6. 🚧 Resource Limits
Displays CPU and Memory limits. **Containers with no limits show a red `unlimited ⚠` warning**, making it trivial to identify containers that could take down the entire host.
