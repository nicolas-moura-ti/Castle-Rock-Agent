package tui

import (
	"strings"
	"testing"

	"github.com/nicolas-moura-ti/castle-rock-agent/internal/logger"
	"github.com/nicolas-moura-ti/castle-rock-agent/pkg/models"
)

func BenchmarkAppendRestartAndLimits(b *testing.B) {
	m := Model{}
	c := logger.ContainerDisplay{
		RestartPolicy: "always",
		RestartCount:  5,
		CPULimit:      2.0,
		MemoryLimit:   1024 * 1024 * 1024,
	}
	var sb strings.Builder

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sb.Reset()
		m.appendRestartAndLimits(&sb, c)
	}
}

func BenchmarkAppendCommandAndMounts(b *testing.B) {
	m := Model{}
	c := logger.ContainerDisplay{
		Entrypoint: "/usr/bin/super-command",
		Mounts: []string{
			"/var/lib/mysql:/var/lib/mysql",
			"/etc/mysql:/etc/mysql",
			"/var/log/mysql:/var/log/mysql",
		},
	}
	var sb strings.Builder

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sb.Reset()
		m.appendCommandAndMounts(&sb, c)
	}
}

func BenchmarkAppendHealthAndEnv(b *testing.B) {
	m := Model{}
	c := logger.ContainerDisplay{
		HealthStatus: "healthy",
		HealthLog:    "Checks passed",
		Env: []string{
			"DB_HOST=localhost",
			"DB_PORT=3306",
			"DB_USER=root",
			"DB_PASS=secret-password-that-is-long-enough-to-be-truncated-by-the-logic",
		},
	}
	var sb strings.Builder

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sb.Reset()
		m.appendHealthAndEnv(&sb, c)
	}
}

func BenchmarkAppendStats(b *testing.B) {
	m := Model{
		stats: map[string]models.ContainerMetrics{
			"test-id": {
				CPUPercent:    12.5,
				MemoryUsage:   256 * 1024 * 1024,
				MemoryLimit:   512 * 1024 * 1024,
				MemoryPercent: 50.0,
				NetworkRx:     1024 * 1024,
				NetworkTx:     2048 * 1024,
			},
		},
	}
	c := logger.ContainerDisplay{
		ID: "test-id",
	}
	var sb strings.Builder

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sb.Reset()
		m.appendStats(&sb, c)
	}
}
