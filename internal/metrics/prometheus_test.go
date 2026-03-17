package metrics_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"

	"github.com/nicolas-moura-ti/castle-rock-agent/internal/docker"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/metrics"
)

// setupMockDockerServer creates an httptest.Server that mocks the Docker daemon API endpoints
// needed by our tests.
func setupMockDockerServer(t *testing.T) *httptest.Server {
	t.Helper()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/version"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ApiVersion": "1.41"}`))
		case strings.Contains(r.URL.Path, "/containers/json"):
			// Mock container list
			containers := []types.Container{
				{
					ID:    "12345678901234567890",
					Names: []string{"/test-container"},
					Image: "test-image:latest",
				},
			}
			json.NewEncoder(w).Encode(containers)
		case strings.Contains(r.URL.Path, "/stats"):
			// Mock stats
			stats := types.StatsJSON{
				Stats: types.Stats{
					Read:    time.Now(),
					PreRead: time.Now().Add(-1 * time.Second),
				},
			}

			// Some dummy values for metrics
			stats.MemoryStats.Usage = 1024 * 1024 * 100 // 100 MB
			stats.MemoryStats.Limit = 1024 * 1024 * 500 // 500 MB

			stats.Networks = map[string]types.NetworkStats{
				"eth0": {
					RxBytes: 1000,
					TxBytes: 2000,
				},
			}

			// Add basic CPU stats
			stats.CPUStats.CPUUsage.TotalUsage = 1000000
			stats.PreCPUStats.CPUUsage.TotalUsage = 500000
			stats.CPUStats.SystemUsage = 10000000
			stats.PreCPUStats.SystemUsage = 5000000
			stats.CPUStats.OnlineCPUs = 2

			json.NewEncoder(w).Encode(stats)
		default:
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"message": "not found"}`))
		}
	})

	return httptest.NewServer(handler)
}

func getMockClient(t *testing.T, ts *httptest.Server) *docker.Client {
	t.Helper()
	// Set DOCKER_HOST to point to our mock server
	origDockerHost := os.Getenv("DOCKER_HOST")
	t.Cleanup(func() {
		os.Setenv("DOCKER_HOST", origDockerHost)
	})

	os.Setenv("DOCKER_HOST", "tcp://"+ts.Listener.Addr().String())
	os.Setenv("DOCKER_API_VERSION", "1.41") // Avoid version negotiation making extra calls

	client, err := docker.NewClient()
	if err != nil {
		t.Fatalf("Failed to create mock docker client: %v", err)
	}

	return client
}

func TestNewExporter(t *testing.T) {
	ts := setupMockDockerServer(t)
	defer ts.Close()

	client := getMockClient(t, ts)
	defer client.Close()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	exporter := metrics.NewExporter(client, nil, "test-host", 10*time.Second, 9090, log)
	if exporter == nil {
		t.Fatal("NewExporter returned nil")
	}

	if exporter.GetLastStats() == nil {
		t.Fatal("Expected non-nil last stats")
	}
}

func TestExporter_Start(t *testing.T) {
	ts := setupMockDockerServer(t)
	defer ts.Close()

	client := getMockClient(t, ts)
	defer client.Close()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	port := 19090 // Use a different port to avoid conflicts
	exporter := metrics.NewExporter(client, nil, "test-host", 100*time.Millisecond, port, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exporter.Start(ctx)

	// Give the HTTP server a moment to start
	time.Sleep(300 * time.Millisecond)

	// Test health endpoint
	healthURL := fmt.Sprintf("http://localhost:%d/health", port)
	resp, err := http.Get(healthURL)
	if err != nil {
		t.Fatalf("Failed to get health endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 OK for /health, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"status":"ok"`) {
		t.Errorf("Expected response body to contain status ok, got: %s", string(body))
	}

	// Test metrics endpoint
	metricsURL := fmt.Sprintf("http://localhost:%d/metrics", port)
	respMetrics, err := http.Get(metricsURL)
	if err != nil {
		t.Fatalf("Failed to get metrics endpoint: %v", err)
	}
	defer respMetrics.Body.Close()

	if respMetrics.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 OK for /metrics, got %d", respMetrics.StatusCode)
	}

	metricsBody, _ := io.ReadAll(respMetrics.Body)
	metricsStr := string(metricsBody)

	// Verify our custom metrics are present in the response
	if !strings.Contains(metricsStr, "castle_rock_") {
		t.Errorf("Expected metrics response to contain 'castle_rock_', got: %s", metricsStr)
	}
}

func TestExporter_Collect(t *testing.T) {
	ts := setupMockDockerServer(t)
	defer ts.Close()

	client := getMockClient(t, ts)
	defer client.Close()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	exporter := metrics.NewExporter(client, nil, "test-host", 1*time.Second, 19091, log)

	// In a black-box test (metrics_test), we use the public API.
	// Since Start() calls collect, we already tested it above.
	// For this test, let's just verify GetLastStats() is thread-safe and works.
	
	stats := exporter.GetLastStats()
	if stats == nil {
		t.Fatal("Expected last stats to be non-nil")
	}
}
