package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/nicolas-moura-ti/castle-rock-agent/internal/config"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/docker"
	"github.com/nicolas-moura-ti/castle-rock-agent/pkg/models"
)

// StartSender inicia a rotina que coleta os dados do Docker local
// e envia (HTTP POST) para o Leader em intervalos definidos.
func StartSender(ctx context.Context, dockerClient *docker.Client, cfg config.Config, log *slog.Logger) {
	// Garante que só roda no modo worker
	if cfg.Cluster.Mode != "worker" {
		return
	}

	log.Info("Iniciando Worker Sender",
		slog.String("leader_url", cfg.Cluster.LeaderURL),
		slog.String("host_id", cfg.Cluster.HostID),
		slog.Duration("interval", cfg.Stats.Interval),
	)

	// Cria HTTP client com timeout garantido
	httpClient := &http.Client{
		Timeout: 5 * time.Second,
	}

	ticker := time.NewTicker(cfg.Stats.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("Worker Sender encerrado")
			return
		case <-ticker.C:
			// Coleta lista de containers rodando
			containers, err := dockerClient.ListRunningContainers(ctx)
			if err != nil {
				log.Error("Sender: Erro ao listar containers", slog.String("error", err.Error()))
				continue
			}

			// Coleta métricas completas e em paralelo via WaitGroup (GetAllContainerStats já faz isso)
			metricsMap, err := dockerClient.GetAllContainerStats(ctx)
			if err != nil {
				log.Error("Sender: Erro ao obter estatísticas", slog.String("error", err.Error()))
				continue
			}

			// Prepara o array final de métricas processadas
			var metricsList []models.ContainerMetrics
			for _, metric := range metricsMap {
				metric.HostID = cfg.Cluster.HostID
				metricsList = append(metricsList, metric)
			}

			// Injeta o HostID em todos os registros de Container
			for i := range containers {
				containers[i].HostID = cfg.Cluster.HostID
			}

			payload := models.PushPayload{
				HostID:     cfg.Cluster.HostID,
				Containers: containers,
				Metrics:    metricsList,
			}

			sendPushPayload(ctx, httpClient, cfg.Cluster.LeaderURL, payload, log)
		}
	}
}

func sendPushPayload(ctx context.Context, client *http.Client, url string, payload models.PushPayload, log *slog.Logger) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Error("Sender: erro ao parear payload JSON", slog.String("error", err.Error()))
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		log.Error("Sender: erro ao criar request", slog.String("error", err.Error()))
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Warn("Sender: falha de comunicação (Líder inativo?)",
			slog.String("url", url),
			slog.String("error", err.Error()),
		)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		log.Warn("Sender: envio rejeitado pelo líder",
			slog.Int("status_code", resp.StatusCode),
		)
	}
}
