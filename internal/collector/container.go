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
	"fmt"
	"time"

	"github.com/nicolas-moura-ti/castle-rock-agent/internal/config"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/docker"
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
	dockerClient *docker.Client
	cfg          config.Config
	interval     time.Duration
}

// NewContainerCollector creates a new ContainerCollector instance.
//
// Follows the Go constructor pattern New<Type>.
func NewContainerCollector(dockerClient *docker.Client, cfg config.Config, interval time.Duration) *ContainerCollector {
	return &ContainerCollector{
		dockerClient: dockerClient,
		cfg:          cfg,
		interval:     interval,
	}
}

// Collect implements the Collector interface.
//
// The Docker Stats API provides real-time CPU, memory,
// network I/O and disk metrics for each container.
func (c *ContainerCollector) Collect(ctx context.Context) ([]models.ContainerMetrics, error) {
	// Verificação de segurança vinda da branch feat
	if c.dockerClient == nil {
		return nil, fmt.Errorf("collector: docker client is not initialized")
	}

	containers, err := c.dockerClient.ListRunningContainers(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("collector: failed to list running containers: %w", err)
	}

	statsMap, err := c.dockerClient.GetAllContainerStats(ctx, containers)
	if err != nil {
		// Uso do %w para permitir o unwrap do erro original posteriormente, se necessário
		return nil, fmt.Errorf("collector: failed to get container stats: %w", err)
	}

	metrics := make([]models.ContainerMetrics, 0, len(statsMap))
	for _, m := range statsMap {
		metrics = append(metrics, m)
	}

	return metrics, nil
}

// Name returns the name of this collector.
func (c *ContainerCollector) Name() string {
	return "container"
}