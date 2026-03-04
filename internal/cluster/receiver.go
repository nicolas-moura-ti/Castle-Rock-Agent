package cluster

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/nicolas-moura-ti/castle-rock-agent/pkg/models"
)

// HostData stores the latest metrics/containers snapshot received from a Worker.
type HostData struct {
	LastUpdate time.Time
	Payload    models.PushPayload
}

// Receiver is the HTTP server integrated in the Leader to receive data from Workers.
// All read/write access is thread-safe (protected by RWMutex) since it can be
// accessed simultaneously by the TUI, Prometheus exporter and HTTP receive routine.
type Receiver struct {
	mu    sync.RWMutex
	hosts map[string]HostData
	log   *slog.Logger
}

// NewReceiver creates a new Receiver instance.
func NewReceiver(log *slog.Logger) *Receiver {
	return &Receiver{
		hosts: make(map[string]HostData),
		log:   log,
	}
}

// ServeHTTP implements the http.Handler interface to receive POSTs with the Payload.
func (r *Receiver) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer req.Body.Close()

	var payload models.PushPayload
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		r.log.Warn("receiver: failed to decode JSON", slog.String("error", err.Error()))
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	if payload.HostID == "" {
		r.log.Warn("receiver: payload rejected, missing HostID")
		http.Error(w, "Missing HostID", http.StatusBadRequest)
		return
	}

	r.mu.Lock()
	r.hosts[payload.HostID] = HostData{
		LastUpdate: time.Now(),
		Payload:    payload,
	}
	r.mu.Unlock()

	w.WriteHeader(http.StatusAccepted)
}

// GetAllMetrics returns a list containing all recent metrics from all Workers.
func (r *Receiver) GetAllMetrics() []models.ContainerMetrics {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var all []models.ContainerMetrics
	now := time.Now()

	for hostID, data := range r.hosts {
		// Fault tolerance: ignore Workers that haven't communicated in over 30s.
		if now.Sub(data.LastUpdate) > 30*time.Second {
			r.log.Debug("receiver: inactive host ignored", slog.String("host", hostID))
			continue
		}
		all = append(all, data.Payload.Metrics...)
	}

	return all
}

// GetAllContainers returns static container info from all recent Workers.
func (r *Receiver) GetAllContainers() []models.ContainerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var all []models.ContainerInfo
	now := time.Now()

	for _, data := range r.hosts {
		if now.Sub(data.LastUpdate) > 30*time.Second {
			continue
		}
		all = append(all, data.Payload.Containers...)
	}

	return all
}
