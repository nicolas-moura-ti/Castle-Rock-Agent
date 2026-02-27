// Package main é o entrypoint do Castle Rock Agent.
//
// Este arquivo demonstra práticas idiomáticas de Go para:
//   - Configuração via YAML + variáveis de ambiente (12-Factor App)
//   - Uso correto de context.Context para propagação de cancelamento
//   - Graceful shutdown com signal.NotifyContext
//   - TUI interativa com Bubble Tea
//   - Exportação de métricas Prometheus em /metrics
//   - Separação de responsabilidades usando packages internos
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/nicolas-moura-ti/castle-rock-agent/internal/config"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/docker"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/logger"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/metrics"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/tui"
)

const Version = "0.3.0"

func main() {
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		slog.Warn("Erro ao carregar config, usando defaults",
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

	log.Info("Castle Rock Agent iniciando",
		slog.String("version", Version),
		slog.String("go_version", runtime.Version()),
		slog.String("log_level", cfg.LogLevel),
	)

	dockerClient, err := docker.NewClient()
	if err != nil {
		log.Error("Falha ao criar Docker client",
			slog.String("error", err.Error()),
			slog.String("hint", "Verifique se o Docker Desktop está rodando"),
		)
		return
	}

	defer func() {
		if err := dockerClient.Close(); err != nil {
			log.Warn("Erro ao fechar Docker client",
				slog.String("error", err.Error()),
			)
		}
	}()

	log.Info("Docker daemon conectado")

	sysInfo := make(map[string]string)
	info, err := dockerClient.GetSystemInfo(ctx)
	if err != nil {
		log.Warn("Não foi possível obter info do Docker",
			slog.String("error", err.Error()),
		)
	} else {
		sysInfo = info
		log.Info("Docker info coletado",
			slog.String("version", sysInfo["Server Version"]),
		)
	}

	// ─────────────────────────────────────────────────────────────────────
	// PROMETHEUS EXPORTER
	// ─────────────────────────────────────────────────────────────────────
	if cfg.Prometheus.Enabled {
		exporter := metrics.NewExporter(dockerClient, cfg.Stats.Interval, cfg.Prometheus.Port, log)
		exporter.Start(ctx)
		log.Info("Prometheus exporter ativo",
			slog.Int("port", cfg.Prometheus.Port),
			slog.Duration("interval", cfg.Stats.Interval),
		)
	}

	// ─────────────────────────────────────────────────────────────────────
	// MODO DE EXECUÇÃO
	// ─────────────────────────────────────────────────────────────────────
	//
	// CASTLE_ROCK_MODE=headless:
	//   Roda sem TUI, ideal para Docker Compose / Kubernetes.
	//   O agente fica ativo apenas como servidor de métricas Prometheus.
	//
	// Modo padrão:
	//   TUI interativa com dashboard completo.
	mode := os.Getenv("CASTLE_ROCK_MODE")
	if mode == "headless" {
		log.Info("Modo headless — aguardando scraping Prometheus",
			slog.Int("port", cfg.Prometheus.Port),
		)
		// Bloqueia até receber sinal de shutdown
		<-ctx.Done()
		log.Info("Castle Rock Agent encerrado (headless)")
		return
	}

	// Modo TUI
	log.Info("Iniciando dashboard interativo...")
	if err := tui.Run(dockerClient, ctx, sysInfo, Version, cfg); err != nil {
		log.Error("Erro na TUI", slog.String("error", err.Error()))
		return
	}
	log.Info("Castle Rock Agent encerrado")
}
