package prune

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nicolas-moura-ti/castle-rock-agent/internal/docker"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/storage"
	"github.com/shirou/gopsutil/v3/disk"
)

// AutoPruner manages the disk check and cleanup cycle.
type AutoPruner struct {
	dockerClient docker.ContainerEngine
	store        *storage.SQLiteStore
	log          *slog.Logger

	thresholdPct float64
	checkPeriod  time.Duration
	pathToCheck  string

	// Prevents rapid consecutive executions
	lastPrune time.Time

	// dependency injection for testing
	diskCheckFunc func(ctx context.Context, path string) (*disk.UsageStat, error)
}

// NewAutoPruner creates a configured pruner instance to watch a path and threshold.
func NewAutoPruner(client docker.ContainerEngine, store *storage.SQLiteStore, triggerPct float64) *AutoPruner {
	// In most systems (even containerized via docker-socket),
	// checking the root FS ("/") is sufficient, or the host-mapped volume.
	return &AutoPruner{
		dockerClient:  client,
		store:         store,
		log:           slog.Default(),
		thresholdPct:  triggerPct,
		checkPeriod:   time.Minute * 5, // Check disk every 5m
		pathToCheck:   "/",
		diskCheckFunc: disk.UsageWithContext,
	}
}

// Start launches the watchdog routine in background.
// Only terminates when the Context is cancelled.
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
	// If ran less than 1h ago, skip (avoids overhead if disk doesn't clear even after prune)
	if time.Since(p.lastPrune) < time.Hour {
		return
	}

	usage, err := p.diskCheckFunc(ctx, p.pathToCheck)
	if err != nil {
		p.log.Debug("pruner: failed to check disk usage", slog.String("error", err.Error()))
		return
	}

	if usage.UsedPercent >= p.thresholdPct {
		reclaimed, err := p.dockerClient.PruneUnused(ctx)
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
