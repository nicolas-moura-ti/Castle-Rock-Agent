package prune

import (
	"context"
	"fmt"
	"time"

	"github.com/nicolas-moura-ti/castle-rock-agent/internal/docker"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/storage"
	"github.com/shirou/gopsutil/v3/disk"
)

// AutoPruner gerencia o ciclo de verificação de disco e limpeza
type AutoPruner struct {
	dockerClient *docker.Client
	store        *storage.SQLiteStore

	thresholdPct float64
	checkPeriod  time.Duration
	pathToCheck  string

	// Previne execuções seguidas muito rapidamente
	lastPrune time.Time
}

// NewAutoPruner cria uma instância do pruner configurada para vigiar uma rota e limite
func NewAutoPruner(client *docker.Client, store *storage.SQLiteStore, triggerPct float64) *AutoPruner {
	// Geralmente na maioria dos sistemas (mesmo containerizados via docker-socket)
	// avaliar o root FS ("/") é o suficiente, ou o próprio volume host mapeado.
	return &AutoPruner{
		dockerClient: client,
		store:        store,
		thresholdPct: triggerPct,
		checkPeriod:  time.Minute * 5, // Checa o disco a cada 5m
		pathToCheck:  "/",
	}
}

// Start dispara a rotina de watchdog em background.
// Só termina quando o Context morre.
func (p *AutoPruner) Start(ctx context.Context) {
	ticker := time.NewTicker(p.checkPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.checkAndPrune(ctx)
		}
	}
}

func (p *AutoPruner) checkAndPrune(ctx context.Context) {
	// Se rodou há menos de 1h, skip (evita oneração caso disco não esvazie mesmo com prune)
	if time.Since(p.lastPrune) < time.Hour {
		return
	}

	usage, err := disk.UsageWithContext(ctx, p.pathToCheck)
	if err != nil {
		return
	}

	if usage.UsedPercent >= p.thresholdPct {
		reclaimed, err := p.dockerClient.PruneSystem(ctx)
		p.lastPrune = time.Now()

		if p.store != nil {
			msg := fmt.Sprintf("AutoPrune triggered. %.1f%% used.", usage.UsedPercent)
			if err == nil {
				mb := float64(reclaimed) / 1024 / 1024
				msg += fmt.Sprintf(" Recovered: %.1f MB.", mb)
			} else {
				msg += fmt.Sprintf(" Error: %v", err)
			}
			p.store.SaveEvent(ctx, "prune", "host", msg)
		}
	}
}
