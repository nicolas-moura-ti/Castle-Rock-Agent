package cluster

import (
	"bytes"
	"context"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nicolas-moura-ti/castle-rock-agent/internal/config"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/docker"
	"github.com/nicolas-moura-ti/castle-rock-agent/pkg/models"
)

func TestStartSender_Loop(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	// Mock Docker Daemon Server
	dockerMux := http.NewServeMux()

	// Catch-all handler for Docker API to match any API version prefix
	dockerMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/version" || r.URL.Path == "/v1.41/version" {
			w.Write([]byte(`{"Version": "20.10.0", "ApiVersion": "1.41"}`))
			return
		}
		if r.URL.Path == "/containers/json" || strings.HasSuffix(r.URL.Path, "/containers/json") {
			w.Write([]byte(`[{"Id": "container123", "Names": ["/test-container"], "Image": "alpine", "State": "running", "Status": "Up 2 hours", "NetworkSettings": {"Networks": {}}, "Ports": []}]`))
			return
		}
		if r.URL.Path == "/containers/container123/stats" || strings.HasSuffix(r.URL.Path, "/containers/container123/stats") {
			w.Write([]byte(`{"read": "2023-01-01T00:00:00Z", "precpu_stats": {}, "cpu_stats": {}, "memory_stats": {"usage": 1024, "limit": 2048}}`))
			return
		}

		// Return empty JSON array for other container listing to avoid unmarshal errors
		if strings.HasSuffix(r.URL.Path, "json") {
			w.Write([]byte(`[]`))
			return
		}

		// Fallback for debugging unhandled requests
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	})

	dockerServer := httptest.NewServer(dockerMux)
	defer dockerServer.Close()

	t.Setenv("DOCKER_HOST", dockerServer.URL)

	// Create a real Docker client but pointed to our mock server
	dockerClient, err := docker.NewClient()
	if err != nil {
		t.Fatalf("Failed to create docker client: %v", err)
	}
	defer dockerClient.Close()

	// Mock Leader Server
	done := make(chan struct{}, 1)

	leaderServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusOK)
			select {
			case done <- struct{}{}:
			default:
			}
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer leaderServer.Close()

	cfg := config.Config{}
	cfg.Cluster.Mode = "worker"
	cfg.Cluster.HostID = "test-worker-1"
	cfg.Cluster.LeaderURL = leaderServer.URL
	cfg.Cluster.SharedSecret = "test-token"
	cfg.Stats.Interval = 10 * time.Millisecond // very short interval for testing

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run sender in a goroutine
	go StartSender(ctx, dockerClient, cfg, logger)

	// Wait for the mock leader server to receive a payload, or timeout
	select {
	case <-done:
		// Success, received a payload
	case <-time.After(1 * time.Second):
		t.Logf("timed out waiting for leader server to receive payload. Output: %s", logBuf.String())
		t.FailNow()
	}

	// Cancel context to stop the sender loop
	cancel()

	// Give the sender a moment to log the stop message
	time.Sleep(20 * time.Millisecond)

	logOutput := logBuf.String()
	if !bytes.Contains([]byte(logOutput), []byte("Worker Sender stopped")) {
		t.Errorf("expected 'Worker Sender stopped' log, got: %s", logOutput)
	}
}

func TestStartSender_EarlyReturn_NotWorker(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	cfg := config.Config{}
	cfg.Cluster.Mode = "standalone"

	// Call StartSender with a mode that is not "worker"
	// It should return immediately and not panic or block.
	StartSender(context.Background(), nil, cfg, logger)

	if logBuf.Len() > 0 {
		t.Errorf("expected no logs, got: %s", logBuf.String())
	}
}

func TestSendPushPayload_Success(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	serverCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected Bearer test-token, got: %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json, got: %s", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	payload := models.PushPayload{HostID: "test-host"}
	sendPushPayload(context.Background(), server.Client(), server.URL, "test-token", payload, logger)

	if !serverCalled {
		t.Error("expected server to be called")
	}

	logOutput := logBuf.String()
	if bytes.Contains([]byte(logOutput), []byte("error")) || bytes.Contains([]byte(logOutput), []byte("Warn")) {
		t.Errorf("expected no errors/warnings, got: %s", logOutput)
	}
}

func TestSendPushPayload_Rejected(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	payload := models.PushPayload{HostID: "test-host"}
	sendPushPayload(context.Background(), server.Client(), server.URL, "test-token", payload, logger)

	logOutput := logBuf.String()
	if !bytes.Contains([]byte(logOutput), []byte("Sender: push rejected by leader")) {
		t.Errorf("expected log about rejected push, got: %s", logOutput)
	}
}

func TestSendPushPayload_CommunicationFailure(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	// Invalid URL to force an error (using an invalid IP prevents DNS resolution delays)
	payload := models.PushPayload{HostID: "test-host"}
	sendPushPayload(context.Background(), http.DefaultClient, "http://127.0.0.1:0", "token", payload, logger)

	logOutput := logBuf.String()
	if !bytes.Contains([]byte(logOutput), []byte("Sender: communication failure (Leader inactive?)")) {
		t.Errorf("expected log about communication failure, got: %s", logOutput)
	}
}

func TestSendPushPayload_JSONMarshalError(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := server.Client()

	payload := models.PushPayload{
		HostID: "test-host",
		Metrics: []models.ContainerMetrics{
			{
				CPUPercent: math.NaN(),
			},
		},
	}

	sendPushPayload(context.Background(), client, server.URL, "token", payload, logger)

	logOutput := logBuf.String()
	if !bytes.Contains([]byte(logOutput), []byte("Sender: error marshaling payload JSON")) {
		t.Errorf("expected log about marshaling error, got: %s", logOutput)
	}
}
