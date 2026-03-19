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

type Agent struct {
	cfg             config.Config
	log             *slog.Logger
	dockerClient    *docker.Client
	sysInfo         map[string]string
	clusterProvider metrics.ClusterProvider
	store           *storage.SQLiteStore
}

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

	agent := &Agent{
		cfg: cfg,
		log: log,
	}

	agent.log.Info("Castle Rock Agent starting",
		slog.String("version", Version),
		slog.String("go_version", runtime.Version()),
		slog.String("log_level", agent.cfg.LogLevel),
	)

	if err := agent.initDockerClient(ctx); err != nil {
		return
	}
	defer agent.closeDockerClient()

	agent.setupClusterReceiver()
	defer agent.startMDNSAdvertiser()()
	agent.startPrometheusExporter(ctx)

	agent.runExecutionMode(ctx)
}

func (a *Agent) initDockerClient(ctx context.Context) error {
	dockerClient, err := docker.NewClient()
	if err != nil {
		a.log.Error("failed to create docker client",
			slog.String("error", err.Error()),
			slog.String("hint", "check if Docker Desktop is running"),
		)
		return err
	}

	dockerClient.SetIncludeContainers(a.cfg.Stats.IncludeContainers)
	a.log.Info("docker daemon connected")

	a.sysInfo = make(map[string]string)
	info, err := dockerClient.GetSystemInfo(ctx)
	if err != nil {
		a.log.Warn("unable to get docker info",
			slog.String("error", err.Error()),
		)
	} else {
		a.sysInfo = info
		a.log.Info("docker info collected",
			slog.String("version", a.sysInfo["Server Version"]),
		)
	}

	a.dockerClient = dockerClient
	return nil
}

func (a *Agent) closeDockerClient() {
	if a.dockerClient != nil {
		if err := a.dockerClient.Close(); err != nil {
			a.log.Warn("error closing docker client",
				slog.String("error", err.Error()),
			)
		}
	}
}

func (a *Agent) setupClusterReceiver() {
	if a.cfg.Cluster.Mode == "leader" {
		memStore := cluster.NewMemoryStore(a.log)
		a.clusterProvider = cluster.NewReceiver(a.log, memStore, a.cfg.Cluster.SharedSecret, a.cfg.Cluster.AuthToken)
		a.log.Info("Starting in LEADER mode. Will receive metrics on /api/v1/push",
			slog.Bool("auth_token_enabled", a.cfg.Cluster.AuthToken != ""),
			slog.Bool("aes_encryption_enabled", a.cfg.Cluster.SharedSecret != ""),
			slog.String("host_id", a.cfg.Cluster.HostID))
	}
}

func (a *Agent) startMDNSAdvertiser() func() {
	if a.cfg.Cluster.Mode == "leader" {
		advertiser, err := cluster.NewAdvertiser(a.cfg.Prometheus.Port, a.log)
		if err == nil {
			return func() {
				_ = advertiser.Close()
			}
		}
		a.log.Warn("mDNS: failed to start advertiser", slog.String("error", err.Error()))
	}
	return func() {}
}

func (a *Agent) startPrometheusExporter(ctx context.Context) {
	if a.cfg.Prometheus.Enabled {
		exporter := metrics.NewExporter(a.dockerClient, a.clusterProvider, a.cfg.Cluster.HostID, a.cfg.Stats.Interval, a.cfg.Prometheus.Port, a.log)
		exporter.Start(ctx)
		a.log.Info("prometheus exporter active",
			slog.Int("port", a.cfg.Prometheus.Port),
			slog.Duration("interval", a.cfg.Stats.Interval),
		)
	}
}

func (a *Agent) runExecutionMode(ctx context.Context) {
	mode := os.Getenv("CASTLE_ROCK_MODE")

	if a.cfg.Cluster.Mode == "worker" {
		a.runWorkerMode(ctx)
		return
	}

	if mode == "headless" || a.cfg.Cluster.Mode == "leader" {
		a.runHeadlessMode(ctx)
		return
	}

	a.runTUIMode(ctx)
}

func (a *Agent) runWorkerMode(ctx context.Context) {
	a.log.Info("starting in worker mode",
		slog.String("leader_url", a.cfg.Cluster.LeaderURL),
	)
	go cluster.StartSender(ctx, a.dockerClient, a.cfg, a.log)

	<-ctx.Done()
	a.log.Info("Castle Rock Agent stopped (worker)")
}

func (a *Agent) runHeadlessMode(ctx context.Context) {
	a.log.Info("headless/leader mode — waiting for connections and scraping",
		slog.Int("port", a.cfg.Prometheus.Port),
	)
	<-ctx.Done()
	a.log.Info("Castle Rock Agent stopped (headless/leader)")
}

func (a *Agent) runTUIMode(ctx context.Context) {
	a.setupStorageAndPruner(ctx)
	if a.store != nil {
		defer a.store.Close()
	}

	a.log.Info("starting interactive dashboard...")
	if err := tui.Run(a.dockerClient, a.clusterProvider, ctx, a.sysInfo, Version, a.cfg, a.store); err != nil {
		a.log.Error("TUI error", slog.String("error", err.Error()))
		return
	}
	a.log.Info("Castle Rock Agent stopped")
}

func (a *Agent) setupStorageAndPruner(ctx context.Context) {
	// Storage (SQLite)
	store, err := storage.NewSQLiteStore("castle-rock-events.db")
	if err != nil {
		a.log.Error("failed to init sqlite store", slog.String("error", err.Error()))
	}
	a.store = store

	// Auto Pruner (Garbage Collector)
	if a.cfg.Prune.Enabled {
		pruner := prune.NewAutoPruner(a.dockerClient, store, a.cfg.Prune.TriggerDiskPercent)
		go pruner.Start(ctx)
		a.log.Info("auto-prune enabled", slog.Float64("threshold_pct", a.cfg.Prune.TriggerDiskPercent))
	}
}
