// Package alerts implementa o motor de alertas do Castle Rock Agent.
//
// O motor avalia regras configuráveis contra métricas coletadas dos
// containers Docker. Quando uma condição persiste por tempo suficiente
// (duration), um alerta é disparado.
//
// DESIGN:
//   - Regras são carregadas do config.yaml
//   - Cada regra define: métrica, operador, threshold, duration, severity
//   - O motor mantém estado de "desde quando" cada condição está ativa
//   - Alertas só disparam após a condição persistir por >= duration
//   - Cada alerta dispara apenas uma vez (não repete enquanto ativo)
package alerts

import (
	"fmt"
	"sync"
	"time"

	"github.com/nicolas-moura-ti/castle-rock-agent/internal/config"
	"github.com/nicolas-moura-ti/castle-rock-agent/pkg/models"
)

// Alert representa um alerta ativo.
type Alert struct {
	RuleName      string
	ContainerID   string
	ContainerName string
	Metric        string
	CurrentValue  float64
	Threshold     float64
	Severity      string
	FiredAt       time.Time
	ActiveSince   time.Time
}

// Engine é o motor de avaliação de alertas.
//
// REGRA DE DESIGN:
//
//	O motor é thread-safe (protegido por mutex) porque pode ser
//	acessado tanto pela goroutine de coleta quanto pela TUI.
type Engine struct {
	rules []config.AlertRule

	// conditionStart rastreia desde quando cada condição está ativa.
	// Chave: "containerID:ruleName"
	conditionStart map[string]time.Time

	// activeAlerts armazena alertas que já foram disparados.
	// Chave: "containerID:ruleName"
	activeAlerts map[string]Alert

	// firedAlerts histórico de alertas (últimos 50)
	firedAlerts []Alert

	mu sync.Mutex
}

// NewEngine cria um novo motor de alertas.
func NewEngine(rules []config.AlertRule) *Engine {
	return &Engine{
		rules:          rules,
		conditionStart: make(map[string]time.Time),
		activeAlerts:   make(map[string]Alert),
		firedAlerts:    make([]Alert, 0),
	}
}

// Evaluate avalia todas as regras contra as métricas fornecidas.
//
// ALGORITMO:
//
//	Para cada container × cada regra:
//	  1. Extrai o valor da métrica relevante
//	  2. Aplica o operador de comparação
//	  3. Se condição TRUE:
//	     a. Se é novo → registra o início da condição
//	     b. Se já está ativo há >= duration → dispara alerta
//	  4. Se condição FALSE:
//	     a. Limpa o estado (reseta timer)
//
// Este padrão é similar ao que o Prometheus Alertmanager usa:
//
//	Pending → Firing → Resolved
func (e *Engine) Evaluate(stats map[string]models.ContainerMetrics) []Alert {
	e.mu.Lock()
	defer e.mu.Unlock()

	var newAlerts []Alert
	now := time.Now()

	// Limpa alertas de containers que não existem mais
	for key := range e.activeAlerts {
		found := false
		for _, s := range stats {
			if key == e.alertKey(s.ContainerID, e.activeAlerts[key].RuleName) {
				found = true
				break
			}
		}
		if !found {
			delete(e.activeAlerts, key)
			delete(e.conditionStart, key)
		}
	}

	for _, s := range stats {
		for _, rule := range e.rules {
			key := e.alertKey(s.ContainerID, rule.Name)

			// Extrai o valor da métrica
			value := e.getMetricValue(s, rule.Metric)

			// Avalia a condição
			conditionMet := e.evaluateCondition(value, rule.Operator, rule.Threshold)

			if conditionMet {
				// Condição ativa — verifica se já está sendo rastreada
				startTime, exists := e.conditionStart[key]
				if !exists {
					// Novo — registra início
					e.conditionStart[key] = now
					continue
				}

				// Verifica se já passou tempo suficiente (duration)
				if now.Sub(startTime) >= rule.Duration {
					// Verifica se já disparou (não repetir)
					if _, alreadyFired := e.activeAlerts[key]; !alreadyFired {
						alert := Alert{
							RuleName:      rule.Name,
							ContainerID:   s.ContainerID,
							ContainerName: s.ContainerName,
							Metric:        rule.Metric,
							CurrentValue:  value,
							Threshold:     rule.Threshold,
							Severity:      rule.Severity,
							FiredAt:       now,
							ActiveSince:   startTime,
						}

						e.activeAlerts[key] = alert
						newAlerts = append(newAlerts, alert)

						// Adiciona ao histórico
						e.firedAlerts = append([]Alert{alert}, e.firedAlerts...)
						if len(e.firedAlerts) > 50 {
							e.firedAlerts = e.firedAlerts[:50]
						}
					}
				}
			} else {
				// Condição não ativa — limpa estado
				delete(e.conditionStart, key)
				delete(e.activeAlerts, key)
			}
		}
	}

	return newAlerts
}

// GetActiveAlerts retorna todos os alertas atualmente ativos,
// filtrando para manter apenas o de maior severidade por container e métrica.
func (e *Engine) GetActiveAlerts() []Alert {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Agrupa por ContainerID + Metric para deduzir alertas duplicados
	// (ex: Warning e Critical de Memória ao mesmo tempo)
	bestAlerts := make(map[string]Alert)

	for _, a := range e.activeAlerts {
		key := a.ContainerID + ":" + a.Metric
		existing, ok := bestAlerts[key]
		if !ok {
			bestAlerts[key] = a
		} else {
			if severityWeight(a.Severity) > severityWeight(existing.Severity) {
				bestAlerts[key] = a
			}
		}
	}

	result := make([]Alert, 0, len(bestAlerts))
	for _, a := range bestAlerts {
		result = append(result, a)
	}
	return result
}

// severityWeight retorna um peso numérico para priorização de alertas
func severityWeight(severity string) int {
	switch severity {
	case "critical":
		return 3
	case "warning":
		return 2
	case "info":
		return 1
	default:
		return 0
	}
}

// GetPendingConditions retorna contagem de condições em avaliação
// (ativas mas que ainda não atingiram o duration threshold).
func (e *Engine) GetPendingCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()

	pending := 0
	for key := range e.conditionStart {
		if _, fired := e.activeAlerts[key]; !fired {
			pending++
		}
	}
	return pending
}

// alertKey gera a chave única para rastrear estado de uma condição.
func (e *Engine) alertKey(containerID, ruleName string) string {
	return containerID + ":" + ruleName
}

// getMetricValue extrai o valor numérico de uma métrica do container.
func (e *Engine) getMetricValue(stats models.ContainerMetrics, metric string) float64 {
	switch metric {
	case "cpu_percent":
		return stats.CPUPercent
	case "memory_percent":
		return stats.MemoryPercent
	case "memory_usage":
		return float64(stats.MemoryUsage)
	case "network_rx":
		return float64(stats.NetworkRx)
	case "network_tx":
		return float64(stats.NetworkTx)
	default:
		return 0
	}
}

// evaluateCondition aplica o operador de comparação.
func (e *Engine) evaluateCondition(value float64, operator string, threshold float64) bool {
	switch operator {
	case ">":
		return value > threshold
	case ">=":
		return value >= threshold
	case "<":
		return value < threshold
	case "<=":
		return value <= threshold
	case "==":
		return value == threshold
	default:
		return false
	}
}

// FormatAlert formata um alerta para exibição.
func FormatAlert(a Alert) string {
	return fmt.Sprintf("[%s] %s: %s %.1f%% > %.1f%% (container: %s)",
		a.Severity, a.RuleName, a.Metric,
		a.CurrentValue, a.Threshold, a.ContainerName,
	)
}
