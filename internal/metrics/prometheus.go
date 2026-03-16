// Package metrics provides the Prometheus metrics exporter.
//
// PROMETHEUS — THE OBSERVABILITY STANDARD:
//
//	Prometheus is the most widely used monitoring system in cloud-native
//	environments (CNCF graduated project). It uses a PULL model:
//	  1. The application exposes metrics on an HTTP endpoint (/metrics)
//	  2. The Prometheus server periodically scrapes that endpoint
//	  3. Metrics are stored in a time-series database
//	  4. Grafana visualizes the metrics in dashboards
//
// PROMETHEUS METRIC TYPES:
//   - Gauge: value that goes up and down (e.g. CPU%, memory used)
//   - Counter: value that only increases (e.g. total requests)
//   - Histogram: value distribution (e.g. request latency)
//   - Summary: like histogram, but computes percentiles client-side
//
// In this exporter, we use GaugeVec (gauge with labels) because container
// metrics are instantaneous values that vary continuously.
package metrics

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/nicolas-moura-ti/castle-rock-agent/pkg/models"
)

// MetricsProvider defines what a data source must provide for Prometheus.
type MetricsProvider interface {
	ListRunningContainers(ctx context.Context, all bool) ([]models.ContainerInfo, error)
	GetAllContainerStats(ctx context.Context, containers []models.ContainerInfo) (map[string]models.ContainerMetrics, error)
}

// ClusterProvider defines what a cluster source must provide for Prometheus.
type ClusterProvider interface {
	GetAllMetrics() []models.ContainerMetrics
	GetAllContainers() []models.ContainerInfo
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}

// Exporter manages Prometheus metrics and the HTTP server.
type Exporter struct {
	dockerClient MetricsProvider
	interval     time.Duration
	port         int
	log          *slog.Logger

	// receiver (optional) brings data from other cluster nodes in Leader mode
	receiver ClusterProvider

	// hostID is this machine's own name
	hostID string

	// Prometheus metrics.
	// GaugeVec allows multiple series with different labels.
	// Labels: container_id, container_name, image
	cpuPercent    *prometheus.GaugeVec
	memoryUsage   *prometheus.GaugeVec
	memoryLimit   *prometheus.GaugeVec
	memoryPercent *prometheus.GaugeVec
	networkRx     *prometheus.GaugeVec
	networkTx     *prometheus.GaugeVec
	blockRead     *prometheus.GaugeVec
	blockWrite    *prometheus.GaugeVec

	// Info gauge (always 1, carries metadata as labels)
	containerInfo *prometheus.GaugeVec

	// Lifecycle control
	mu        sync.RWMutex
	lastStats map[string]models.ContainerMetrics
}

// containerLabels defines the labels used in all metrics.
// Labels are dimensions that allow filtering and grouping metrics.
//
// Example PromQL query:
//
//	castle_rock_container_cpu_percent{container_name="postgres"}
var containerLabels = []string{"host_id", "container_id", "container_name", "image"}

// NewExporter creates a new Prometheus exporter.
//
// PROMETHEUS naming convention for metrics:
//   - Prefix: application name (castle_rock_)
//   - Suffix: unit (_bytes, _percent, _total)
//   - Snake_case always
//   - Reference: https://prometheus.io/docs/practices/naming/
func NewExporter(dockerClient MetricsProvider, receiver ClusterProvider, hostID string, interval time.Duration, port int, log *slog.Logger) *Exporter {
	e := &Exporter{
		dockerClient: dockerClient,
		interval:     interval,
		port:         port,
		log:          log,
		receiver:     receiver,
		hostID:       hostID,
		lastStats:    make(map[string]models.ContainerMetrics),

		cpuPercent: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "castle_rock",
			Subsystem: "container",
			Name:      "cpu_percent",
			Help:      "CPU usage percentage of the container",
		}, containerLabels),

		memoryUsage: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "castle_rock",
			Subsystem: "container",
			Name:      "memory_usage_bytes",
			Help:      "Memory usage of the container in bytes",
		}, containerLabels),

		memoryLimit: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "castle_rock",
			Subsystem: "container",
			Name:      "memory_limit_bytes",
			Help:      "Memory limit of the container in bytes",
		}, containerLabels),

		memoryPercent: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "castle_rock",
			Subsystem: "container",
			Name:      "memory_percent",
			Help:      "Memory usage percentage of the container",
		}, containerLabels),

		networkRx: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "castle_rock",
			Subsystem: "container",
			Name:      "network_rx_bytes",
			Help:      "Total bytes received over the network",
		}, containerLabels),

		networkTx: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "castle_rock",
			Subsystem: "container",
			Name:      "network_tx_bytes",
			Help:      "Total bytes transmitted over the network",
		}, containerLabels),

		blockRead: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "castle_rock",
			Subsystem: "container",
			Name:      "block_read_bytes",
			Help:      "Total bytes read from disk",
		}, containerLabels),

		blockWrite: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "castle_rock",
			Subsystem: "container",
			Name:      "block_write_bytes",
			Help:      "Total bytes written to disk",
		}, containerLabels),

		containerInfo: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "castle_rock",
			Subsystem: "container",
			Name:      "info",
			Help:      "Container info (value always 1, metadata in labels)",
		}, containerLabels),
	}

	// Register all metrics. We use Register instead of MustRegister
	// to avoid panics during tests if the metrics are already registered.
	metricsList := []prometheus.Collector{
		e.cpuPercent,
		e.memoryUsage,
		e.memoryLimit,
		e.memoryPercent,
		e.networkRx,
		e.networkTx,
		e.blockRead,
		e.blockWrite,
		e.containerInfo,
	}

	for _, m := range metricsList {
		if err := prometheus.Register(m); err != nil {
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				log.Warn("failed to register metric", slog.String("error", err.Error()))
			}
		}
	}

	return e
}

// Start initializes the exporter: HTTP server + collection loop.
//
// LONG-RUNNING GOROUTINES:
//
//	We launch two goroutines:
//	1. HTTP server (blocks on configured port)
//	2. Collection loop (periodic ticker)
//
//	Both are controlled by context — when cancelled, both terminate.
func (e *Exporter) Start(ctx context.Context) {
	// Goroutine 1: HTTP server for /metrics
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		// Health check endpoint
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"status":"ok"}`)
		})

		// Mount the Receiver (Leader mode) on the same HTTP server as Prometheus
		if e.receiver != nil {
			mux.Handle("/api/v1/push", e.receiver)
		}

		addr := fmt.Sprintf(":%d", e.port)
		server := &http.Server{
			Addr:         addr,
			Handler:      mux,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
		}

		e.log.Info("prometheus metrics server started",
			slog.String("endpoint", fmt.Sprintf("http://localhost%s/metrics", addr)),
			slog.Int("port", e.port),
		)

		// Goroutine for graceful HTTP server shutdown
		go func() {
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := server.Shutdown(shutdownCtx); err != nil {
				e.log.Warn("error during http server shutdown", slog.String("error", err.Error()))
			}
		}()

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			e.log.Error("prometheus server error",
				slog.String("error", err.Error()),
			)
		}
	}()

	// Goroutine 2: Periodic collection loop
	go func() {
		// Initial collection
		e.collect(ctx)

		ticker := time.NewTicker(e.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				e.collect(ctx)
			}
		}
	}()
}

// collect executes a metrics collection and updates Prometheus gauges.
func (e *Exporter) collect(ctx context.Context) {
	containers, err := e.dockerClient.ListRunningContainers(ctx, true)
	if err != nil {
		e.log.Debug("failed to list containers for prometheus",
			slog.String("error", err.Error()),
		)
		return
	}

	stats, err := e.dockerClient.GetAllContainerStats(ctx, containers)
	if err != nil {
		e.log.Debug("failed to collect stats for prometheus",
			slog.String("error", err.Error()),
		)
		return
	}

	// Reset old metrics (stopped containers)
	e.cpuPercent.Reset()
	e.memoryUsage.Reset()
	e.memoryLimit.Reset()
	e.memoryPercent.Reset()
	e.networkRx.Reset()
	e.networkTx.Reset()
	e.blockRead.Reset()
	e.blockWrite.Reset()
	e.containerInfo.Reset()

	// Prepare the total metrics list (Local + Remote from Cluster)
	var allStats []models.ContainerMetrics

	// Format local stats by adding the HostID
	for _, s := range stats {
		s.HostID = e.hostID
		allStats = append(allStats, s)
	}

	// Append remote stats if a receiver exists (Leader mode)
	if e.receiver != nil {
		allStats = append(allStats, e.receiver.GetAllMetrics()...)
	}

	// Update metrics for each container (local and remote combined)
	for _, s := range allStats {
		// Friendly fallback
		hid := s.HostID
		if hid == "" {
			hid = "unknown"
		}

		labels := prometheus.Labels{
			"host_id":        hid,
			"container_id":   s.ContainerID,
			"container_name": s.ContainerName,
			"image":          s.Image,
		}

		e.cpuPercent.With(labels).Set(s.CPUPercent)
		e.memoryUsage.With(labels).Set(float64(s.MemoryUsage))
		e.memoryLimit.With(labels).Set(float64(s.MemoryLimit))
		e.memoryPercent.With(labels).Set(s.MemoryPercent)
		e.networkRx.With(labels).Set(float64(s.NetworkRx))
		e.networkTx.With(labels).Set(float64(s.NetworkTx))
		e.blockRead.With(labels).Set(float64(s.BlockRead))
		e.blockWrite.With(labels).Set(float64(s.BlockWrite))
		e.containerInfo.With(labels).Set(1)
	}

	// Save stats for external access (by the TUI)
	e.mu.Lock()
	e.lastStats = stats
	e.mu.Unlock()
}

// GetLastStats returns the last collected metrics snapshot.
// Thread-safe — can be called from any goroutine.
func (e *Exporter) GetLastStats() map[string]models.ContainerMetrics {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Return a copy to avoid race conditions
	result := make(map[string]models.ContainerMetrics, len(e.lastStats))
	for k, v := range e.lastStats {
		result[k] = v
	}
	return result
}
