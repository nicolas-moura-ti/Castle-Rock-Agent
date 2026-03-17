// Package main is the entrypoint for the Castle Rock Agent.
//
// This file demonstrates idiomatic Go practices for:
//   - Configuration via YAML + environment variables (12-Factor App)
//   - Correct usage of context.Context for cancellation propagation
//   - Graceful shutdown with signal.NotifyContext
//   - Interactive TUI with Bubble Tea
//   - Prometheus metrics export at /metrics
//   - Separation of concerns using internal packages
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/nicolas-moura-ti/castle-rock-agent/internal/cluster"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/config"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/docker"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/logger"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/metrics"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/prune"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/storage"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/tui"
)

const Version = "0.3.0"

func main() {
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		slog.Warn("failed to load config, using defaults",
			slog.String("error", err.Error()),
		)
		cfg = config.DefaultConfig()
	}

	log := logger.Setup(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()

	log.Info("Castle Rock Agent starting",
		slog.String("version", Version),
		slog.String("go_version", runtime.Version()),
		slog.String("log_level", cfg.LogLevel),
	)

	dockerClient, sysInfo, err := initDockerClient(ctx, &cfg, log)
	if err != nil {
		return
	}
	defer func() {
		if err := dockerClient.Close(); err != nil {
			log.Warn("error closing docker client",
				slog.String("error", err.Error()),
			)
		}
	}()

	clusterProvider := setupClusterReceiver(&cfg, log)
	
	// Start mDNS Service Discovery Advertising (if in leader mode)
	if cfg.Cluster.Mode == "leader" {
		if advertiser, err := cluster.NewAdvertiser(cfg.Prometheus.Port, log); err == nil {
			defer advertiser.Close()
		} else {
			log.Warn("mDNS: failed to start advertiser", slog.String("error", err.Error()))
		}
	}

	startPrometheusExporter(ctx, &cfg, log, dockerClient, clusterProvider)

	// ─────────────────────────────────────────────────────────────────────
	// EXECUTION MODE
	// ─────────────────────────────────────────────────────────────────────
	mode := os.Getenv("CASTLE_ROCK_MODE")

	if cfg.Cluster.Mode == "worker" {
		log.Info("starting in worker mode",
			slog.String("leader_url", cfg.Cluster.LeaderURL),
		)
		go cluster.StartSender(ctx, dockerClient, cfg, log)

		<-ctx.Done()
		log.Info("Castle Rock Agent stopped (worker)")
		return
	}

	if mode == "headless" || cfg.Cluster.Mode == "leader" {
		log.Info("headless/leader mode — waiting for connections and scraping",
			slog.Int("port", cfg.Prometheus.Port),
		)
		<-ctx.Done()
		log.Info("Castle Rock Agent stopped (headless/leader)")
		return
	}

	store, err := setupStorageAndPruner(ctx, &cfg, log, dockerClient)
	if err == nil && store != nil {
		defer store.Close()
	}

	// TUI Mode
	log.Info("starting interactive dashboard...")
	if err := tui.Run(dockerClient, clusterProvider, ctx, sysInfo, Version, cfg, store); err != nil {
		log.Error("TUI error", slog.String("error", err.Error()))
		return
	}
	log.Info("Castle Rock Agent stopped")
}

func initDockerClient(ctx context.Context, cfg *config.Config, log *slog.Logger) (*docker.Client, map[string]string, error) {
	dockerClient, err := docker.NewClient()
	if err != nil {
		log.Error("failed to create docker client",
			slog.String("error", err.Error()),
			slog.String("hint", "check if Docker Desktop is running"),
		)
		return nil, nil, err
	}

	dockerClient.SetIncludeContainers(cfg.Stats.IncludeContainers)

	log.Info("docker daemon connected")

	sysInfo := make(map[string]string)
	info, err := dockerClient.GetSystemInfo(ctx)
	if err != nil {
		log.Warn("unable to get docker info",
			slog.String("error", err.Error()),
		)
	} else {
		sysInfo = info
		log.Info("docker info collected",
			slog.String("version", sysInfo["Server Version"]),
		)
	}

	return dockerClient, sysInfo, nil
}

func setupClusterReceiver(cfg *config.Config, log *slog.Logger) metrics.ClusterProvider {
	var clusterProvider metrics.ClusterProvider
	if cfg.Cluster.Mode == "leader" {
		store := cluster.NewMemoryStore(log)
		clusterProvider = cluster.NewReceiver(log, store, cfg.Cluster.SharedSecret, cfg.Cluster.AuthToken)
		log.Info("Starting in LEADER mode. Will receive metrics on /api/v1/push",
			slog.Bool("auth_token_enabled", cfg.Cluster.AuthToken != ""),
			slog.Bool("aes_encryption_enabled", cfg.Cluster.SharedSecret != ""),
			slog.String("host_id", cfg.Cluster.HostID))
	}
	return clusterProvider
}

func startPrometheusExporter(ctx context.Context, cfg *config.Config, log *slog.Logger, dockerClient *docker.Client, clusterProvider metrics.ClusterProvider) {
	if cfg.Prometheus.Enabled {
		exporter := metrics.NewExporter(dockerClient, clusterProvider, cfg.Cluster.HostID, cfg.Stats.Interval, cfg.Prometheus.Port, log)
		exporter.Start(ctx)
		log.Info("prometheus exporter active",
			slog.Int("port", cfg.Prometheus.Port),
			slog.Duration("interval", cfg.Stats.Interval),
		)
	}
}

func setupStorageAndPruner(ctx context.Context, cfg *config.Config, log *slog.Logger, dockerClient *docker.Client) (*storage.SQLiteStore, error) {
	// Storage (SQLite)
	store, err := storage.NewSQLiteStore("castle-rock-events.db")
	if err != nil {
		log.Error("failed to init sqlite store", slog.String("error", err.Error()))
	}

	// Auto Pruner (Garbage Collector)
	if cfg.Prune.Enabled {
		pruner := prune.NewAutoPruner(dockerClient, store, cfg.Prune.TriggerDiskPercent)
		go pruner.Start(ctx)
		log.Info("auto-prune enabled", slog.Float64("threshold_pct", cfg.Prune.TriggerDiskPercent))
	}

	return store, err
}