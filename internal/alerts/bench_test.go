package alerts_test

import (
	"fmt"
	"testing"

	"github.com/nicolas-moura-ti/castle-rock-agent/internal/alerts"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/config"
	"github.com/nicolas-moura-ti/castle-rock-agent/pkg/models"
)

func BenchmarkClearInactiveAlerts(b *testing.B) {
	numAlerts := 1000

	rules := []config.AlertRule{
		{
			Name:      "High CPU",
			Metric:    "cpu_percent",
			Operator:  ">",
			Threshold: 80.0,
			Duration:  0,
			Severity:  "critical",
		},
	}

	engine := alerts.NewEngine(rules)

	stats1 := make(map[string]models.ContainerMetrics)
	for i := 0; i < numAlerts; i++ {
		cid := fmt.Sprintf("container-%d", i)
		stats1[cid] = models.ContainerMetrics{
			ContainerID:   cid,
			ContainerName: cid,
			CPUPercent:    90.0,
		}
	}
	engine.Evaluate(stats1)
	engine.Evaluate(stats1)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		engine.Evaluate(stats1)
	}
}
