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

// Auditor analyzes running containers looking for risky configurations
// or security anti-patterns.
type Auditor struct {
	dockerClient *docker.Client

	// Cache to avoid running 'docker inspect' every second.
	// Key: ContainerID
	cache map[string][]alerts.Alert
	mu    sync.RWMutex
}

// NewAuditor creates a new auditor integrated with the Docker client.
func NewAuditor(client *docker.Client) *Auditor {
	return &Auditor{
		dockerClient: client,
		cache:        make(map[string][]alerts.Alert),
	}
}

// Audit scans the provided container list.
// Uses a cache keyed by ContainerID (containers are immutable so
// the configuration won't change unless the container is recreated, changing the ID).
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

				secAlerts := a.evaluateSecurityRules(c, inspectJSON, now)

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

// evaluateSecurityRules evaluates all predictive security rules on a container.
func (a *Auditor) evaluateSecurityRules(c logger.ContainerDisplay, inspectJSON types.ContainerJSON, now time.Time) []alerts.Alert {
	var secAlerts []alerts.Alert
	secAlerts = a.checkPrivilegedMode(secAlerts, c, inspectJSON, now)
	secAlerts = a.checkRootUser(secAlerts, c, inspectJSON, now)
	secAlerts = a.checkDBPortExposed(secAlerts, c, inspectJSON, now)
	secAlerts = a.checkSensitiveCaps(secAlerts, c, inspectJSON, now)
	secAlerts = a.checkResourceQuotas(secAlerts, c, inspectJSON, now)
	secAlerts = a.checkWritableRootFS(secAlerts, c, inspectJSON, now)
	secAlerts = a.checkInsecurePorts(secAlerts, c, inspectJSON, now)
	secAlerts = a.checkNoNewPrivileges(secAlerts, c, inspectJSON, now)
	secAlerts = a.checkHostNetworking(secAlerts, c, inspectJSON, now)
	return secAlerts
}

func (a *Auditor) checkPrivilegedMode(result []alerts.Alert, c logger.ContainerDisplay, j types.ContainerJSON, now time.Time) []alerts.Alert {
	if j.HostConfig != nil && j.HostConfig.Privileged {
		return append(result, alerts.Alert{
			RuleName: "Sec: Privileged Mode", ContainerID: c.ID, ContainerName: c.Name,
			Metric: "security_privileged", CurrentValue: 1, Severity: "critical", ActiveSince: now, FiredAt: now,
		})
	}
	return result
}

func (a *Auditor) checkRootUser(result []alerts.Alert, c logger.ContainerDisplay, j types.ContainerJSON, now time.Time) []alerts.Alert {
	user := ""
	if j.Config != nil {
		user = j.Config.User
	}
	if user == "" || user == "root" || user == "0" {
		return append(result, alerts.Alert{
			RuleName: "Sec: Root User", ContainerID: c.ID, ContainerName: c.Name,
			Metric: "security_root_user", CurrentValue: 1, Severity: "warning", ActiveSince: now, FiredAt: now,
		})
	}
	return result
}

func isDBPort(portStr string) bool {
	return strings.HasPrefix(portStr, "3306/") ||
		strings.HasPrefix(portStr, "5432/") ||
		strings.HasPrefix(portStr, "27017/") ||
		strings.HasPrefix(portStr, "6379/")
}

func isGloballyExposed(ip string) bool {
	return ip == "0.0.0.0" || ip == "" || ip == "::"
}

func (a *Auditor) checkDBPortExposed(result []alerts.Alert, c logger.ContainerDisplay, j types.ContainerJSON, now time.Time) []alerts.Alert {
	if j.NetworkSettings == nil {
		return result
	}
	for port, bindings := range j.NetworkSettings.Ports {
		if !isDBPort(string(port)) {
			continue
		}
		for _, b := range bindings {
			if isGloballyExposed(b.HostIP) {
				return append(result, alerts.Alert{
					RuleName: "Sec: DB Port Exposed globally", ContainerID: c.ID, ContainerName: c.Name,
					Metric: "security_db_port", CurrentValue: float64(port.Int()), Severity: "critical", ActiveSince: now, FiredAt: now,
				})
			}
		}
	}
	return result
}

func (a *Auditor) checkSensitiveCaps(result []alerts.Alert, c logger.ContainerDisplay, j types.ContainerJSON, now time.Time) []alerts.Alert {
	if j.HostConfig == nil {
		return result
	}
	for _, cap := range j.HostConfig.CapAdd {
		if cap == "SYS_ADMIN" || cap == "NET_ADMIN" ||
			cap == "SYS_PTRACE" || cap == "DAC_OVERRIDE" ||
			cap == "SYS_RAWIO" || cap == "SYS_MODULE" {
			return append(result, alerts.Alert{
				RuleName: "Sec: Sensitive CAP_ADD", ContainerID: c.ID, ContainerName: c.Name,
				Metric: "security_sensitive_cap", CurrentValue: 1, Severity: "warning", ActiveSince: now, FiredAt: now,
			})
		}
	}
	return result
}

func (a *Auditor) checkResourceQuotas(result []alerts.Alert, c logger.ContainerDisplay, j types.ContainerJSON, now time.Time) []alerts.Alert {
	if j.HostConfig != nil && (j.HostConfig.Memory == 0 || j.HostConfig.NanoCPUs == 0) {
		return append(result, alerts.Alert{
			RuleName: "Sec: No Resource Quotas", ContainerID: c.ID, ContainerName: c.Name,
			Metric: "security_no_quotas", CurrentValue: 1, Severity: "warning", ActiveSince: now, FiredAt: now,
		})
	}
	return result
}

func (a *Auditor) checkWritableRootFS(result []alerts.Alert, c logger.ContainerDisplay, j types.ContainerJSON, now time.Time) []alerts.Alert {
	if j.HostConfig != nil && !j.HostConfig.ReadonlyRootfs {
		return append(result, alerts.Alert{
			RuleName: "Sec: Writable RootFS", ContainerID: c.ID, ContainerName: c.Name,
			Metric: "security_writable_rootfs", CurrentValue: 1, Severity: "warning", ActiveSince: now, FiredAt: now,
		})
	}
	return result
}

func (a *Auditor) checkInsecurePorts(result []alerts.Alert, c logger.ContainerDisplay, j types.ContainerJSON, now time.Time) []alerts.Alert {
	if j.NetworkSettings == nil {
		return result
	}
	for port := range j.NetworkSettings.Ports {
		portStr := string(port)
		if strings.HasPrefix(portStr, "22/") || strings.HasPrefix(portStr, "23/") {
			return append(result, alerts.Alert{
				RuleName: "Sec: Insecure Port Exposed", ContainerID: c.ID, ContainerName: c.Name,
				Metric: "security_insecure_port", CurrentValue: 1, Severity: "critical", ActiveSince: now, FiredAt: now,
			})
		}
	}
	return result
}

func (a *Auditor) checkNoNewPrivileges(result []alerts.Alert, c logger.ContainerDisplay, j types.ContainerJSON, now time.Time) []alerts.Alert {
	if j.HostConfig == nil {
		return result
	}
	for _, opt := range j.HostConfig.SecurityOpt {
		if strings.Contains(opt, "no-new-privileges:true") || strings.Contains(opt, "no-new-privileges") {
			return result
		}
	}
	return append(result, alerts.Alert{
		RuleName: "Sec: Missing No-New-Privileges", ContainerID: c.ID, ContainerName: c.Name,
		Metric: "security_no_new_privs", CurrentValue: 1, Severity: "warning", ActiveSince: now, FiredAt: now,
	})
}

func (a *Auditor) checkHostNetworking(result []alerts.Alert, c logger.ContainerDisplay, j types.ContainerJSON, now time.Time) []alerts.Alert {
	if j.HostConfig != nil && string(j.HostConfig.NetworkMode) == "host" {
		return append(result, alerts.Alert{
			RuleName: "Sec: Host Networking Mode", ContainerID: c.ID, ContainerName: c.Name,
			Metric: "security_host_network", CurrentValue: 1, Severity: "critical", ActiveSince: now, FiredAt: now,
		})
	}
	return result
}
