package cluster

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nicolas-moura-ti/castle-rock-agent/pkg/models"
)

func TestReceiver_ServeHTTP(t *testing.T) {
	// Create a dummy logger to discard output during tests
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	token := "secret-key"
	// Use empty secret so it accepts unencrypted JSON requests for these tests
	receiver := NewReceiver(logger, "", token)

	tests := []struct {
		name           string
		method         string
		authHeader     string
		body           interface{}
		expectedStatus int
	}{
		{
			name:           "Method Not Allowed",
			method:         http.MethodGet,
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "Unauthorized - Missing Header",
			method:         http.MethodPost,
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Unauthorized - Invalid Token",
			method:         http.MethodPost,
			authHeader:     "Bearer wrong-token",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Bad Request - Invalid JSON",
			method:         http.MethodPost,
			authHeader:     "Bearer " + token,
			body:           "invalid-json",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Bad Request - Missing HostID",
			method:         http.MethodPost,
			authHeader:     "Bearer " + token,
			body:           models.PushPayload{},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Success",
			method:         http.MethodPost,
			authHeader:     "Bearer " + token,
			body:           models.PushPayload{HostID: "test-host"},
			expectedStatus: http.StatusAccepted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyBytes []byte
			if tt.body != nil {
				if s, ok := tt.body.(string); ok && s == "invalid-json" {
					bodyBytes = []byte(s)
				} else {
					bodyBytes, _ = json.Marshal(tt.body)
				}
			}

			req := httptest.NewRequest(tt.method, "/", bytes.NewReader(bodyBytes))
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rr := httptest.NewRecorder()
			receiver.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.expectedStatus)
			}
		})
	}
}

func TestReceiver_GetAllMetrics(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	receiver := NewReceiver(logger, "", "")

	// Insert active host
	receiver.hosts["active-host"] = HostData{
		LastUpdate: time.Now(),
		Payload: models.PushPayload{
			Metrics: []models.ContainerMetrics{{ContainerID: "1"}},
		},
	}

	// Insert inactive host
	receiver.hosts["inactive-host"] = HostData{
		LastUpdate: time.Now().Add(-40 * time.Second),
		Payload: models.PushPayload{
			Metrics: []models.ContainerMetrics{{ContainerID: "2"}},
		},
	}

	metrics := receiver.GetAllMetrics()
	if len(metrics) != 1 {
		t.Errorf("GetAllMetrics returned %d metrics, want 1", len(metrics))
	}
	if len(metrics) > 0 && metrics[0].ContainerID != "1" {
		t.Errorf("GetAllMetrics returned wrong metric: got %v", metrics[0].ContainerID)
	}
}

func TestReceiver_GetAllContainers(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	receiver := NewReceiver(logger, "", "")

	// Insert active host
	receiver.hosts["active-host"] = HostData{
		LastUpdate: time.Now(),
		Payload: models.PushPayload{
			Containers: []models.ContainerInfo{{ID: "1"}},
		},
	}

	// Insert inactive host
	receiver.hosts["inactive-host"] = HostData{
		LastUpdate: time.Now().Add(-40 * time.Second),
		Payload: models.PushPayload{
			Containers: []models.ContainerInfo{{ID: "2"}},
		},
	}

	containers := receiver.GetAllContainers()
	if len(containers) != 1 {
		t.Errorf("GetAllContainers returned %d containers, want 1", len(containers))
	}
	if len(containers) > 0 && containers[0].ID != "1" {
		t.Errorf("GetAllContainers returned wrong container: got %v", containers[0].ID)
	}
}
