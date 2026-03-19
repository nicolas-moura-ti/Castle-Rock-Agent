package collector_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nicolas-moura-ti/castle-rock-agent/internal/collector"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/config"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/docker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContainerCollector_Collect_Success(t *testing.T) {
	// Setup a mock Docker daemon API
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock ping for API negotiation
		if r.URL.Path == "/_ping" {
			w.Header().Set("API-Version", "1.43")
			w.Write([]byte("OK"))
			return
		}

		// Mock container list com ID de 64 caracteres (Padrão Docker)
		if strings.HasSuffix(r.URL.Path, "/containers/json") {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"Id":"1234567890123456789012345678901234567890123456789012345678901234","Names":["/test-container"],"Image":"test-image"}]`))
			return
		}

		// Mock container stats utilizando o ID truncado (12 caracteres)
		if strings.HasSuffix(r.URL.Path, "/containers/123456789012/stats") {
			w.Header().Set("Content-Type", "application/json")
			statsJSON := `{
				"read":"2023-01-01T00:00:00Z",
				"preread":"2023-01-01T00:00:00Z",
				"pids_stats":{},
				"blkio_stats":{
					"io_service_bytes_recursive":[
						{"op":"Read","value":100},
						{"op":"Write","value":200}
					]
				},
				"num_procs":0,
				"cpu_stats":{
					"cpu_usage":{"total_usage":100, "percpu_usage":[50,50]},
					"system_cpu_usage":200,
					"online_cpus":2
				},
				"precpu_stats":{
					"cpu_usage":{"total_usage":50, "percpu_usage":[25,25]},
					"system_cpu_usage":100,
					"online_cpus":2
				},
				"memory_stats":{
					"usage":500,
					"limit":1000
				},
				"networks":{
					"eth0":{
						"rx_bytes":10,
						"tx_bytes":20
					}
				}
			}`
			w.Write([]byte(statsJSON))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	t.Setenv("DOCKER_HOST", mockServer.URL)

	cli, err := docker.NewClient()
	require.NoError(t, err)
	defer cli.Close()

	cfg := config.DefaultConfig()
	c := collector.NewContainerCollector(cli, cfg, 1*time.Second)

	metrics, err := c.Collect(context.Background())
	require.NoError(t, err)
	require.NotNil(t, metrics)
	require.Len(t, metrics, 1)

	m := metrics[0]
	// Verificando o ID truncado de 12 caracteres (Lógica da Main)
	assert.Equal(t, "123456789012", m.ContainerID)
	assert.Equal(t, "test-container", m.ContainerName)
	assert.Equal(t, "test-image", m.Image)
	assert.Equal(t, float64(100), m.CPUPercent)
	assert.Equal(t, uint64(500), m.MemoryUsage)
	assert.Equal(t, uint64(1000), m.MemoryLimit)
	assert.Equal(t, float64(50), m.MemoryPercent)
	assert.Equal(t, uint64(10), m.NetworkRx)
	assert.Equal(t, uint64(20), m.NetworkTx)
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
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_ping" {
			w.Header().Set("API-Version", "1.43")
			w.Write([]byte("OK"))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/containers/json") {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"message": "internal server error"}`))
			return
		}
	}))
	defer mockServer.Close()

	t.Setenv("DOCKER_HOST", mockServer.URL)
	cli, err := docker.NewClient()
	require.NoError(t, err)
	defer cli.Close()

	cfg := config.DefaultConfig()
	c := collector.NewContainerCollector(cli, cfg, 1*time.Second)

	metrics, err := c.Collect(context.Background())
	require.Error(t, err)
	assert.Nil(t, metrics)
	
	// Ajuste do erro esperado conforme a implementação atual do coletor (Branch fix)
	assert.Contains(t, err.Error(), "collector: failed to list running containers")
}

func TestContainerCollector_Name(t *testing.T) {
	cfg := config.DefaultConfig()
	c := collector.NewContainerCollector(nil, cfg, 1*time.Second)
	assert.Equal(t, "container", c.Name())
}