// Package docker provides a wrapper over the official Docker SDK.
//
// This package encapsulates all interaction with the Docker daemon, following
// the Dependency Inversion Principle: business logic (collector) does not
// depend directly on the Docker SDK, but on abstractions defined here.
//
// Why use a wrapper?
//   - Makes unit testing easier (interfaces can be mocked)
//   - Centralizes configuration and error handling
//   - Allows swapping the implementation without affecting the rest of the system
//   - Encapsulates Docker API details (versioning, authentication)
package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	// Official Docker SDK — maintained by Docker Inc.
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"

	"github.com/nicolas-moura-ti/castle-rock-agent/internal/logger"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/metrics"
	"github.com/nicolas-moura-ti/castle-rock-agent/pkg/models"
)

// staticContainerData holds container information that does not change
// during the container's lifecycle, avoiding heavy Inspect calls.
type staticContainerData struct {
	Env           []string
	Entrypoint    string
	Command       string
	Mounts        []string
	RestartPolicy string
	CPULimit      float64
	MemoryLimit   int64
}

// Client wraps the official Docker client, adding high-level methods
// specific to the Castle Rock Agent.
type Client struct {
	cli *client.Client
	includeContainers []string

	metadataCache   map[string]staticContainerData
	metadataCacheMu sync.RWMutex
}

// NewClient creates a new Client instance connected to the local Docker daemon.
func NewClient() (*Client, error) {
	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("docker.NewClient: failed to create client: %w", err)
	}

	return &Client{
		cli:           cli,
		metadataCache: make(map[string]staticContainerData),
	}, nil
}

// SetIncludeContainers configuration on the client.
func (c *Client) SetIncludeContainers(includes []string) {
	c.includeContainers = includes
}

// isMonitored checks if the container should be monitored.
func (c *Client) isMonitored(name string) bool {
	if len(c.includeContainers) == 0 {
		return true // No filter, monitor all
	}
	for _, include := range c.includeContainers {
		if strings.Contains(name, include) {
			return true
		}
	}
	return false
}

// Close closes the connection to the Docker daemon.
//
// ALWAYS call Close() when done using the client.
// The idiomatic Go pattern is to use defer right after creation:
//
//	client, err := docker.NewClient()
//	if err != nil { ... }
//	defer client.Close()
func (c *Client) Close() error {
	return c.cli.Close()
}

// StopContainer stops a container by name or ID.
//
// TIMEOUT:
//
//	Docker first sends SIGTERM to PID 1 of the container.
//	If the process does not exit within the timeout, it sends SIGKILL.
//	We use 10 seconds as the default timeout — same as `docker stop`.
func (c *Client) StopContainer(ctx context.Context, containerID string) error {
	timeout := 10 // seconds
	stopOpts := container.StopOptions{Timeout: &timeout}

	if err := c.cli.ContainerStop(ctx, containerID, stopOpts); err != nil {
		return fmt.Errorf("docker.StopContainer: %w", err)
	}
	return nil
}

// RestartContainer restarts a container.
//
// Docker performs stop + start atomically. The timeout applies
// only to the stop phase (SIGTERM → SIGKILL).
func (c *Client) RestartContainer(ctx context.Context, containerID string) error {
	timeout := 10
	restartOpts := container.StopOptions{Timeout: &timeout}

	if err := c.cli.ContainerRestart(ctx, containerID, restartOpts); err != nil {
		return fmt.Errorf("docker.RestartContainer: %w", err)
	}
	return nil
}

// InspectContainer returns detailed information about a container,
// including security settings, networking and volumes.
func (c *Client) InspectContainer(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	inspectJSON, err := c.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return types.ContainerJSON{}, fmt.Errorf("docker.InspectContainer: %w", err)
	}
	return inspectJSON, nil
}

// PruneSystem performs a safe cleanup (Garbage Collection):
// Removes stopped containers, empty networks, and dangling images.
func (c *Client) PruneSystem(ctx context.Context) (uint64, error) {
	var totalReclaimed uint64

	// Prune Containers (exited)
	containersPrune, err := c.cli.ContainersPrune(ctx, filters.Args{})
	if err == nil {
		totalReclaimed += containersPrune.SpaceReclaimed
	}

	// Prune Networks
	_, _ = c.cli.NetworksPrune(ctx, filters.Args{})

	// Prune Images (dangling by default)
	imagesPrune, err := c.cli.ImagesPrune(ctx, filters.Args{})
	if err == nil {
		totalReclaimed += imagesPrune.SpaceReclaimed
	}

	return totalReclaimed, nil
}

// ListNetworks lists detailed Docker connectivity information.
func (c *Client) ListNetworks(ctx context.Context) ([]network.Inspect, error) {
	nets, err := c.cli.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("docker.ListNetworks: %w", err)
	}

	results := make([]network.Inspect, len(nets))
	var wg sync.WaitGroup

	for i, n := range nets {
		wg.Add(1)
		// Launch a goroutine for each network inspect
		// We capture 'n' as a parameter to avoid race condition
		go func(i int, netID string) {
			defer wg.Done()

			// Inspect each one to get dynamically connected containers
			insp, err := c.cli.NetworkInspect(ctx, netID, network.InspectOptions{})
			if err == nil {
				results[i] = insp
			}
		}(i, n.ID)
	}

	wg.Wait()

	// Filter out failed inspections (empty structs)
	var filteredResults []network.Inspect
	for _, res := range results {
		if res.ID != "" {
			filteredResults = append(filteredResults, res)
		}
	}

	return filteredResults, nil
}

// StreamContainerLogs returns a channel with the last lines of log
// from a container, followed by real-time logs (tail -f).
//
// DOCKER LOGS API:
//
//	The /containers/{id}/logs API with Follow:true keeps the connection
//	open and sends new lines as the container produces output.
//	We use Tail:"50" to show the last 50 lines first.
//
// CHANNEL PATTERN:
//
//	We return a string channel to integrate with the Bubble Tea
//	message loop. Each log line becomes a message.
func (c *Client) StreamContainerLogs(ctx context.Context, containerID string) (<-chan string, error) {
	reader, err := c.cli.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Tail:       "50",
		Timestamps: true,
	})
	if err != nil {
		return nil, fmt.Errorf("docker.StreamContainerLogs: %w", err)
	}

	logCh := make(chan string, 100) // Buffer of 100 lines

	go func() {
		defer close(logCh)
		defer reader.Close()

		// Docker logs have an 8-byte header per line in multiplexed mode.
		// We read byte by byte to extract complete lines.
		buf := make([]byte, 8192)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				n, err := reader.Read(buf)
				if err != nil {
					return
				}
				if n > 0 {
					processLogChunk(n, buf, logCh)
				}
			}
		}
	}()

	return logCh, nil
}

// processLogChunk splits a multiplexed log chunk into lines and sends them to the channel.
func processLogChunk(n int, buf []byte, logCh chan<- string) {
	// Remove 8-byte header from Docker multiplexed stream
	content := string(buf[:n])
	// Split by lines and send each one
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		// Clean Docker header control characters
		cleaned := cleanLogLine(line)
		if cleaned != "" {
			select {
			case logCh <- cleaned:
			default:
				// Buffer full, discard old line
			}
		}
	}
}

// cleanLogLine removes the 8-byte header from Docker multiplexed stream.
func cleanLogLine(line string) string {
	// Docker header has 8 bytes: [stream_type, 0, 0, 0, size_bytes...]
	// If the line starts with byte <= 2, it probably has a header
	if len(line) > 8 && (line[0] == 1 || line[0] == 2 || line[0] == 0) {
		return strings.TrimSpace(line[8:])
	}
	return strings.TrimSpace(line)
}

// SystemDiskUsage represents a simplified summary of Docker disk consumption on the host.
type SystemDiskUsage struct {
	ImagesReclaimable  int64
	VolumesReclaimable int64
}

// GetDiskUsage returns Docker disk usage (equivalent to docker system df).
func (c *Client) GetDiskUsage(ctx context.Context) (SystemDiskUsage, error) {
	du, err := c.cli.DiskUsage(ctx, types.DiskUsageOptions{})
	if err != nil {
		return SystemDiskUsage{}, fmt.Errorf("docker.GetDiskUsage: %w", err)
	}

	var imgReclaim, volReclaim int64

	for _, img := range du.Images {
		if img.Containers == 0 { // Image not used by any container
			imgReclaim += img.SharedSize + (img.Size - img.SharedSize)
		}
	}

	for _, vol := range du.Volumes {
		if vol.UsageData != nil && vol.UsageData.RefCount == 0 {
			volReclaim += vol.UsageData.Size
		}
	}

	return SystemDiskUsage{
		ImagesReclaimable:  imgReclaim,
		VolumesReclaimable: volReclaim,
	}, nil
}

// PruneImages deletes all orphaned images (dangling = true).
func (c *Client) PruneImages(ctx context.Context) (uint64, error) {
	report, err := c.cli.ImagesPrune(ctx, filters.NewArgs(filters.Arg("dangling", "true")))
	if err != nil {
		return 0, err
	}
	return report.SpaceReclaimed, nil
}

// PruneVolumes deletes local volumes not attached to any container.
func (c *Client) PruneVolumes(ctx context.Context) (uint64, error) {
	report, err := c.cli.VolumesPrune(ctx, filters.NewArgs())
	if err != nil {
		return 0, err
	}
	return report.SpaceReclaimed, nil
}

// DockerEvent represents a container lifecycle event.
//
// Docker events are the native mechanism for real-time monitoring.
// Instead of periodic polling (slow and inefficient), the Docker daemon
// sends events via HTTP streaming when containers change state.
//
// Relevant event types for observability:
//   - "start"   → container started
//   - "stop"    → container was stopped (graceful)
//   - "die"     → container terminated (could be a crash)
//   - "create"  → container was created (may not be running)
//   - "destroy" → container was removed
//   - "pause"   → container was paused
//   - "unpause" → container was resumed
type DockerEvent struct {
	// Action is the event type (start, stop, die, create, destroy, etc.)
	Action string

	// ContainerID is the full ID of the affected container.
	ContainerID string

	// ContainerName is the container name (without "/" prefix).
	ContainerName string

	// Image is the container's Docker image.
	Image string
}

// WatchEvents listens for container events from the Docker daemon in real-time.
//
// ARCHITECTURE — Event-Driven vs Polling:
//
//	This function uses the Docker Events API (HTTP streaming) to receive
//	instant notifications of changes. This is MUCH more efficient
//	than periodic polling because:
//	  - Zero CPU when no events occur
//	  - Minimal latency (milliseconds vs polling seconds)
//	  - Fewer calls to the Docker API
//
// CHANNELS IN GO:
//
//	The function returns a channel (<-chan DockerEvent) which is Go's
//	native mechanism for communication between goroutines. The caller
//	reads events from this channel using range or select.
//
// PATTERN — Context-based lifecycle:
//
//	The channel is automatically closed when the context is cancelled
//	(Ctrl+C or SIGTERM). This guarantees the internal goroutine does
//	not leak, even in error scenarios.
func (c *Client) WatchEvents(ctx context.Context) (<-chan DockerEvent, <-chan error) {
	eventCh := make(chan DockerEvent)
	errCh := make(chan error, 1) // buffer 1 to avoid blocking the goroutine

	// Filter only container events (ignore images, volumes, networks).
	// This reduces event volume and required processing.
	msgs, errs := c.cli.Events(ctx, types.EventsOptions{
		Filters: filters.NewArgs(
			filters.Arg("type", "container"),
			filters.Arg("event", "start"),
			filters.Arg("event", "stop"),
			filters.Arg("event", "die"),
			filters.Arg("event", "create"),
			filters.Arg("event", "destroy"),
			filters.Arg("event", "pause"),
			filters.Arg("event", "unpause"),
		),
	})

	// GOROUTINE — Async event processing:
	//   We launch a goroutine to read events from the Docker daemon
	//   and forward them to the output channel. The goroutine terminates
	//   when the context is cancelled (msgs channel is closed by the SDK).
	//
	//   RULE: every goroutine must have a clear exit condition.
	//   Here, exit is guaranteed by ctx.Done() which closes msgs.
	go func() {
		defer close(eventCh)
		defer close(errCh)

		for {
			select {
			case <-ctx.Done():
				return
			case err, ok := <-errs:
				if !ok {
					return
				}
				errCh <- err
				return
			case msg, ok := <-msgs:
				if !ok {
					return
				}
				// Extract container name from the event's "name" attribute.
				name := msg.Actor.Attributes["name"]

				if !c.isMonitored(name) {
					continue
				}

				eventCh <- DockerEvent{
					Action:        string(msg.Action),
					ContainerID:   msg.Actor.ID[:12],
					ContainerName: name,
					Image:         msg.Actor.Attributes["image"],
				}
			}
		}
	}()

	return eventCh, errCh
}

// GetAllContainerStats returns performance metrics for ALL running
// containers. Collects stats from each container in parallel.
//
// DOCKER STATS API:
//
//	The /containers/{id}/stats API returns a JSON stream with real-time
//	metrics. We use Stream:false to get only a snapshot (no continuous
//	stream), which is adequate for periodic polling.
//
// CONCURRENCY WITH GOROUTINES:
//
//	We collect stats from all containers in parallel using goroutines
//	+ sync.WaitGroup. This reduces total latency from N*RTT to ~1*RTT
//	(where RTT is the round-trip time of one API call).
func (c *Client) GetAllContainerStats(ctx context.Context, containers []models.ContainerInfo) (map[string]models.ContainerMetrics, error) {
	// Thread-safe map for results
	results := make(map[string]models.ContainerMetrics)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, cont := range containers {
		if !c.isMonitored(cont.Name) {
			continue
		}

		wg.Add(1)

		// Launch a goroutine for each container.
		// We capture 'cont' as parameter to avoid race condition
		// (shared loop variable).
		go func(ctr models.ContainerInfo) {
			defer wg.Done()

			stats, err := c.getContainerStats(ctx, ctr.ID)
			if err != nil {
				return // Ignore containers that failed (may have stopped)
			}

			stats.ContainerID = ctr.ID
			if len(stats.ContainerID) > 12 {
				stats.ContainerID = stats.ContainerID[:12]
			}
			stats.ContainerName = ctr.Name
			stats.Image = ctr.Image

			mu.Lock()
			results[stats.ContainerID] = stats
			mu.Unlock()
		}(cont)
	}

	wg.Wait()
	return results, nil
}

// getContainerStats collects metrics for ONE container via Docker Stats API.
//
// CPU% CALCULATION:
//
//	The Docker daemon returns accumulated counters (total CPU nanoseconds
//	used since container start). To calculate percentage,
//	we would need two snapshots to compute the delta. Since we use
//	Stream:false (single snapshot), we compute an approximation using
//	the system counters.
//
//	Official formula (same used by `docker stats`):
//	  cpuDelta = container_cpu_usage - pre_container_cpu_usage
//	  systemDelta = system_cpu_usage - pre_system_cpu_usage
//	  cpu% = (cpuDelta / systemDelta) * numCPUs * 100
func (c *Client) getContainerStats(ctx context.Context, containerID string) (models.ContainerMetrics, error) {
	// Stream: false returns a single snapshot (not a continuous stream).
	// This is more efficient for periodic polling.
	resp, err := c.cli.ContainerStats(ctx, containerID, false)
	if err != nil {
		return models.ContainerMetrics{}, fmt.Errorf("stats: %w", err)
	}
	defer resp.Body.Close()

	var statsJSON types.StatsJSON
	if err := json.NewDecoder(resp.Body).Decode(&statsJSON); err != nil {
		return models.ContainerMetrics{}, fmt.Errorf("decode stats: %w", err)
	}

	// CPU percentage calculation (docker stats CLI formula)
	cpuPercent := calculateCPUPercent(&statsJSON)

	// Memory calculation
	memUsage := statsJSON.MemoryStats.Usage
	memLimit := statsJSON.MemoryStats.Limit
	var memPercent float64
	if memLimit > 0 {
		memPercent = float64(memUsage) / float64(memLimit) * 100.0
	}

	// Network calculation (sum of all interfaces)
	var netRx, netTx uint64
	for _, net := range statsJSON.Networks {
		netRx += net.RxBytes
		netTx += net.TxBytes
	}

	// Block I/O calculation
	var blockRead, blockWrite uint64
	for _, bio := range statsJSON.BlkioStats.IoServiceBytesRecursive {
		switch bio.Op {
		case "read", "Read":
			blockRead += bio.Value
		case "write", "Write":
			blockWrite += bio.Value
		}
	}

	return models.ContainerMetrics{
		CPUPercent:    cpuPercent,
		MemoryUsage:   memUsage,
		MemoryLimit:   memLimit,
		MemoryPercent: memPercent,
		NetworkRx:     netRx,
		NetworkTx:     netTx,
		BlockRead:     blockRead,
		BlockWrite:    blockWrite,
	}, nil
}

// calculateCPUPercent calculates the CPU percentage using the
// official Docker CLI formula.
//
// REFERENCE: https://github.com/moby/moby/blob/master/api/types/stats.go
func calculateCPUPercent(stats *types.StatsJSON) float64 {
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage) -
		float64(stats.PreCPUStats.CPUUsage.TotalUsage)

	systemDelta := float64(stats.CPUStats.SystemUsage) -
		float64(stats.PreCPUStats.SystemUsage)

	if systemDelta > 0.0 && cpuDelta > 0.0 {
		numCPUs := float64(stats.CPUStats.OnlineCPUs)
		if numCPUs == 0 {
			numCPUs = float64(len(stats.CPUStats.CPUUsage.PercpuUsage))
		}
		if numCPUs == 0 {
			numCPUs = 1
		}
		return (cpuDelta / systemDelta) * numCPUs * 100.0
	}

	return 0.0
}

// GetSystemInfo returns detailed information about the Docker daemon.
//
// This information is essential for observability:
//   - Server and API version
//   - Host operating system and architecture
//   - Total available memory
//   - Number of containers and images
//
// In a production agent, this information would be exported as
// metrics (Prometheus gauges) and used for infrastructure dashboards.
func (c *Client) GetSystemInfo(ctx context.Context) (map[string]string, error) {
	// ServerVersion returns the Docker daemon version.
	// It is a lightweight call (HTTP GET /version) and useful for health checks.
	version, err := c.cli.ServerVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("docker.GetSystemInfo: failed to get version: %w", err)
	}

	// Info returns detailed Docker system information.
	// Includes: OS, architecture, memory, CPUs, containers, images, etc.
	info, err := c.cli.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("docker.GetSystemInfo: failed to get info: %w", err)
	}

	// Format total memory in human-readable format (GB).
	memoryGB := float64(info.MemTotal) / (1024 * 1024 * 1024)

	result := map[string]string{
		"Server Version": version.Version,
		"API Version":    version.APIVersion,
		"OS/Arch":        fmt.Sprintf("%s/%s", version.Os, version.Arch),
		"Kernel":         version.KernelVersion,
		"Total Memory":   fmt.Sprintf("%.1f GB", memoryGB),
		"Containers":     fmt.Sprintf("%d total (%d running, %d stopped, %d paused)", info.Containers, info.ContainersRunning, info.ContainersStopped, info.ContainersPaused),
		"Images":         fmt.Sprintf("%d", info.Images),
	}

	return result, nil
}

// ListRunningContainers returns a list of all running containers.
//
// PARAMETER ctx (context.Context):
//   - Allows operation cancellation (e.g. Ctrl+C, timeout, deadline)
//   - If the context is cancelled during the HTTP call to the Docker daemon,
//     the operation is aborted immediately and returns ctx.Err()
//   - GOLDEN RULE: every function that does I/O should accept a context
//     as its first parameter
//
// RETURN ([]models.ContainerInfo, error):
//   - Multi-value return is the Go standard for operations that can fail
//   - Never return (nil, nil) — always return data OR error
//   - On success with empty list, return ([]ContainerInfo{}, nil)
func (c *Client) ListRunningContainers(ctx context.Context, all bool) ([]models.ContainerInfo, error) {
	// container.ListOptions filters which containers to return.
	// All: false returns only running containers.
	// To include stopped containers, use All: true.
	containers, err := c.cli.ContainerList(ctx, container.ListOptions{
		All: all,
	})
	if err != nil {
		return nil, fmt.Errorf("docker.ListRunningContainers: failed to list: %w", err)
	}

	// Pre-allocate the slice with exact size using make([]T, 0, cap).
	// This avoids memory reallocations during append, improving
	// performance. In an observability agent, every allocation counts.
	result := make([]models.ContainerInfo, 0, len(containers))

	for _, ctr := range containers {
		info := toContainerInfo(ctr)
		if c.isMonitored(info.Name) {
			result = append(result, info)
		}
	}

	return result, nil
}

// ListRunningContainersDetailed returns detailed information for all
// running containers, including labels, networks, command and creation
// time. Used for advanced logging.
//
// This version is heavier than ListRunningContainers because it collects
// additional data. Use it for display; use the simpler version for
// metrics collection where performance is critical.
func (c *Client) ListRunningContainersDetailed(ctx context.Context, all bool) ([]logger.ContainerDisplay, error) {
	containers, err := c.cli.ContainerList(ctx, container.ListOptions{
		All: all,
	})
	if err != nil {
		return nil, fmt.Errorf("docker.ListRunningContainersDetailed: failed to list: %w", err)
	}

	// First pass: filter monitored containers and create base display structs
	monitored := make([]logger.ContainerDisplay, 0, len(containers))
	ids := make([]string, 0, len(containers))
	for _, ct := range containers {
		cd := toContainerDisplay(ct)
		if c.isMonitored(cd.Name) {
			monitored = append(monitored, cd)
			ids = append(ids, ct.ID)
		}
	}

	if len(monitored) == 0 {
		metrics.MonitoredContainers.Set(0)
		return monitored, nil
	}

	metrics.MonitoredContainers.Set(float64(len(monitored)))

	// Second pass: fetch details concurrently
	var wg sync.WaitGroup
	wg.Add(len(monitored))

	for i := range monitored {
		go func(idx int) {
			defer wg.Done()
			c.enrichContainerDetails(ctx, ids[idx], &monitored[idx])
		}(i)
	}

	wg.Wait()

	return monitored, nil
}

// enrichContainerDetails acquires metadata from ContainerInspect and fills the display struct.
func (c *Client) enrichContainerDetails(ctx context.Context, id string, cd *logger.ContainerDisplay) {
	c.metadataCacheMu.RLock()
	cached, exists := c.metadataCache[id]
	c.metadataCacheMu.RUnlock()

	if exists {
		metrics.MetadataCacheHits.Inc()
		cd.Env = cached.Env
		cd.Entrypoint = cached.Entrypoint
		// Command is already partially populated from ContainerList, but we use the cached exact one
		if cached.Command != "" {
			cd.Command = cached.Command
		}
		cd.Mounts = cached.Mounts
		cd.RestartPolicy = cached.RestartPolicy
		cd.CPULimit = cached.CPULimit
		cd.MemoryLimit = cached.MemoryLimit
		// HealthStatus and RestartCount are dynamic, but avoiding the heavy inspect on every tick
		// is worth the trade-off. Their basic states are often inferred via the Status field from ContainerList.
		return
	}

	metrics.MetadataCacheMisses.Inc()
	inspect, err := c.cli.ContainerInspect(ctx, id)
	if err != nil {
		return
	}

	c.enrichConfigDetails(&inspect, cd)
	c.enrichHealthDetails(&inspect, cd)
	c.enrichHostConfigDetails(&inspect, cd)
	c.enrichMountDetails(&inspect, cd)

	c.metadataCacheMu.Lock()
	c.metadataCache[id] = staticContainerData{
		Env:           cd.Env,
		Entrypoint:    cd.Entrypoint,
		Command:       cd.Command,
		Mounts:        cd.Mounts,
		RestartPolicy: cd.RestartPolicy,
		CPULimit:      cd.CPULimit,
		MemoryLimit:   cd.MemoryLimit,
	}
	c.metadataCacheMu.Unlock()
}

func (c *Client) enrichConfigDetails(inspect *types.ContainerJSON, cd *logger.ContainerDisplay) {
	if inspect.Config == nil {
		return
	}

	// Redact sensitive environment variables
	var redactedEnv []string
	for _, env := range inspect.Config.Env {
		redactedEnv = append(redactedEnv, logger.Redact(env))
	}
	cd.Env = redactedEnv

	// Build and redact Entrypoint + Cmd
	fullCommand := ""
	if len(inspect.Config.Entrypoint) > 0 {
		fullCommand = strings.Join(inspect.Config.Entrypoint, " ")
	}
	if len(inspect.Config.Cmd) > 0 {
		cmdStr := strings.Join(inspect.Config.Cmd, " ")
		if fullCommand != "" {
			fullCommand += " " + cmdStr
		} else {
			fullCommand = cmdStr
		}
	}

	// Apply redaction to the full command line
	cd.Entrypoint = logger.Redact(fullCommand)

	if len(cd.Entrypoint) > 80 {
		cd.Entrypoint = cd.Entrypoint[:77] + "..."
	}
}

func (c *Client) enrichHealthDetails(inspect *types.ContainerJSON, cd *logger.ContainerDisplay) {
	if inspect.State == nil || inspect.State.Health == nil {
		return
	}
	cd.HealthStatus = string(inspect.State.Health.Status)
	if len(inspect.State.Health.Log) > 0 {
		last := inspect.State.Health.Log[len(inspect.State.Health.Log)-1]
		cd.HealthLog = strings.TrimSpace(last.Output)
		if len(cd.HealthLog) > 120 {
			cd.HealthLog = cd.HealthLog[:117] + "..."
		}
	}
}

func (c *Client) enrichHostConfigDetails(inspect *types.ContainerJSON, cd *logger.ContainerDisplay) {
	if inspect.HostConfig == nil {
		return
	}
	cd.RestartPolicy = string(inspect.HostConfig.RestartPolicy.Name)
	if inspect.HostConfig.RestartPolicy.MaximumRetryCount > 0 {
		cd.RestartPolicy += fmt.Sprintf(":%d", inspect.HostConfig.RestartPolicy.MaximumRetryCount)
	}
	if inspect.HostConfig.NanoCPUs > 0 {
		cd.CPULimit = float64(inspect.HostConfig.NanoCPUs) / 1e9
	}
	if inspect.HostConfig.Memory > 0 {
		cd.MemoryLimit = inspect.HostConfig.Memory
	}
	if inspect.RestartCount > 0 {
		cd.RestartCount = inspect.RestartCount
	}
}

func (c *Client) enrichMountDetails(inspect *types.ContainerJSON, cd *logger.ContainerDisplay) {
	for _, mt := range inspect.Mounts {
		label := fmt.Sprintf("%s → %s (%s)", mt.Source, mt.Destination, mt.Type)
		if len(label) > 80 {
			label = label[:77] + "..."
		}
		cd.Mounts = append(cd.Mounts, label)
	}
}

// toContainerInfo converts a Docker SDK container to our internal DTO.
//
// This function is unexported (lowercase) because it is an implementation detail.
// External callers do not need to know how we perform the conversion.
func toContainerInfo(c types.Container) models.ContainerInfo {
	// Docker container names start with "/" for historical reasons.
	// We remove the slash for cleaner display.
	name := ""
	if len(c.Names) > 0 {
		name = strings.TrimPrefix(c.Names[0], "/")
	}

	// Format exposed ports in a human-readable way.
	ports := formatPorts(c.Ports)

	return models.ContainerInfo{
		ID:     c.ID[:12], // Use only the first 12 characters (short ID)
		Name:   name,
		Image:  c.Image,
		Status: c.Status,
		State:  c.State,
		Ports:  ports,
	}
}

// toContainerDisplay converts a container to the detailed display format.
func toContainerDisplay(c types.Container) logger.ContainerDisplay {
	name := ""
	if len(c.Names) > 0 {
		name = strings.TrimPrefix(c.Names[0], "/")
	}

	// Extract names of networks connected to the container.
	networks := make([]string, 0)
	if c.NetworkSettings != nil {
		for netName := range c.NetworkSettings.Networks {
			networks = append(networks, netName)
		}
	}

	// Filter relevant labels (ignore Docker/Compose internal labels).
	labels := make(map[string]string)
	for k, v := range c.Labels {
		// Include only user or compose labels, ignore overly verbose
		// Docker Desktop internal labels.
		if !strings.HasPrefix(k, "desktop.docker.") &&
			!strings.HasPrefix(k, "org.opencontainers.") {
			labels[k] = v
		}
	}

	// Format the creation timestamp in a human-readable format.
	created := time.Unix(c.Created, 0).Format("2006-01-02 15:04:05")

	// Truncate command if too long for display.
	command := c.Command
	if len(command) > 60 {
		command = command[:57] + "..."
	}

	return logger.ContainerDisplay{
		ID:       c.ID[:12],
		Name:     name,
		Image:    c.Image,
		Status:   c.Status,
		State:    c.State,
		Command:  command,
		Ports:    formatPorts(c.Ports),
		Created:  created,
		Networks: networks,
		Labels:   labels,
	}
}

// formatPorts converts the Docker API port list into a
// human-readable representation (e.g. "0.0.0.0:8080->80/tcp").
func formatPorts(ports []types.Port) string {
	if len(ports) == 0 {
		return ""
	}

	// strings.Builder is more efficient than concatenation with + for
	// multiple strings, as it avoids intermediate allocations.
	var b strings.Builder

	for i, p := range ports {
		if i > 0 {
			b.WriteString(", ")
		}

		if p.PublicPort != 0 {
			fmt.Fprintf(&b, "%s:%d->%d/%s", p.IP, p.PublicPort, p.PrivatePort, p.Type)
		} else {
			fmt.Fprintf(&b, "%d/%s", p.PrivatePort, p.Type)
		}
	}

	return b.String()
}

// RunStressTest creates a temporary stress test container.
// We use alpine + stress-ng for compatibility with all architectures
// (arm64, amd64) and to avoid old manifest errors.
// The container self-removes after finishing.
//
// MODES:
//   - "cpu"    → stress-ng --cpu 2 --timeout Xs
//   - "memory" → stress-ng --vm 1 --vm-bytes 256M --timeout Xs
//   - "both"   → stress-ng --cpu 2 --vm 1 --vm-bytes 256M --timeout Xs
//
// DOCKER API (ContainerCreate + ContainerStart):
//
//	We use the same API that `docker run` uses internally.
//	AutoRemove:true ensures the container is removed after execution,
//	avoiding accumulation of stopped containers.
func (c *Client) RunStressTest(ctx context.Context, mode string, durationSec int) error {
	stressImage := "ghcr.io/alexei-led/stress-ng:latest"
	containerName := "castle-rock-stress"

	// Build stress-ng arguments
	var cmd []string
	switch mode {
	case "cpu":
		cmd = []string{"--cpu", "2", "--timeout", fmt.Sprintf("%ds", durationSec)}
	case "memory":
		// --vm-hang 0 = sleep after allocation to keep memory held without burning CPU
		cmd = []string{"--vm", "1", "--vm-bytes", "256M", "--vm-hang", "0", "--timeout", fmt.Sprintf("%ds", durationSec)}
	case "both":
		cmd = []string{"--cpu", "2", "--vm", "1", "--vm-bytes", "256M", "--vm-hang", "0", "--timeout", fmt.Sprintf("%ds", durationSec)}
	default:
		return fmt.Errorf("docker.RunStressTest: invalid mode: %s", mode)
	}

	// Try to remove previous container with the same name (if it exists)
	_ = c.cli.ContainerRemove(ctx, containerName, container.RemoveOptions{Force: true})

	// Pull image if not available locally
	_, _, err := c.cli.ImageInspectWithRaw(ctx, stressImage)
	if err != nil {
		reader, pullErr := c.cli.ImagePull(ctx, stressImage, image.PullOptions{})
		if pullErr != nil {
			return fmt.Errorf("docker.RunStressTest: failed to pull image %s: %w", stressImage, pullErr)
		}
		defer reader.Close()
		// Consume the reader to complete the pull
		buf := make([]byte, 4096)
		for {
			_, readErr := reader.Read(buf)
			if readErr != nil {
				break
			}
		}
	}

	// Create the container
	resp, err := c.cli.ContainerCreate(ctx,
		&container.Config{
			Image: stressImage,
			Cmd:   cmd,
		},
		&container.HostConfig{
			AutoRemove: true, // Automatically remove after finishing
		},
		nil, nil, containerName,
	)
	if err != nil {
		return fmt.Errorf("docker.RunStressTest: failed to create container: %w", err)
	}

	// Start the container
	if err := c.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("docker.RunStressTest: failed to start container: %w", err)
	}

	return nil
}
