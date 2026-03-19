package collector_test

import (
	"context"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/network"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/collector"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/config"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/docker"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/logger"
	"github.com/nicolas-moura-ti/castle-rock-agent/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockContainerEngine is a mock implementation of docker.ContainerEngine for testing.
type mockContainerEngine struct {
	ListRunningContainersFunc         func(ctx context.Context, all bool) ([]models.ContainerInfo, error)
	ListRunningContainersDetailedFunc func(ctx context.Context, all bool) ([]logger.ContainerDisplay, error)
	GetAllContainerStatsFunc          func(ctx context.Context, containers []models.ContainerInfo) (map[string]models.ContainerMetrics, error)
	StreamContainerLogsFunc           func(ctx context.Context, containerID string) (<-chan string, error)
	StopContainerFunc                 func(ctx context.Context, id string) error
	RestartContainerFunc              func(ctx context.Context, id string) error
	InspectContainerFunc              func(ctx context.Context, id string) (types.ContainerJSON, error)
	WatchEventsFunc                   func(ctx context.Context) (<-chan docker.ContainerEvent, <-chan error)
	SetIncludeContainersFunc          func(includes []string)
	RunStressTestFunc                 func(ctx context.Context, mode string, duration int) error
	PruneUnusedFunc                   func(ctx context.Context) (uint64, error)
	PruneImagesFunc                   func(ctx context.Context) (uint64, error)
	PruneVolumesFunc                  func(ctx context.Context) (uint64, error)
	GetDiskUsageFunc                  func(ctx context.Context) (docker.SystemDiskUsage, error)
	ListNetworksFunc                  func(ctx context.Context) ([]network.Inspect, error)
	GetSystemInfoFunc                 func(ctx context.Context) (map[string]string, error)
	CloseFunc                         func() error
}

func (m *mockContainerEngine) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}
func (m *mockContainerEngine) ListRunningContainers(ctx context.Context, all bool) ([]models.ContainerInfo, error) {
	if m.ListRunningContainersFunc != nil {
		return m.ListRunningContainersFunc(ctx, all)
	}
	return nil, nil
}
func (m *mockContainerEngine) ListRunningContainersDetailed(ctx context.Context, all bool) ([]logger.ContainerDisplay, error) {
	if m.ListRunningContainersDetailedFunc != nil {
		return m.ListRunningContainersDetailedFunc(ctx, all)
	}
	return nil, nil
}
func (m *mockContainerEngine) GetAllContainerStats(ctx context.Context, containers []models.ContainerInfo) (map[string]models.ContainerMetrics, error) {
	if m.GetAllContainerStatsFunc != nil {
		return m.GetAllContainerStatsFunc(ctx, containers)
	}
	return nil, nil
}
func (m *mockContainerEngine) StreamContainerLogs(ctx context.Context, containerID string) (<-chan string, error) {
	if m.StreamContainerLogsFunc != nil {
		return m.StreamContainerLogsFunc(ctx, containerID)
	}
	return nil, nil
}
func (m *mockContainerEngine) StopContainer(ctx context.Context, id string) error {
	if m.StopContainerFunc != nil {
		return m.StopContainerFunc(ctx, id)
	}
	return nil
}
func (m *mockContainerEngine) RestartContainer(ctx context.Context, id string) error {
	if m.RestartContainerFunc != nil {
		return m.RestartContainerFunc(ctx, id)
	}
	return nil
}
func (m *mockContainerEngine) InspectContainer(ctx context.Context, id string) (types.ContainerJSON, error) {
	if m.InspectContainerFunc != nil {
		return m.InspectContainerFunc(ctx, id)
	}
	return types.ContainerJSON{}, nil
}
func (m *mockContainerEngine) WatchEvents(ctx context.Context) (<-chan docker.ContainerEvent, <-chan error) {
	if m.WatchEventsFunc != nil {
		return m.WatchEventsFunc(ctx)
	}
	return nil, nil
}
func (m *mockContainerEngine) SetIncludeContainers(includes []string) {
	if m.SetIncludeContainersFunc != nil {
		m.SetIncludeContainersFunc(includes)
	}
}
func (m *mockContainerEngine) RunStressTest(ctx context.Context, mode string, duration int) error {
	if m.RunStressTestFunc != nil {
		return m.RunStressTestFunc(ctx, mode, duration)
	}
	return nil
}
func (m *mockContainerEngine) PruneUnused(ctx context.Context) (uint64, error) {
	if m.PruneUnusedFunc != nil {
		return m.PruneUnusedFunc(ctx)
	}
	return 0, nil
}
func (m *mockContainerEngine) PruneImages(ctx context.Context) (uint64, error) {
	if m.PruneImagesFunc != nil {
		return m.PruneImagesFunc(ctx)
	}
	return 0, nil
}
func (m *mockContainerEngine) PruneVolumes(ctx context.Context) (uint64, error) {
	if m.PruneVolumesFunc != nil {
		return m.PruneVolumesFunc(ctx)
	}
	return 0, nil
}
func (m *mockContainerEngine) GetDiskUsage(ctx context.Context) (docker.SystemDiskUsage, error) {
	if m.GetDiskUsageFunc != nil {
		return m.GetDiskUsageFunc(ctx)
	}
	return docker.SystemDiskUsage{}, nil
}
func (m *mockContainerEngine) ListNetworks(ctx context.Context) ([]network.Inspect, error) {
	if m.ListNetworksFunc != nil {
		return m.ListNetworksFunc(ctx)
	}
	return nil, nil
}
func (m *mockContainerEngine) GetSystemInfo(ctx context.Context) (map[string]string, error) {
	if m.GetSystemInfoFunc != nil {
		return m.GetSystemInfoFunc(ctx)
	}
	return nil, nil
}

func TestContainerCollector_Collect_Success(t *testing.T) {
	mockEngine := &mockContainerEngine{
		ListRunningContainersFunc: func(ctx context.Context, all bool) ([]models.ContainerInfo, error) {
			return []models.ContainerInfo{
				{
					ID:    "test-container-id",
					Name:  "test-container",
					Image: "test-image",
				},
			}, nil
		},
		GetAllContainerStatsFunc: func(ctx context.Context, containers []models.ContainerInfo) (map[string]models.ContainerMetrics, error) {
			return map[string]models.ContainerMetrics{
				"test-container-id": {
					ContainerID:   "test-container-id",
					ContainerName: "test-container",
					Image:         "test-image",
					CPUPercent:    100.0,
					MemoryUsage:   500,
					MemoryLimit:   1000,
					MemoryPercent: 50.0,
					NetworkRx:     10,
					NetworkTx:     20,
					BlockRead:     100,
					BlockWrite:    200,
				},
			}, nil
		},
	}

	cfg := config.DefaultConfig()
	c := collector.NewContainerCollector(mockEngine, cfg, 1*time.Second)

	metrics, err := c.Collect(context.Background())
	require.NoError(t, err)
	require.NotNil(t, metrics)
	require.Len(t, metrics, 1)

	m := metrics[0]
	assert.Equal(t, "test-container-id", m.ContainerID)
	assert.Equal(t, "test-container", m.ContainerName)
	assert.Equal(t, "test-image", m.Image)
	assert.Equal(t, float64(100), m.CPUPercent)
	assert.Equal(t, uint64(500), m.MemoryUsage)
	assert.Equal(t, uint64(1000), m.MemoryLimit)
	assert.Equal(t, float64(50), m.MemoryPercent)
	assert.Equal(t, uint64(10), m.NetworkRx)
	assert.Equal(t, uint64(20), m.NetworkTx)
	assert.Equal(t, uint64(100), m.BlockRead)
	assert.Equal(t, uint64(200), m.BlockWrite)
}

func TestContainerCollector_Collect_NilClient(t *testing.T) {
	cfg := config.DefaultConfig()
	c := collector.NewContainerCollector(nil, cfg, 1*time.Second)

	metrics, err := c.Collect(context.Background())
	require.Error(t, err)
	assert.Nil(t, metrics)
	assert.Contains(t, err.Error(), "collector: docker client is not initialized")
}

func TestContainerCollector_Collect_Error(t *testing.T) {
	mockEngine := &mockContainerEngine{
		ListRunningContainersFunc: func(ctx context.Context, all bool) ([]models.ContainerInfo, error) {
			return []models.ContainerInfo{
				{
					ID:    "test-container-id",
					Name:  "test-container",
					Image: "test-image",
				},
			}, nil
		},
		GetAllContainerStatsFunc: func(ctx context.Context, containers []models.ContainerInfo) (map[string]models.ContainerMetrics, error) {
			return nil, assert.AnError
		},
	}

	cfg := config.DefaultConfig()
	c := collector.NewContainerCollector(mockEngine, cfg, 1*time.Second)

	metrics, err := c.Collect(context.Background())
	require.Error(t, err)
	assert.Nil(t, metrics)
	// Ajuste do erro esperado conforme a implementação atual do coletor
	assert.Contains(t, err.Error(), "collector: failed to get container stats")
}

func TestContainerCollector_Name(t *testing.T) {
	cfg := config.DefaultConfig()
	c := collector.NewContainerCollector(nil, cfg, 1*time.Second)
	assert.Equal(t, "container", c.Name())
}