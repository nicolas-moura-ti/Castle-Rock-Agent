// Package main is the entrypoint for the Castle Rock Agent.
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
	store           *storage.SQLiteStore
}

func main() {
	app := &App{}
	
	// 1. Configuração e Logger
	app.loadConfig()
	app.log = logger.Setup(app.cfg.LogLevel)

	// 2. Contexto com cancelamento gracioso
	ctx, stop := app.setupContext()
	app.ctx = ctx
	defer stop()

	app.log.Info("Castle Rock Agent starting",
		slog.String("version", Version),
		slog.String("go_version", runtime.Version()),
		slog.String("log_level", app.cfg.LogLevel),
	)

	// 3. Inicialização de dependências
	if err := app.initDockerClient(); err != nil {
		return
	}
	defer app.closeDockerClient()

	app.setupClusterReceiver()
	defer app.startMDNSAdvertiser()()

	// 4. Servidor de Métricas e Execução
	app.startPrometheusExporter()
	app.runExecutionMode()
}

// loadConfig carrega o YAML com fallback para defaults.
func (a *App) loadConfig() {
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		slog.Warn("failed to load config, using defaults", slog.String("error", err.Error()))
		cfg = config.DefaultConfig()
	}
	a.cfg = cfg
}

// setupContext configura a escuta de sinais do SO.
func (a *App) setupContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
}

// initDockerClient conecta ao socket do Docker e coleta infos básicas.
func (a *App) initDockerClient() error {
	cli, err := docker.NewClient()
	if err != nil {
		a.log.Error("failed to create docker client",
			slog.String("error", err.Error()),
			slog.String("hint", "check if Docker Desktop is running"),
		)
		return err
	}

	cli.SetIncludeContainers(a.cfg.Stats.IncludeContainers)
	a.dockerClient = cli
	a.log.Info("docker daemon connected")

	info, err := cli.GetSystemInfo(a.ctx)
	if err != nil {
		a.log.Warn("unable to get docker info", slog.String("error", err.Error()))
	} else {
		a.sysInfo = info
		a.log.Info("docker info collected", slog.String("version", info["Server Version"]))
	}

	return nil
}

// closeDockerClient garante o fechamento limpo da conexão.
func (a *App) closeDockerClient() {
	if a.dockerClient != nil {
		if err := a.dockerClient.Close(); err != nil {
			a.log.Warn("error closing docker client", slog.String("error", err.Error()))
		}
	}
}

// setupClusterReceiver configura o modo Leader para receber métricas.
func (a *App) setupClusterReceiver() {
	if a.cfg.Cluster.Mode == "leader" {
		memStore := cluster.NewMemoryStore(a.log)
		a.clusterProvider = cluster.NewReceiver(a.log, memStore, a.cfg.Cluster.SharedSecret, a.cfg.Cluster.AuthToken)
		a.log.Info("Starting in LEADER mode. Will receive metrics on /api/v1/push",
			slog.Bool("auth_token_enabled", a.cfg.Cluster.AuthToken != ""),
			slog.Bool("aes_encryption_enabled", a.cfg.Cluster.SharedSecret != ""),
			slog.String("host_id", a.cfg.Cluster.HostID))
	}
}

// startMDNSAdvertiser anuncia o líder na rede local.
func (a *App) startMDNSAdvertiser() func() {
	if a.cfg.Cluster.Mode == "leader" {
		advertiser, err := cluster.NewAdvertiser(a.cfg.Prometheus.Port, a.log)
		if err == nil {
			return func() { _ = advertiser.Close() }
		}
		a.log.Warn("mDNS: failed to start advertiser", slog.String("error", err.Error()))
	}
	return func() {}
}

// startPrometheusExporter ativa o endpoint /metrics.
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

// runExecutionMode define se roda TUI, Headless ou Worker.
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
	a.log.Info("starting in worker mode", slog.String("leader_url", a.cfg.Cluster.LeaderURL))
	go cluster.StartSender(a.ctx, a.dockerClient, a.cfg, a.log)

	<-a.ctx.Done()
	a.log.Info("Castle Rock Agent stopped (worker)")
}

func (a *App) runHeadlessMode() {
	a.log.Info("headless/leader mode — waiting for connections", slog.Int("port", a.cfg.Prometheus.Port))
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
}

func (a *App) setupStorageAndPruner() (*storage.SQLiteStore, error) {
	// Storage (SQLite)
	store, err := storage.NewSQLiteStore("castle-rock-events.db")
	if err != nil {
		a.log.Error("failed to init sqlite store", slog.String("error", err.Error()))
		return nil, err
	}
	a.store = store

	// Auto Pruner
	if a.cfg.Prune.Enabled {
		pruner := prune.NewAutoPruner(a.dockerClient, store, a.cfg.Prune.TriggerDiskPercent)
		go pruner.Start(a.ctx)
		a.log.Info("auto-prune enabled", slog.Float64("threshold_pct", a.cfg.Prune.TriggerDiskPercent))
	}

	return store, nil
}