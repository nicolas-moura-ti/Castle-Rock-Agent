package cluster

import (
	"bytes"
	"context"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nicolas-moura-ti/castle-rock-agent/pkg/models"
)

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
