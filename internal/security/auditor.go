// Package security implementa auditoria de segurança preditiva em containers.
package security

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/nicolas-moura-ti/castle-rock-agent/internal/alerts"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/docker"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/logger"
)

// Auditor analisa containers vivos em busca de configurações
// arriscadas ou anti-patterns de segurança.
type Auditor struct {
	dockerClient *docker.Client

	// Cache para não rodar um 'docker inspect' a cada segundo
	// Chave: ContainerID
	cache map[string][]alerts.Alert
	mu    sync.RWMutex
}

// NewAuditor cria um novo auditor integrado ao cliente do Docker.
func NewAuditor(client *docker.Client) *Auditor {
	return &Auditor{
		dockerClient: client,
		cache:        make(map[string][]alerts.Alert),
	}
}

// Audit varre a lista de containers fornecida.
// Utiliza um cache por ContainerID (como containers são imutáveis,
// a configuração não muda a menos que o container seja recriado, o que muda o ID).
func (a *Auditor) Audit(ctx context.Context, containers []logger.ContainerDisplay) []alerts.Alert {
	a.mu.Lock()
	defer a.mu.Unlock()

	var activeAlerts []alerts.Alert
	activeIDs := make(map[string]bool)
	now := time.Now()

	for _, c := range containers {
		activeIDs[c.ID] = true

		if cached, exists := a.cache[c.ID]; exists {
			// Atualiza os tempos para evitar que pareçam obsoletos
			for i := range cached {
				cached[i].FiredAt = now
				activeAlerts = append(activeAlerts, cached[i])
			}
			continue
		}

		// Cache miss — realiza o inspect
		inspectJSON, err := a.dockerClient.InspectContainer(ctx, c.ID)
		if err != nil {
			continue // container pode ter morrido antes do inspect
		}

		var secAlerts []alerts.Alert

		// 1. Privileged = true (Crítico)
		// Isso permite ao container fazer praticamente tudo no host.
		if inspectJSON.HostConfig != nil && inspectJSON.HostConfig.Privileged {
			secAlerts = append(secAlerts, alerts.Alert{
				RuleName:      "Sec: Privileged Mode",
				ContainerID:   c.ID,
				ContainerName: c.Name,
				Metric:        "security_privileged",
				CurrentValue:  1,
				Threshold:     0,
				Severity:      "critical",
				ActiveSince:   now,
				FiredAt:       now,
			})
		}

		// 2. Root user
		// Prática ruim rodar processos internos do container como root
		user := ""
		if inspectJSON.Config != nil {
			user = inspectJSON.Config.User
		}
		if user == "" || user == "root" || user == "0" {
			secAlerts = append(secAlerts, alerts.Alert{
				RuleName:      "Sec: Root User",
				ContainerID:   c.ID,
				ContainerName: c.Name,
				Metric:        "security_root_user",
				CurrentValue:  1,
				Threshold:     0,
				Severity:      "warning",
				ActiveSince:   now,
				FiredAt:       now,
			})
		}

		// 3. PortBindings de DBs abertos pro mundo (0.0.0.0)
		if inspectJSON.NetworkSettings != nil {
			for port, bindings := range inspectJSON.NetworkSettings.Ports {
				portStr := string(port)
				// Portas comuns de SGDB
				isDBPort := strings.HasPrefix(portStr, "3306/") || // MySQL/MariaDB
					strings.HasPrefix(portStr, "5432/") || // PostgreSQL
					strings.HasPrefix(portStr, "27017/") || // MongoDB
					strings.HasPrefix(portStr, "6379/") // Redis

				if isDBPort {
					for _, b := range bindings {
						if b.HostIP == "0.0.0.0" || b.HostIP == "" || b.HostIP == "::" {
							secAlerts = append(secAlerts, alerts.Alert{
								RuleName:      "Sec: DB Port Exposed globally",
								ContainerID:   c.ID,
								ContainerName: c.Name,
								Metric:        "security_db_port",
								CurrentValue:  float64(port.Int()),
								Threshold:     0,
								Severity:      "critical",
								ActiveSince:   now,
								FiredAt:       now,
							})
							break
						}
					}
				}
			}
		}

		// 4. Capabilities sensíveis (SYS_ADMIN)
		if inspectJSON.HostConfig != nil {
			for _, cap := range inspectJSON.HostConfig.CapAdd {
				if cap == "SYS_ADMIN" || cap == "NET_ADMIN" {
					secAlerts = append(secAlerts, alerts.Alert{
						RuleName:      "Sec: Sensitive CAP_ADD",
						ContainerID:   c.ID,
						ContainerName: c.Name,
						Metric:        "security_sensitive_cap",
						CurrentValue:  1,
						Threshold:     0,
						Severity:      "warning",
						ActiveSince:   now,
						FiredAt:       now,
					})
					break // Um alerta de cap já basta
				}
			}
		}

		a.cache[c.ID] = secAlerts
		activeAlerts = append(activeAlerts, secAlerts...)
	}

	// Limpeza do cache para containers que morreram
	for id := range a.cache {
		if !activeIDs[id] {
			delete(a.cache, id)
		}
	}

	return activeAlerts
}
