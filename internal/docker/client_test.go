package docker_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nicolas-moura-ti/castle-rock-agent/internal/docker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAllContainerStats(t *testing.T) {
	// Setup a mock Docker daemon API
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock ping for API negotiation
		if r.URL.Path == "/_ping" {
			w.Header().Set("API-Version", "1.43")
			w.Write([]byte("OK"))
			return
		}

		// Mock container list
		if r.URL.Path == "/v1.43/containers/json" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"Id":"test-container-id","Names":["/test-container"],"Image":"test-image"}]`))
			return
		}

		// Mock container stats
		if r.URL.Path == "/v1.43/containers/test-container-id/stats" {
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
				"storage_stats":{},
				"cpu_stats":{
					"cpu_usage":{
						"total_usage":100,
						"percpu_usage":[50,50],
						"usage_in_kernelmode":0,
						"usage_in_usermode":0
					},
					"system_cpu_usage":200,
					"online_cpus":2,
					"throttling_data":{
						"periods":0,
						"throttled_periods":0,
						"throttled_time":0
					}
				},
				"precpu_stats":{
					"cpu_usage":{
						"total_usage":50,
						"percpu_usage":[25,25],
						"usage_in_kernelmode":0,
						"usage_in_usermode":0
					},
					"system_cpu_usage":100,
					"online_cpus":2,
					"throttling_data":{
						"periods":0,
						"throttled_periods":0,
						"throttled_time":0
					}
				},
				"memory_stats":{
					"usage":500,
					"max_usage":0,
					"stats":{},
					"limit":1000
				},
				"networks":{
					"eth0":{
						"rx_bytes":10,
						"rx_packets":0,
						"rx_errors":0,
						"rx_dropped":0,
						"tx_bytes":20,
						"tx_packets":0,
						"tx_errors":0,
						"tx_dropped":0
					}
				}
			}`
			w.Write([]byte(statsJSON))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	// Redirect Docker client to the mock server
	t.Setenv("DOCKER_HOST", mockServer.URL)

	// Create new client
	cli, err := docker.NewClient()
	require.NoError(t, err)
	defer cli.Close()

	// Run GetAllContainerStats
	stats, err := cli.GetAllContainerStats(context.Background(), false)
	require.NoError(t, err)
	require.NotNil(t, stats)

	// Assertions
	require.Contains(t, stats, "test-contain")
	metrics := stats["test-contain"]

	assert.Equal(t, "test-contain", metrics.ContainerID)
	assert.Equal(t, "test-container", metrics.ContainerName)
	assert.Equal(t, "test-image", metrics.Image)
	assert.Equal(t, float64(100), metrics.CPUPercent)
	assert.Equal(t, uint64(500), metrics.MemoryUsage)
	assert.Equal(t, uint64(1000), metrics.MemoryLimit)
	assert.Equal(t, float64(50), metrics.MemoryPercent)
	assert.Equal(t, uint64(10), metrics.NetworkRx)
	assert.Equal(t, uint64(20), metrics.NetworkTx)
	assert.Equal(t, uint64(100), metrics.BlockRead)
	assert.Equal(t, uint64(200), metrics.BlockWrite)
}

func TestGetAllContainerStats_Error(t *testing.T) {
	// Setup a mock Docker daemon API that returns an error
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock ping for API negotiation
		if r.URL.Path == "/_ping" {
			w.Header().Set("API-Version", "1.43")
			w.Write([]byte("OK"))
			return
		}

		// Mock container list returning 500 Internal Server Error
		if r.URL.Path == "/v1.43/containers/json" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"message": "internal server error"}`))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	// Redirect Docker client to the mock server
	t.Setenv("DOCKER_HOST", mockServer.URL)

	// Create new client
	cli, err := docker.NewClient()
	require.NoError(t, err)
	defer cli.Close()

	// Run GetAllContainerStats, expect an error
	stats, err := cli.GetAllContainerStats(context.Background(), false)
	require.Error(t, err)
	assert.Nil(t, stats)
	assert.Contains(t, err.Error(), "docker.GetAllContainerStats: failed to list:")
}
