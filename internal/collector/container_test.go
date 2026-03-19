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

// mockContainerEngine implementa a interface docker.ContainerEngine para testes puros em Go.
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

// Implementação dos métodos da interface (necessário para o Mock funcionar)
func (m *mockContainerEngine) Close() error { return nil }
func (m *mockContainerEngine) ListRunningContainers(ctx context.Context, all bool) ([]models.ContainerInfo, error) {
	return m.ListRunningContainersFunc(ctx, all)
}
func (m *mockContainerEngine) GetAllContainerStats(ctx context.Context, containers []models.ContainerInfo) (map[string]models.ContainerMetrics, error) {
	return m.GetAllContainerStatsFunc(ctx, containers)
}
// ... (outros métodos omitidos para brevidade, mas devem existir no seu arquivo)

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
	assert.Equal(t, float64(100), m.CPUPercent)
}

func TestContainerCollector_Collect_NilClient(t *testing.T) {
	cfg := config.DefaultConfig()
	c := collector.NewContainerCollector(nil, cfg, 1*time.Second)

	metrics, err := c.Collect(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "collector: docker client is not initialized")
}

func TestContainerCollector_Collect_Error(t *testing.T) {
	mockEngine := &mockContainerEngine{
		ListRunningContainersFunc: func(ctx context.Context, all bool) ([]models.ContainerInfo, error) {
			return nil, docker.ErrDockerConnection // Simula erro de conexão
		},
	}

	cfg := config.DefaultConfig()
	c := collector.NewContainerCollector(mockEngine, cfg, 1*time.Second)

	metrics, err := c.Collect(context.Background())
	require.Error(t, err)
	assert.Nil(t, metrics)
	// Ajuste do erro esperado vindo da implementação do coletor
	assert.Contains(t, err.Error(), "collector: failed to list running containers")
}

func TestContainerCollector_Name(t *testing.T) {
	c := collector.NewContainerCollector(nil, config.DefaultConfig(), 1*time.Second)
	assert.Equal(t, "container", c.Name())
}