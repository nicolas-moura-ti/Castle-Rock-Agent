// Package alerts — testes unitários para o motor de alertas.
package alerts

import (
	"testing"
	"time"

	"github.com/nicolas-moura-ti/castle-rock-agent/internal/config"
	"github.com/nicolas-moura-ti/castle-rock-agent/pkg/models"
)

// TestEvaluateCondition testa os operadores de comparação.
func TestEvaluateCondition(t *testing.T) {
	engine := NewEngine(nil)

	tests := []struct {
		name     string
		value    float64
		operator string
		thresh   float64
		expected bool
	}{
		{"greater_true", 90.0, ">", 80.0, true},
		{"greater_false", 50.0, ">", 80.0, false},
		{"greater_equal", 80.0, ">", 80.0, false},
		{"gte_true", 80.0, ">=", 80.0, true},
		{"less_true", 30.0, "<", 50.0, true},
		{"less_false", 70.0, "<", 50.0, false},
		{"lte_true", 50.0, "<=", 50.0, true},
		{"equal_true", 42.0, "==", 42.0, true},
		{"equal_false", 41.0, "==", 42.0, false},
		{"invalid_operator", 50.0, "!=", 50.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.evaluateCondition(tt.value, tt.operator, tt.thresh)
			if result != tt.expected {
				t.Errorf("evaluateCondition(%f, %q, %f) = %v, want %v",
					tt.value, tt.operator, tt.thresh, result, tt.expected)
			}
		})
	}
}

// TestGetMetricValue testa a extração de valores de métricas.
func TestGetMetricValue(t *testing.T) {
	engine := NewEngine(nil)

	stats := models.ContainerMetrics{
		CPUPercent:    45.5,
		MemoryPercent: 67.3,
		MemoryUsage:   1024 * 1024 * 500,
		NetworkRx:     1000000,
		NetworkTx:     500000,
	}

	tests := []struct {
		metric   string
		expected float64
	}{
		{"cpu_percent", 45.5},
		{"memory_percent", 67.3},
		{"memory_usage", float64(1024 * 1024 * 500)},
		{"network_rx", 1000000},
		{"network_tx", 500000},
		{"unknown", 0},
	}

	for _, tt := range tests {
		t.Run(tt.metric, func(t *testing.T) {
			result := engine.getMetricValue(stats, tt.metric)
			if result != tt.expected {
				t.Errorf("getMetricValue(%q) = %f, want %f", tt.metric, result, tt.expected)
			}
		})
	}
}

// TestEvaluateNoAlert verifica que nenhum alerta é disparado
// quando as condições não são atingidas.
func TestEvaluateNoAlert(t *testing.T) {
	rules := []config.AlertRule{
		{
			Name:      "High CPU",
			Metric:    "cpu_percent",
			Operator:  ">",
			Threshold: 80.0,
			Duration:  1 * time.Minute,
			Severity:  "critical",
		},
	}

	engine := NewEngine(rules)

	stats := map[string]models.ContainerMetrics{
		"abc123": {
			ContainerID:   "abc123",
			ContainerName: "test",
			CPUPercent:    30.0, // Below threshold
		},
	}

	alerts := engine.Evaluate(stats)
	if len(alerts) != 0 {
		t.Errorf("Expected 0 alerts, got %d", len(alerts))
	}
}

// TestEvaluateAlertFires verifica que um alerta é disparado
// quando a condição persiste por tempo suficiente.
func TestEvaluateAlertFires(t *testing.T) {
	rules := []config.AlertRule{
		{
			Name:      "High CPU",
			Metric:    "cpu_percent",
			Operator:  ">",
			Threshold: 80.0,
			Duration:  0, // Duration 0 = dispara imediatamente
			Severity:  "critical",
		},
	}

	engine := NewEngine(rules)

	stats := map[string]models.ContainerMetrics{
		"abc123": {
			ContainerID:   "abc123",
			ContainerName: "test-container",
			CPUPercent:    95.0,
		},
	}

	// Primeira avaliação: registra início da condição
	engine.Evaluate(stats)

	// Segunda avaliação: como duration=0, deve disparar
	alerts := engine.Evaluate(stats)
	if len(alerts) != 1 {
		t.Fatalf("Expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].ContainerName != "test-container" {
		t.Errorf("Alert container name = %q, want %q", alerts[0].ContainerName, "test-container")
	}
	if alerts[0].Severity != "critical" {
		t.Errorf("Alert severity = %q, want %q", alerts[0].Severity, "critical")
	}

	// Terceira avaliação: não deve repetir o alerta
	alerts = engine.Evaluate(stats)
	if len(alerts) != 0 {
		t.Errorf("Expected 0 repeated alerts, got %d", len(alerts))
	}
}

// TestAlertResolvesWhenConditionClears verifica que alertas
// são resolvidos quando a condição deixa de ser verdadeira.
func TestAlertResolvesWhenConditionClears(t *testing.T) {
	rules := []config.AlertRule{
		{
			Name:      "High CPU",
			Metric:    "cpu_percent",
			Operator:  ">",
			Threshold: 80.0,
			Duration:  0,
			Severity:  "warning",
		},
	}

	engine := NewEngine(rules)

	// CPU alta — deve disparar
	highStats := map[string]models.ContainerMetrics{
		"abc123": {ContainerID: "abc123", ContainerName: "test", CPUPercent: 95.0},
	}
	engine.Evaluate(highStats)
	engine.Evaluate(highStats)
	active := engine.GetActiveAlerts()
	if len(active) != 1 {
		t.Fatalf("Expected 1 active alert after high CPU")
	}

	// CPU normaliza — alerta deve resolver
	normalStats := map[string]models.ContainerMetrics{
		"abc123": {ContainerID: "abc123", ContainerName: "test", CPUPercent: 30.0},
	}
	engine.Evaluate(normalStats)
	active = engine.GetActiveAlerts()
	if len(active) != 0 {
		t.Errorf("Expected 0 active alerts after CPU normalized, got %d", len(active))
	}
}
