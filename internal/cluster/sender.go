package cluster

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/nicolas-moura-ti/castle-rock-agent/internal/config"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/docker"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/metrics"
	"github.com/nicolas-moura-ti/castle-rock-agent/pkg/models"
	"github.com/prometheus/client_golang/prometheus"
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
			containers, err := dockerClient.ListRunningContainers(ctx, true)
			if err != nil {
				log.Error("Sender: error listing containers", slog.String("error", err.Error()))
				continue
			}

			// Collect full metrics in parallel
			metricsMap, err := dockerClient.GetAllContainerStats(ctx, containers)
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

			// Resolvido: Passando Token e Segredo independentes
			sendPushPayload(ctx, httpClient, cfg.Cluster.LeaderURL, cfg.Cluster.SharedSecret, cfg.Cluster.AuthToken, payload, log)
		}
	}
}

func sendPushPayload(ctx context.Context, client *http.Client, url string, secret string, authToken string, payload models.PushPayload, log *slog.Logger) {
	// Measure the total duration of the push attempt (including retries)
	timer := prometheus.NewTimer(metrics.ClusterPushDuration)
	defer timer.ObserveDuration()

	data, err := json.Marshal(payload)
	if err != nil {
		log.Error("Sender: error marshaling payload JSON", slog.String("error", err.Error()))
		return
	}

	encrypted := false
	if secret != "" {
		data, err = Encrypt(data, secret)
		if err != nil {
			log.Error("Sender: error encrypting payload", slog.String("error", err.Error()))
			return
		}
		encrypted = true
	}

	// Prepare the compressed body once
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write(data); err != nil {
		log.Error("Sender: error compressing payload", slog.String("error", err.Error()))
		return
	}
	gw.Close()
	compressedData := buf.Bytes()

	// ─────────────────────────────────────────────────────────────────────
	// RETRY LOGIC (Exponential Backoff)
	// ─────────────────────────────────────────────────────────────────────
	maxRetries := 3
	backoff := 1 * time.Second

	for i := 0; i <= maxRetries; i++ {
		success := func() bool {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(compressedData))
			if err != nil {
				return false
			}

			if encrypted {
				req.Header.Set("Content-Type", "application/octet-stream")
				req.Header.Set("X-CastleRock-Encrypted", "true")
			} else {
				req.Header.Set("Content-Type", "application/json")
			}
			req.Header.Set("Content-Encoding", "gzip")
			if authToken != "" {
				req.Header.Set("Authorization", "Bearer "+authToken)
			}

			resp, err := client.Do(req)
			if err != nil {
				return false
			}
			defer resp.Body.Close()

			return resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted
		}()

		if success {
			return
		}

		if i < maxRetries {
			log.Warn("Sender: push failed, retrying...", 
				slog.Int("attempt", i+1), 
				slog.Duration("next_retry_in", backoff))
			
			select {
			case <-time.After(backoff):
				backoff *= 2 // Exponential backoff
			case <-ctx.Done():
				return
			}
		}
	}

	// If we reach here, all retries failed
	metrics.ClusterPushFailures.Inc()
	log.Error("Sender: push failed after all retries", slog.String("url", url))
}