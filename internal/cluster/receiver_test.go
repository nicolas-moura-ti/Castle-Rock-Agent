package cluster

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nicolas-moura-ti/castle-rock-agent/pkg/models"
)

func TestReceiver_ServeHTTP(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	token := "secret-key"
	store := NewMemoryStore(logger)
	receiver := NewReceiver(logger, store, "", token)

	tests := []struct {
		name           string
		method         string
		header         string
		payload        interface{}
		expectedStatus int
	}{
		{
			name:           "Valid request",
			method:         http.MethodPost,
			header:         "Bearer secret-key",
			payload:        models.PushPayload{HostID: "worker-1"},
			expectedStatus: http.StatusAccepted,
		},
		{
			name:           "Missing token",
			method:         http.MethodPost,
			header:         "",
			payload:        models.PushPayload{HostID: "worker-1"},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Wrong method",
			method:         http.MethodGet,
			header:         "Bearer secret-key",
			payload:        nil,
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body []byte
			if tt.payload != nil {
				body, _ = json.Marshal(tt.payload)
			}
			req := httptest.NewRequest(tt.method, "/api/v1/push", bytes.NewReader(body))
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}

			rr := httptest.NewRecorder()
			receiver.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}
		})
	}
}

func TestReceiver_GetAllMetrics(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	store := NewMemoryStore(logger)
	receiver := NewReceiver(logger, store, "", "")

	// Insert active host directly into store for testing
	store.UpdateHost(models.PushPayload{
		HostID: "active-host",
		Metrics: []models.ContainerMetrics{
			{ContainerName: "c1"},
		},
	})

	metrics := receiver.GetAllMetrics()
	if len(metrics) != 1 {
		t.Errorf("expected 1 metric, got %d", len(metrics))
	}
}

func TestReceiver_GetAllContainers(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	store := NewMemoryStore(logger)
	receiver := NewReceiver(logger, store, "", "")

	store.UpdateHost(models.PushPayload{
		HostID: "active-host",
		Containers: []models.ContainerInfo{
			{Name: "cont1"},
		},
	})

	containers := receiver.GetAllContainers()
	if len(containers) != 1 {
		t.Errorf("expected 1 container, got %d", len(containers))
	}
}
