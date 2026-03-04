// Package alerts implements the Castle Rock Agent alert engine.
//
// The engine evaluates configurable rules against metrics collected from
// Docker containers. When a condition persists long enough (duration),
// an alert is fired.
//
// DESIGN:
//   - Rules are loaded from config.yaml
//   - Each rule defines: metric, operator, threshold, duration, severity
//   - The engine maintains state of "since when" each condition is active
//   - Alerts only fire after the condition persists for >= duration
//   - Each alert fires only once (does not repeat while active)
package alerts

import (
	"fmt"
	"sync"
	"time"

	"github.com/nicolas-moura-ti/castle-rock-agent/internal/config"
	"github.com/nicolas-moura-ti/castle-rock-agent/pkg/models"
)

// Alert represents an active alert.
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

// Engine is the alert evaluation engine.
//
// DESIGN RULE:
//
//	The engine is thread-safe (protected by mutex) because it can be
//	accessed by both the collection goroutine and the TUI.
type Engine struct {
	rules []config.AlertRule

	// conditionStart tracks since when each condition has been active.
	// Key: "containerID:ruleName"
	conditionStart map[string]time.Time

	// activeAlerts stores alerts that have already been fired.
	// Key: "containerID:ruleName"
	activeAlerts map[string]Alert

	// firedAlerts is the alert history (last 50)
	firedAlerts []Alert

	mu sync.Mutex
}

// NewEngine creates a new alert engine.
func NewEngine(rules []config.AlertRule) *Engine {
	return &Engine{
		rules:          rules,
		conditionStart: make(map[string]time.Time),
		activeAlerts:   make(map[string]Alert),
		firedAlerts:    make([]Alert, 0),
	}
}

// Evaluate evaluates all rules against the provided metrics.
//
// ALGORITHM:
//
//	For each container x each rule:
//	  1. Extract the relevant metric value
//	  2. Apply the comparison operator
//	  3. If condition TRUE:
//	     a. If new -> record the condition start time
//	     b. If already active for >= duration -> fire alert
//	  4. If condition FALSE:
//	     a. Clear state (reset timer)
//
// This pattern is similar to what Prometheus Alertmanager uses:
//
//	Pending -> Firing -> Resolved
func (e *Engine) Evaluate(stats map[string]models.ContainerMetrics) []Alert {
	e.mu.Lock()
	defer e.mu.Unlock()

	var newAlerts []Alert
	now := time.Now()

	// Clear alerts for containers that no longer exist
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

			// Extract the metric value
			value := e.getMetricValue(s, rule.Metric)

			// Evaluate the condition
			conditionMet := e.evaluateCondition(value, rule.Operator, rule.Threshold)

			if conditionMet {
				// Condition active — check if already being tracked
				startTime, exists := e.conditionStart[key]
				if !exists {
					// New — record start
					e.conditionStart[key] = now
					continue
				}

				// Check if enough time has passed (duration)
				if now.Sub(startTime) >= rule.Duration {
					// Check if already fired (don't repeat)
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

						// Add to history
						e.firedAlerts = append([]Alert{alert}, e.firedAlerts...)
						if len(e.firedAlerts) > 50 {
							e.firedAlerts = e.firedAlerts[:50]
						}
					}
				}
			} else {
				// Condition not active — clear state
				delete(e.conditionStart, key)
				delete(e.activeAlerts, key)
			}
		}
	}

	return newAlerts
}

// GetActiveAlerts returns all currently active alerts,
// filtering to keep only the highest severity per container and metric.
func (e *Engine) GetActiveAlerts() []Alert {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Group by ContainerID + Metric to deduplicate alerts
	// (e.g. Warning and Critical for Memory at the same time)
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

// severityWeight returns a numeric weight for alert prioritization.
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

// GetPendingCount returns the count of conditions being evaluated
// (active but haven't reached the duration threshold yet).
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

// alertKey generates the unique key for tracking a condition's state.
func (e *Engine) alertKey(containerID, ruleName string) string {
	return containerID + ":" + ruleName
}

// getMetricValue extracts the numeric value of a container metric.
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

// evaluateCondition applies the comparison operator.
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

// FormatAlert formats an alert for display.
func FormatAlert(a Alert) string {
	return fmt.Sprintf("[%s] %s: %s %.1f%% > %.1f%% (container: %s)",
		a.Severity, a.RuleName, a.Metric,
		a.CurrentValue, a.Threshold, a.ContainerName,
	)
}
