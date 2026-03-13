// Package collector defines interfaces and implementations for
// collecting Docker container metrics.
//
// This package is the heart of the observability system. It defines
// the Collector interface that abstracts collection logic, allowing
// different implementations (CPU, memory, network, disk).
//
// ARCHITECTURE:
//   - Collector is an interface (contract)
//   - Each metric type will have its own implementation
//   - The orchestrator (main or a scheduler) calls Collect() periodically
//   - Follows the Open/Closed principle: open for extension, closed for modification
package collector

import (
	"context"
	"errors"

	"github.com/nicolas-moura-ti/castle-rock-agent/pkg/models"
)

// Collector defines the contract for metric collectors.
//
// INTERFACES IN GO:
//   - Interfaces are defined by the consumer, not the implementer
//   - A small interface is better than a large one (Interface Segregation)
//   - Go convention: "Accept interfaces, return structs"
//   - Don't create interfaces prematurely — extract them when there's
//     a real need for polymorphism
//
// Why define this interface now?
//   - Establishes the contract that future implementations must follow
//   - Allows using mocks in unit tests
//   - Documents the architectural intent of the system
type Collector interface {
	// Collect executes metric collection for all containers.
	//
	// Accepts a context to support cancellation and timeouts.
	// Returns a slice of ContainerMetrics or an error.
	Collect(ctx context.Context) ([]models.ContainerMetrics, error)

	// Name returns the collector name (e.g. "cpu", "memory", "network").
	// Used for logging and identification.
	Name() string
}

// ContainerCollector is a scaffold for the main container collector.
// In the future, this struct will be expanded with necessary dependencies
// (Docker client, configuration, etc.).
type ContainerCollector struct {
	// TODO: Add dependencies when implementing real collection.
	// Example:
	//   dockerClient *docker.Client
	//   interval     time.Duration
}

// NewContainerCollector creates a new ContainerCollector instance.
//
// Follows the Go constructor pattern New<Type>.
func NewContainerCollector() *ContainerCollector {
	return &ContainerCollector{}
}

// Collect implements the Collector interface.
//
// TODO: Implement real metric collection via Docker Stats API.
// The Docker Stats API provides real-time CPU, memory,
// network I/O and disk metrics for each container.
func (c *ContainerCollector) Collect(ctx context.Context) ([]models.ContainerMetrics, error) {
	// Placeholder — will be implemented in the next iteration.
	return nil, errors.New("not implemented")
}

// Name returns the name of this collector.
func (c *ContainerCollector) Name() string {
	return "container"
}
