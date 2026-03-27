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

// mockContainerEngine implementa a interface docker.ContainerEngine para testes unitários.
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

// Implementações mínimas necessárias para satisfazer a interface durante o teste
func (m *mockContainerEngine) Close() error { return nil }
func (m *mockContainerEngine) ListRunningContainers(ctx context.Context, all bool) ([]models.ContainerInfo, error) {
	if m.ListRunningContainersFunc != nil {
		return m.ListRunningContainersFunc(ctx, all)
	}
	return nil, nil
}
func (m *mockContainerEngine) GetAllContainerStats(ctx context.Context, containers []models.ContainerInfo) (map[string]models.ContainerMetrics, error) {
	if m.GetAllContainerStatsFunc != nil {
		return m.GetAllContainerStatsFunc(ctx, containers)
	}
	return nil, nil
}

func TestContainerCollector_Collect_Success(t *testing.T) {
	// Usando IDs reais para validar o truncamento da lógica da Main
	longID := "1234567890123456789012345678901234567890123456789012345678901234"
	shortID := "123456789012"

	mockEngine := &mockContainerEngine{
		ListRunningContainersFunc: func(ctx context.Context, all bool) ([]models.ContainerInfo, error) {
			return []models.ContainerInfo{
				{ID: longID, Name: "test-container", Image: "test-image"},
			}, nil
		},
		GetAllContainerStatsFunc: func(ctx context.Context, containers []models.ContainerInfo) (map[string]models.ContainerMetrics, error) {
			return map[string]models.ContainerMetrics{
				shortID: {
					ContainerID:   shortID,
					ContainerName: "test-container",
					Image:         "test-image",
					CPUPercent:    100.0,
					MemoryUsage:   500,
				},
			}, nil
		},
	}

	cfg := config.DefaultConfig()
	c := collector.NewContainerCollector(mockEngine, cfg, 1*time.Second)

	metrics, err := c.Collect(context.Background())
	require.NoError(t, err)
	require.Len(t, metrics, 1)

	m := metrics[0]
	// Validando o truncamento de 12 caracteres consolidado na Main
	assert.Equal(t, shortID, m.ContainerID)
	assert.Equal(t, "test-container", m.ContainerName)
	assert.Equal(t, "test-image", m.Image)
	assert.Equal(t, float64(100), m.CPUPercent)
}

func TestContainerCollector_Collect_NilClient(t *testing.T) {
	cfg := config.DefaultConfig()
	c := collector.NewContainerCollector(nil, cfg, 1*time.Second)

	_, err := c.Collect(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "collector: docker client is not initialized")
}

func TestContainerCollector_Collect_Error(t *testing.T) {
	mockEngine := &mockContainerEngine{
		ListRunningContainersFunc: func(ctx context.Context, all bool) ([]models.ContainerInfo, error) {
			// Simula falha logo no primeiro passo (listagem)
			return nil, assert.AnError 
		},
	}

	cfg := config.DefaultConfig()
	c := collector.NewContainerCollector(mockEngine, cfg, 1*time.Second)

	metrics, err := c.Collect(context.Background())
	require.Error(t, err)
	assert.Nil(t, metrics)
	// Resolvido: Erro esperado quando a listagem falha, conforme a Main
	assert.Contains(t, err.Error(), "collector: failed to list running containers")
}

func TestContainerCollector_Name(t *testing.T) {
	c := collector.NewContainerCollector(nil, config.DefaultConfig(), 1*time.Second)
	assert.Equal(t, "container", c.Name())
}
// Add missing methods to mockContainerEngine to satisfy docker.ContainerEngine interface
func (m *mockContainerEngine) GetDiskUsage(ctx context.Context) (docker.SystemDiskUsage, error) {
	return docker.SystemDiskUsage{}, nil
}
func (m *mockContainerEngine) ListNetworks(ctx context.Context) ([]network.Inspect, error) {
	return nil, nil
}
func (m *mockContainerEngine) GetSystemInfo(ctx context.Context) (map[string]string, error) {
	return nil, nil
}

func (m *mockContainerEngine) InspectContainer(ctx context.Context, id string) (types.ContainerJSON, error) {
	return types.ContainerJSON{}, nil
}

func (m *mockContainerEngine) ListRunningContainersDetailed(ctx context.Context, all bool) ([]logger.ContainerDisplay, error) {
	return nil, nil
}

func (m *mockContainerEngine) PruneImages(ctx context.Context) (uint64, error) {
	return 0, nil
}
func (m *mockContainerEngine) PruneUnused(ctx context.Context) (uint64, error) {
	return 0, nil
}
func (m *mockContainerEngine) PruneVolumes(ctx context.Context) (uint64, error) {
	return 0, nil
}

func (m *mockContainerEngine) RestartContainer(ctx context.Context, id string) error {
	return nil
}
func (m *mockContainerEngine) RunStressTest(ctx context.Context, mode string, duration int) error {
	return nil
}
func (m *mockContainerEngine) StopContainer(ctx context.Context, id string) error {
	return nil
}
func (m *mockContainerEngine) StreamContainerLogs(ctx context.Context, containerID string) (<-chan string, error) {
	return nil, nil
}

func (m *mockContainerEngine) SetIncludeContainers(includes []string) {}

func (m *mockContainerEngine) WatchEvents(ctx context.Context) (<-chan docker.ContainerEvent, <-chan error) {
	return nil, nil
}
