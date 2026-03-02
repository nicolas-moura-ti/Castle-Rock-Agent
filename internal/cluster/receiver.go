package cluster

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/nicolas-moura-ti/castle-rock-agent/pkg/models"
)

// HostData armazena o último snapshot de métricas/containers recebido de um Worker.
type HostData struct {
	LastUpdate time.Time
	Payload    models.PushPayload
}

// Receiver é o servidor HTTP integrado no Leader para receber dados dos Workers.
// Todo o acesso de leitura/escrita é thread-safe (protegido por RWMutex) pois pode
// ser acessado simultaneamente pela TUI, exportador Prometheus e rotina de recebimento HTTP.
type Receiver struct {
	mu    sync.RWMutex
	hosts map[string]HostData
	log   *slog.Logger
}

func NewReceiver(log *slog.Logger) *Receiver {
	return &Receiver{
		hosts: make(map[string]HostData),
		log:   log,
	}
}

// ServeHTTP implementa a interface http.Handler para receber POSTs com o Payload.
func (r *Receiver) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload models.PushPayload
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		r.log.Warn("Receiver: falha ao decodificar JSON", slog.String("error", err.Error()))
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	if payload.HostID == "" {
		r.log.Warn("Receiver: bloco rejeitado, payload sem HostID")
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

// GetAllMetrics retorna uma lista contendo todas as métricas recentes de todos os Workers.
func (r *Receiver) GetAllMetrics() []models.ContainerMetrics {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var all []models.ContainerMetrics
	now := time.Now()

	for hostID, data := range r.hosts {
		// Tolerância a falhas: Ignoramos Workers que não se comunicam há mais de 30s.
		if now.Sub(data.LastUpdate) > 30*time.Second {
			r.log.Debug("Receiver: host inativo ignorado", slog.String("host", hostID))
			continue
		}
		all = append(all, data.Payload.Metrics...)
	}

	return all
}

// GetAllContainers retorna informações estáticas (Containers) recentes de todos os Workers.
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
