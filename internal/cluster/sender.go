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

// StartSender starts the routine that collects local Docker data
// and sends it (HTTP POST) to the Leader at defined intervals.
func StartSender(ctx context.Context, dockerClient *docker.Client, cfg config.Config, log *slog.Logger) {
	// Only runs in worker mode
	if cfg.Cluster.Mode != "worker" {
		return
	}

	log.Info("Starting Worker Sender",
		slog.String("leader_url", cfg.Cluster.LeaderURL),
		slog.String("host_id", cfg.Cluster.HostID),
		slog.Duration("interval", cfg.Stats.Interval),
	)

	// Creates HTTP client with guaranteed timeout
	httpClient := &http.Client{
		Timeout: 5 * time.Second,
	}

	ticker := time.NewTicker(cfg.Stats.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("Worker Sender stopped")
			return
		case <-ticker.C:
			// Collect list of running containers
			containers, err := dockerClient.ListRunningContainers(ctx)
			if err != nil {
				log.Error("Sender: error listing containers", slog.String("error", err.Error()))
				continue
			}

			// Collect full metrics in parallel via WaitGroup (GetAllContainerStats already does this)
			metricsMap, err := dockerClient.GetAllContainerStats(ctx)
			if err != nil {
				log.Error("Sender: error fetching stats", slog.String("error", err.Error()))
				continue
			}

			// Prepare final array of processed metrics
			var metricsList []models.ContainerMetrics
			for _, metric := range metricsMap {
				metric.HostID = cfg.Cluster.HostID
				metricsList = append(metricsList, metric)
			}

			// Inject HostID into all Container records
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
		log.Error("Sender: error marshaling payload JSON", slog.String("error", err.Error()))
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		log.Error("Sender: error creating request", slog.String("error", err.Error()))
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Warn("Sender: communication failure (Leader inactive?)",
			slog.String("url", url),
			slog.String("error", err.Error()),
		)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		log.Warn("Sender: push rejected by leader",
			slog.Int("status_code", resp.StatusCode),
		)
	}
}
