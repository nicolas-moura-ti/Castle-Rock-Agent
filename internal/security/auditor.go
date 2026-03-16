// Package security implements predictive security auditing for containers.
package security

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/alerts"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/docker"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/logger"
)

// SecurityRule defines the interface for a single security audit check.
// This follows the Open/Closed Principle (OCP), allowing new rules
// to be added without modifying the Auditor core.
type SecurityRule interface {
	Name() string
	Evaluate(c logger.ContainerDisplay, j types.ContainerJSON, now time.Time) []alerts.Alert
}

// Auditor analyzes running containers looking for risky configurations.
type Auditor struct {
	dockerClient *docker.Client
	rules        []SecurityRule

	// Cache to avoid running 'docker inspect' every second.
	cache map[string][]alerts.Alert
	mu    sync.RWMutex
}

// NewAuditor creates a new auditor with a default set of security rules.
func NewAuditor(client *docker.Client) *Auditor {
	a := &Auditor{
		dockerClient: client,
		cache:        make(map[string][]alerts.Alert),
	}
	a.registerDefaultRules()
	return a
}

func (a *Auditor) registerDefaultRules() {
	a.rules = []SecurityRule{
		&PrivilegedModeRule{},
		&RootUserRule{},
		&DBPortExposedRule{},
		&SensitiveCapsRule{},
		&ResourceQuotasRule{},
		&WritableRootFSRule{},
		&InsecurePortsRule{},
		&NoNewPrivilegesRule{},
		&HostNetworkingRule{},
		&SensitiveMountsRule{},
	}
}

// Audit scans the provided container list using registered rules.
func (a *Auditor) Audit(ctx context.Context, containers []logger.ContainerDisplay) []alerts.Alert {
	a.mu.Lock()
	defer a.mu.Unlock()

	var activeAlerts []alerts.Alert
	activeIDs := make(map[string]bool)
	now := time.Now()

	toInspect := make([]logger.ContainerDisplay, 0, len(containers))

	for _, c := range containers {
		activeIDs[c.ID] = true
		if cached, exists := a.cache[c.ID]; exists {
			for i := range cached {
				cached[i].FiredAt = now
				activeAlerts = append(activeAlerts, cached[i])
			}
			continue
		}
		toInspect = append(toInspect, c)
	}

	if len(toInspect) > 0 {
		var wg sync.WaitGroup
		var newAlertsMu sync.Mutex
		newAlerts := make(map[string][]alerts.Alert)

		for _, c := range toInspect {
			wg.Add(1)
			go func(c logger.ContainerDisplay) {
				defer wg.Done()
				inspectJSON, err := a.dockerClient.InspectContainer(ctx, c.ID)
				if err != nil {
					return
				}

				var secAlerts []alerts.Alert
				for _, rule := range a.rules {
					secAlerts = append(secAlerts, rule.Evaluate(c, inspectJSON, now)...)
				}

				newAlertsMu.Lock()
				newAlerts[c.ID] = secAlerts
				newAlertsMu.Unlock()
			}(c)
		}
		wg.Wait()

		for _, c := range toInspect {
			if secAlerts, ok := newAlerts[c.ID]; ok {
				a.cache[c.ID] = secAlerts
				activeAlerts = append(activeAlerts, secAlerts...)
			}
		}
	}

	for id := range a.cache {
		if !activeIDs[id] {
			delete(a.cache, id)
		}
	}
	return activeAlerts
}

// ─────────────────────────────────────────────────────────────────────────────
// CONCRETE RULES IMPLEMENTATION (SRP)
// ─────────────────────────────────────────────────────────────────────────────

type PrivilegedModeRule struct{}

func (r *PrivilegedModeRule) Name() string { return "Privileged Mode" }
func (r *PrivilegedModeRule) Evaluate(c logger.ContainerDisplay, j types.ContainerJSON, now time.Time) []alerts.Alert {
	if j.HostConfig != nil && j.HostConfig.Privileged {
		return []alerts.Alert{{
			RuleName: "Sec: Privileged Mode", ContainerID: c.ID, ContainerName: c.Name,
			Metric: "security_privileged", CurrentValue: 1, Severity: "critical", ActiveSince: now, FiredAt: now,
		}}
	}
	return nil
}

type RootUserRule struct{}

func (r *RootUserRule) Name() string { return "Root User" }
func (r *RootUserRule) Evaluate(c logger.ContainerDisplay, j types.ContainerJSON, now time.Time) []alerts.Alert {
	user := ""
	if j.Config != nil {
		user = j.Config.User
	}
	if user == "" || user == "root" || user == "0" {
		return []alerts.Alert{{
			RuleName: "Sec: Root User", ContainerID: c.ID, ContainerName: c.Name,
			Metric: "security_root_user", CurrentValue: 1, Severity: "warning", ActiveSince: now, FiredAt: now,
		}}
	}
	return nil
}

type DBPortExposedRule struct{}

func (r *DBPortExposedRule) Name() string { return "DB Port Exposed" }
func (r *DBPortExposedRule) Evaluate(c logger.ContainerDisplay, j types.ContainerJSON, now time.Time) []alerts.Alert {
	if j.NetworkSettings == nil {
		return nil
	}
	var alertsList []alerts.Alert
	for port, bindings := range j.NetworkSettings.Ports {
		portStr := string(port)
		isDB := strings.HasPrefix(portStr, "3306/") || strings.HasPrefix(portStr, "5432/") ||
			strings.HasPrefix(portStr, "27017/") || strings.HasPrefix(portStr, "6379/")
		if !isDB {
			continue
		}
		for _, b := range bindings {
			if b.HostIP == "0.0.0.0" || b.HostIP == "" || b.HostIP == "::" {
				alertsList = append(alertsList, alerts.Alert{
					RuleName: "Sec: DB Port Exposed globally", ContainerID: c.ID, ContainerName: c.Name,
					Metric: "security_db_port", CurrentValue: float64(port.Int()), Severity: "critical", ActiveSince: now, FiredAt: now,
				})
			}
		}
	}
	return alertsList
}

type SensitiveCapsRule struct{}

func (r *SensitiveCapsRule) Name() string { return "Sensitive Capabilities" }
func (r *SensitiveCapsRule) Evaluate(c logger.ContainerDisplay, j types.ContainerJSON, now time.Time) []alerts.Alert {
	if j.HostConfig == nil {
		return nil
	}
	var alertsList []alerts.Alert
	sensitive := map[string]bool{
		"SYS_ADMIN": true, "NET_ADMIN": true, "SYS_PTRACE": true,
		"DAC_OVERRIDE": true, "SYS_RAWIO": true, "SYS_MODULE": true, "NET_RAW": true,
	}
	for _, cap := range j.HostConfig.CapAdd {
		if sensitive[cap] {
			alertsList = append(alertsList, alerts.Alert{
				RuleName: "Sec: Sensitive CAP_ADD", ContainerID: c.ID, ContainerName: c.Name,
				Metric: "security_sensitive_cap", CurrentValue: 1, Severity: "warning", ActiveSince: now, FiredAt: now,
			})
		}
	}
	return alertsList
}

type ResourceQuotasRule struct{}

func (r *ResourceQuotasRule) Name() string { return "Resource Quotas" }
func (r *ResourceQuotasRule) Evaluate(c logger.ContainerDisplay, j types.ContainerJSON, now time.Time) []alerts.Alert {
	if j.HostConfig != nil && (j.HostConfig.Memory == 0 || j.HostConfig.NanoCPUs == 0) {
		return []alerts.Alert{{
			RuleName: "Sec: No Resource Quotas", ContainerID: c.ID, ContainerName: c.Name,
			Metric: "security_no_quotas", CurrentValue: 1, Severity: "warning", ActiveSince: now, FiredAt: now,
		}}
	}
	return nil
}

type WritableRootFSRule struct{}

func (r *WritableRootFSRule) Name() string { return "Writable RootFS" }
func (r *WritableRootFSRule) Evaluate(c logger.ContainerDisplay, j types.ContainerJSON, now time.Time) []alerts.Alert {
	if j.HostConfig != nil && !j.HostConfig.ReadonlyRootfs {
		return []alerts.Alert{{
			RuleName: "Sec: Writable RootFS", ContainerID: c.ID, ContainerName: c.Name,
			Metric: "security_writable_rootfs", CurrentValue: 1, Severity: "warning", ActiveSince: now, FiredAt: now,
		}}
	}
	return nil
}

type InsecurePortsRule struct{}

func (r *InsecurePortsRule) Name() string { return "Insecure Ports" }
func (r *InsecurePortsRule) Evaluate(c logger.ContainerDisplay, j types.ContainerJSON, now time.Time) []alerts.Alert {
	if j.NetworkSettings == nil {
		return nil
	}
	for port := range j.NetworkSettings.Ports {
		p := string(port)
		if strings.HasPrefix(p, "22/") || strings.HasPrefix(p, "23/") {
			return []alerts.Alert{{
				RuleName: "Sec: Insecure Port Exposed", ContainerID: c.ID, ContainerName: c.Name,
				Metric: "security_insecure_port", CurrentValue: 1, Severity: "critical", ActiveSince: now, FiredAt: now,
			}}
		}
	}
	return nil
}

type NoNewPrivilegesRule struct{}

func (r *NoNewPrivilegesRule) Name() string { return "No New Privileges" }
func (r *NoNewPrivilegesRule) Evaluate(c logger.ContainerDisplay, j types.ContainerJSON, now time.Time) []alerts.Alert {
	if j.HostConfig == nil {
		return nil
	}
	for _, opt := range j.HostConfig.SecurityOpt {
		if strings.Contains(opt, "no-new-privileges:true") || strings.Contains(opt, "no-new-privileges") {
			return nil
		}
	}
	return []alerts.Alert{{
		RuleName: "Sec: Missing No-New-Privileges", ContainerID: c.ID, ContainerName: c.Name,
		Metric: "security_no_new_privs", CurrentValue: 1, Severity: "warning", ActiveSince: now, FiredAt: now,
	}}
}

type HostNetworkingRule struct{}

func (r *HostNetworkingRule) Name() string { return "Host Networking" }
func (r *HostNetworkingRule) Evaluate(c logger.ContainerDisplay, j types.ContainerJSON, now time.Time) []alerts.Alert {
	if j.HostConfig != nil && string(j.HostConfig.NetworkMode) == "host" {
		return []alerts.Alert{{
			RuleName: "Sec: Host Networking Mode", ContainerID: c.ID, ContainerName: c.Name,
			Metric: "security_host_network", CurrentValue: 1, Severity: "critical", ActiveSince: now, FiredAt: now,
		}}
	}
	return nil
}

type SensitiveMountsRule struct{}

func (r *SensitiveMountsRule) Name() string { return "Sensitive Mounts" }
func (r *SensitiveMountsRule) Evaluate(c logger.ContainerDisplay, j types.ContainerJSON, now time.Time) []alerts.Alert {
	for _, m := range j.Mounts {
		if strings.Contains(m.Source, "docker.sock") {
			return []alerts.Alert{{
				RuleName: "Sec: Docker Socket Mounted", ContainerID: c.ID, ContainerName: c.Name,
				Metric: "security_docker_sock", CurrentValue: 1, Severity: "critical", ActiveSince: now, FiredAt: now,
			}}
		}
	}
	return nil
}