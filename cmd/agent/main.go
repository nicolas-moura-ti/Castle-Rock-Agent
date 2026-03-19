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

// App holds the main application dependencies.
type App struct {
	cfg             config.Config
	log             *slog.Logger
	ctx             context.Context
	dockerClient    *docker.Client
	clusterProvider metrics.ClusterProvider
	sysInfo         map[string]string
}

func main() {
	app := &App{}
	app.loadConfig()
	app.log = logger.Setup(app.cfg.LogLevel)

	ctx, stop := app.setupContext()
	app.ctx = ctx
	defer stop()

	app.log.Info("Castle Rock Agent starting",
		slog.String("version", Version),
		slog.String("go_version", runtime.Version()),
		slog.String("log_level", app.cfg.LogLevel),
	)

	if err := app.initDockerClient(); err != nil {
		return
	}
	defer app.closeDockerClient()

	app.setupClusterReceiver()

	defer app.startMDNSAdvertiser()()

	app.startPrometheusExporter()

	app.runExecutionMode()
}

func (a *App) loadConfig() {
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		slog.Warn("failed to load config, using defaults",
			slog.String("error", err.Error()),
		)
		cfg = config.DefaultConfig()
	}
	a.cfg = cfg
}

func (a *App) setupContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
}

func (a *App) closeDockerClient() {
	if err := a.dockerClient.Close(); err != nil {
		a.log.Warn("error closing docker client",
			slog.String("error", err.Error()),
		)
	}
}

func (a *App) startMDNSAdvertiser() func() {
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

func (a *App) runExecutionMode() {
	mode := os.Getenv("CASTLE_ROCK_MODE")

	if a.cfg.Cluster.Mode == "worker" {
		a.runWorkerMode()
		return
	}

	if mode == "headless" || a.cfg.Cluster.Mode == "leader" {
		a.runHeadlessMode()
		return
	}

	a.runTUIMode()
}

func (a *App) runWorkerMode() {
	a.log.Info("starting in worker mode",
		slog.String("leader_url", a.cfg.Cluster.LeaderURL),
	)
	go cluster.StartSender(a.ctx, a.dockerClient, a.cfg, a.log)

	<-a.ctx.Done()
	a.log.Info("Castle Rock Agent stopped (worker)")
}

func (a *App) runHeadlessMode() {
	a.log.Info("headless/leader mode — waiting for connections and scraping",
		slog.Int("port", a.cfg.Prometheus.Port),
	)
	<-a.ctx.Done()
	a.log.Info("Castle Rock Agent stopped (headless/leader)")
}

func (a *App) runTUIMode() {
	store, err := a.setupStorageAndPruner()
	if err == nil && store != nil {
		defer store.Close()
	}

	a.log.Info("starting interactive dashboard...")
	if err := tui.Run(a.dockerClient, a.clusterProvider, a.ctx, a.sysInfo, Version, a.cfg, store); err != nil {
		a.log.Error("TUI error", slog.String("error", err.Error()))
		return
	}
	a.log.Info("Castle Rock Agent stopped")
}

func (a *App) initDockerClient() error {
	dockerClient, err := docker.NewClient()
	if err != nil {
		a.log.Error("failed to create docker client",
			slog.String("error", err.Error()),
			slog.String("hint", "check if Docker Desktop is running"),
		)
		return err
	}

	dockerClient.SetIncludeContainers(a.cfg.Stats.IncludeContainers)
	a.dockerClient = dockerClient

	a.log.Info("docker daemon connected")

	sysInfo := make(map[string]string)
	info, err := dockerClient.GetSystemInfo(a.ctx)
	if err != nil {
		a.log.Warn("unable to get docker info",
			slog.String("error", err.Error()),
		)
	} else {
		sysInfo = info
		a.log.Info("docker info collected",
			slog.String("version", sysInfo["Server Version"]),
		)
	}
	a.sysInfo = sysInfo

	return nil
}

func (a *App) setupClusterReceiver() {
	if a.cfg.Cluster.Mode == "leader" {
		store := cluster.NewMemoryStore(a.log)
		a.clusterProvider = cluster.NewReceiver(a.log, store, a.cfg.Cluster.SharedSecret, a.cfg.Cluster.AuthToken)
		a.log.Info("Starting in LEADER mode. Will receive metrics on /api/v1/push",
			slog.Bool("auth_token_enabled", a.cfg.Cluster.AuthToken != ""),
			slog.Bool("aes_encryption_enabled", a.cfg.Cluster.SharedSecret != ""),
			slog.String("host_id", a.cfg.Cluster.HostID))
	}
}

func (a *App) startPrometheusExporter() {
	if a.cfg.Prometheus.Enabled {
		exporter := metrics.NewExporter(a.dockerClient, a.clusterProvider, a.cfg.Cluster.HostID, a.cfg.Stats.Interval, a.cfg.Prometheus.Port, a.log)
		exporter.Start(a.ctx)
		a.log.Info("prometheus exporter active",
			slog.Int("port", a.cfg.Prometheus.Port),
			slog.Duration("interval", a.cfg.Stats.Interval),
		)
	}
}

func (a *App) setupStorageAndPruner() (*storage.SQLiteStore, error) {
	// Storage (SQLite)
	store, err := storage.NewSQLiteStore("castle-rock-events.db")
	if err != nil {
		a.log.Error("failed to init sqlite store", slog.String("error", err.Error()))
	}

	// Auto Pruner (Garbage Collector)
	if a.cfg.Prune.Enabled {
		pruner := prune.NewAutoPruner(a.dockerClient, store, a.cfg.Prune.TriggerDiskPercent)
		go pruner.Start(a.ctx)
		a.log.Info("auto-prune enabled", slog.Float64("threshold_pct", a.cfg.Prune.TriggerDiskPercent))
	}

	return store, err
}