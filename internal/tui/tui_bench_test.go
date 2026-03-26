package tui

import (
	"strings"
	"testing"
	"fmt"

	"github.com/nicolas-moura-ti/castle-rock-agent/internal/alerts"
)

func BenchmarkAppendSecurityAlerts(b *testing.B) {
	m := Model{
		securityAlerts: make([]alerts.Alert, 0, 10000),
	}

	for i := 0; i < 10000; i++ {
		m.securityAlerts = append(m.securityAlerts, alerts.Alert{
			ContainerID: fmt.Sprintf("container-%d", i),
			RuleName: "Test Rule",
		})
	}

	// add some alerts for the target container
	m.securityAlerts = append(m.securityAlerts, alerts.Alert{
		ContainerID: "target-container",
		RuleName: "Test Rule",
	})

	builder := &strings.Builder{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builder.Reset()
		m.appendSecurityAlerts(builder, "target-container", 80)
	}
}
