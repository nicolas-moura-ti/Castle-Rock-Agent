package topology_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/docker/docker/api/types/network"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/docker"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/logger"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/topology"
	"github.com/nicolas-moura-ti/castle-rock-agent/pkg/models"
	"github.com/docker/docker/api/types"
)

// mockEngine implements docker.ContainerEngine for benchmarking
type mockEngine struct {
	networks []network.Inspect
}

func (m *mockEngine) Close() error { return nil }
func (m *mockEngine) ListRunningContainers(ctx context.Context, all bool) ([]models.ContainerInfo, error) { return nil, nil }
func (m *mockEngine) ListRunningContainersDetailed(ctx context.Context, all bool) ([]logger.ContainerDisplay, error) { return nil, nil }
func (m *mockEngine) GetAllContainerStats(ctx context.Context, containers []models.ContainerInfo) (map[string]models.ContainerMetrics, error) { return nil, nil }
func (m *mockEngine) StreamContainerLogs(ctx context.Context, containerID string) (<-chan string, error) { return nil, nil }
func (m *mockEngine) StopContainer(ctx context.Context, id string) error { return nil }
func (m *mockEngine) RestartContainer(ctx context.Context, id string) error { return nil }
func (m *mockEngine) InspectContainer(ctx context.Context, id string) (types.ContainerJSON, error) { return types.ContainerJSON{}, nil }
func (m *mockEngine) WatchEvents(ctx context.Context) (<-chan docker.ContainerEvent, <-chan error) { return nil, nil }
func (m *mockEngine) SetIncludeContainers(includes []string) {}
func (m *mockEngine) RunStressTest(ctx context.Context, mode string, duration int) error { return nil }
func (m *mockEngine) PruneUnused(ctx context.Context) (uint64, error) { return 0, nil }
func (m *mockEngine) PruneImages(ctx context.Context) (uint64, error) { return 0, nil }
func (m *mockEngine) PruneVolumes(ctx context.Context) (uint64, error) { return 0, nil }
func (m *mockEngine) GetDiskUsage(ctx context.Context) (docker.SystemDiskUsage, error) { return docker.SystemDiskUsage{}, nil }
func (m *mockEngine) ListNetworks(ctx context.Context) ([]network.Inspect, error) { return m.networks, nil }
func (m *mockEngine) GetSystemInfo(ctx context.Context) (map[string]string, error) { return nil, nil }

func BenchmarkBuildMap(b *testing.B) {
	// Create mock data: 10 networks, each with 100 containers
	networks := make([]network.Inspect, 0, 10)
	for i := 0; i < 10; i++ {
		containers := make(map[string]network.EndpointResource)
		for j := 0; j < 100; j++ {
			id := fmt.Sprintf("container-%d-%d", i, j)
			// Pad to at least 12 chars
			idStr := id
			for len(idStr) < 12 {
				idStr += "0"
			}
			containers[idStr] = network.EndpointResource{
				Name:        fmt.Sprintf("name-%d-%d", i, j),
				IPv4Address: fmt.Sprintf("10.0.%d.%d/24", i, j),
			}
		}

		netID := fmt.Sprintf("net-%d", i)
		for len(netID) < 12 {
			netID += "0"
		}

		networks = append(networks, network.Inspect{
			Name:       fmt.Sprintf("network-%d", i),
			ID:         netID,
			Driver:     "bridge",
			Containers: containers,
		})
	}

	engine := &mockEngine{networks: networks}
	mapper := topology.NewMapper(engine)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := mapper.BuildMap(ctx)
		if err != nil {
			b.Fatalf("BuildMap failed: %v", err)
		}
	}
}
