// Package models defines the DTOs (Data Transfer Objects) and public
// structs for the Castle Rock Agent.
//
// This package lives in pkg/ (not internal/) because it can be imported
// by other projects that want to consume the agent's data.
//
// GO CONVENTION:
//   - pkg/ = public code, reusable by external projects
//   - internal/ = private code, restricted to the current module
//   - The distinction is enforced by the Go compiler (not just convention)
package models

// ContainerInfo represents basic information about a Docker container.
//
// This struct is the main DTO for container listing.
// It contains only identification and state data — performance metrics
// are stored in ContainerMetrics.
//
// GO STRUCT BEST PRACTICES:
//   - Exported fields (uppercase) for JSON serialization
//   - JSON tags to control names in serialized output
//   - Documentation on each field for API self-documentation
type ContainerInfo struct {
	// HostID identifies which machine the container is running on.
	HostID string `json:"host_id,omitempty"`

	// ID is the container's unique identifier (short ID, 12 characters).
	ID string `json:"id"`

	// Name is the name assigned to the container (without the "/" prefix).
	Name string `json:"name"`

	// Image is the Docker image name used by the container.
	Image string `json:"image"`

	// Status is the human-readable state description (e.g. "Up 2 hours").
	Status string `json:"status"`

	// State is the technical container state (running, exited, paused, etc.).
	State string `json:"state"`

	// Ports are the exposed ports formatted for display (e.g. "0.0.0.0:8080->80/tcp").
	Ports string `json:"ports,omitempty"`
}

// ContainerMetrics represents the performance metrics of a container.
//
// This DTO is populated by the collector using the Docker Stats API.
// The separation between ContainerInfo and ContainerMetrics follows the
// Single Responsibility Principle: info is static, metrics are dynamic.
type ContainerMetrics struct {
	// HostID identifies which machine the metrics came from.
	HostID string `json:"host_id,omitempty"`

	// ContainerID identifies the container these metrics belong to.
	ContainerID string `json:"container_id"`

	// ContainerName is the container name (for metric labels).
	ContainerName string `json:"container_name"`

	// Image is the container's Docker image.
	Image string `json:"image"`

	// CPUPercent is the container's CPU usage percentage.
	CPUPercent float64 `json:"cpu_percent"`

	// MemoryUsage is the memory usage in bytes.
	MemoryUsage uint64 `json:"memory_usage"`

	// MemoryLimit is the configured memory limit in bytes.
	MemoryLimit uint64 `json:"memory_limit"`

	// MemoryPercent is the memory usage percentage.
	MemoryPercent float64 `json:"memory_percent"`

	// NetworkRx is the total bytes received over the network.
	NetworkRx uint64 `json:"network_rx"`

	// NetworkTx is the total bytes transmitted over the network.
	NetworkTx uint64 `json:"network_tx"`

	// BlockRead is the total bytes read from disk.
	BlockRead uint64 `json:"block_read"`

	// BlockWrite is the total bytes written to disk.
	BlockWrite uint64 `json:"block_write"`
}

// PushPayload is the data package sent by a Worker to the Leader.
// It groups the state of all containers at once (snapshot).
type PushPayload struct {
	HostID     string             `json:"host_id"`
	Containers []ContainerInfo    `json:"containers"`
	Metrics    []ContainerMetrics `json:"metrics"`
}
