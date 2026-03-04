// Package config gerencia a configuração do Castle Rock Agent.
//
// ESTRATÉGIA DE CONFIGURAÇÃO (12-Factor App):
//
//	A ordem de precedência segue o padrão recomendado:
//	  1. Valores padrão (hardcoded no código)
//	  2. Arquivo YAML (configs/config.yaml)
//	  3. Variáveis de ambiente (override final)
//
//	Este padrão é o mais usado em aplicações cloud-native porque:
//	  - Valores padrão garantem que o app funciona sem config
//	  - YAML permite config complexa e documentada
//	  - ENV vars permitem override em runtime (Docker, K8s, CI/CD)
//
// YAML vs JSON vs TOML:
//
//	Usamos YAML porque:
//	  - É o padrão de facto em DevOps (K8s, Ansible, Docker Compose)
//	  - Suporta comentários (JSON não)
//	  - Mais legível que TOML para estruturas aninhadas
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config é a estrutura raiz de configuração do agente.
//
// TAGS YAML:
//
//	As tags `yaml:"nome"` definem como o campo é mapeado no arquivo YAML.
//	Sem a tag, o yaml.v3 usa o nome do campo em lowercase.
type Config struct {
	// LogLevel define o nível mínimo de log (debug, info, warn, error).
	LogLevel string `yaml:"log_level"`

	// Language define o idioma (en, pt).
	Language string `yaml:"language"`

	// Prometheus contém configurações do exportador de métricas.
	Prometheus PrometheusConfig `yaml:"prometheus"`

	// Stats contém configurações da coleta de métricas.
	Stats StatsConfig `yaml:"stats"`

	// Cluster contém as configurações de modo Multi-Host.
	Cluster ClusterConfig `yaml:"cluster"`

	// Alerts contém regras de alerta.
	Alerts AlertsConfig `yaml:"alerts"`

	// Prune contém configurações do Garbage Collector Automático.
	Prune PruneConfig `yaml:"prune"`
}

// PruneConfig configura as automatizações de limpeza.
type PruneConfig struct {
	Enabled            bool    `yaml:"enabled"`
	TriggerDiskPercent float64 `yaml:"trigger_disk_percent"`
}

// PrometheusConfig configura o servidor HTTP de métricas.
type PrometheusConfig struct {
	// Enabled ativa/desativa o exportador Prometheus.
	Enabled bool `yaml:"enabled"`

	// Port é a porta do servidor HTTP (/metrics).
	Port int `yaml:"port"`
}

// StatsConfig configura a coleta de métricas de containers.
type StatsConfig struct {
	// Interval é o intervalo entre coletas (ex: "5s", "10s", "1m").
	Interval time.Duration `yaml:"interval"`

	// IncludeContainers is a list of container names/substrings to monitor.
	// If empty, all containers are monitored.
	IncludeContainers []string `yaml:"include_containers"`
}

// ClusterConfig gerencia o modo distribuído (Leader/Worker).
type ClusterConfig struct {
	Mode      string `yaml:"mode"`       // "standalone", "leader", "worker"
	LeaderURL string `yaml:"leader_url"` // Endpoint HTTP para envio de métricas (modo worker)
	HostID    string `yaml:"host_id"`    // Identificador único deste nó
}

// AlertsConfig configura o sistema de alertas.
type AlertsConfig struct {
	// Enabled ativa/desativa alertas.
	Enabled bool `yaml:"enabled"`

	// Rules define as regras de alerta.
	Rules []AlertRule `yaml:"rules"`
}

// AlertRule define uma regra de alerta individual.
//
// Exemplo YAML:
//
//	rules:
//	  - name: "High CPU"
//	    metric: "cpu_percent"
//	    threshold: 80.0
//	    duration: "5m"
//	    severity: "critical"
type AlertRule struct {
	// Name é o nome descritivo do alerta.
	Name string `yaml:"name"`

	// Metric é a métrica monitorada (cpu_percent, memory_percent).
	Metric string `yaml:"metric"`

	// Operator é o operador de comparação (>, <, >=, <=, ==).
	Operator string `yaml:"operator"`

	// Threshold é o valor limite que dispara o alerta.
	Threshold float64 `yaml:"threshold"`

	// Duration é por quanto tempo a condição deve persistir
	// antes de disparar o alerta (ex: "5m", "30s").
	Duration time.Duration `yaml:"duration"`

	// Severity é a gravidade (info, warning, critical).
	Severity string `yaml:"severity"`
}

// DefaultConfig retorna a configuração padrão do agente.
//
// BOAS PRÁTICAS:
//   - Sempre forneça defaults sensatos
//   - O app deve funcionar sem arquivo de configuração
//   - Defaults devem ser seguros (porta não-privilegiada, log level info)
func DefaultConfig() Config {
	return Config{
		LogLevel: "info",
		Language: "en",
		Prometheus: PrometheusConfig{
			Enabled: true,
			Port:    9110,
		},
		Stats: StatsConfig{
			Interval:          5 * time.Second,
			IncludeContainers: []string{},
		},
		Cluster: ClusterConfig{
			Mode:      "standalone",
			LeaderURL: "http://127.0.0.1:9110/api/v1/push",
			HostID:    "", // Se vazio, usa os.Hostname() em runtime
		},
		Alerts: AlertsConfig{
			Enabled: true,
			Rules: []AlertRule{
				{
					Name:      "CPU Alta",
					Metric:    "cpu_percent",
					Operator:  ">",
					Threshold: 80.0,
					Duration:  2 * time.Minute,
					Severity:  "critical",
				},
				{
					Name:      "Memória Alta",
					Metric:    "memory_percent",
					Operator:  ">",
					Threshold: 85.0,
					Duration:  2 * time.Minute,
					Severity:  "critical",
				},
				{
					Name:      "CPU Média",
					Metric:    "cpu_percent",
					Operator:  ">",
					Threshold: 50.0,
					Duration:  5 * time.Minute,
					Severity:  "warning",
				},
			},
		},
		Prune: PruneConfig{
			Enabled:            false, // Default off to be safe
			TriggerDiskPercent: 85.0,
		},
	}
}

// Load carrega a configuração de um arquivo YAML.
// Se o arquivo não existir, retorna a configuração padrão.
//
// FLUXO:
//  1. Inicia com defaults
//  2. Se arquivo YAML existir, faz merge (override parcial)
//  3. Aplica overrides de variáveis de ambiente
//  4. Valida resultado final
func Load(path string) (Config, error) {
	cfg := DefaultConfig()

	// Tenta ler o arquivo YAML
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Arquivo não existe — usa defaults + env vars
			applyEnvOverrides(&cfg)
			return cfg, nil
		}
		return cfg, fmt.Errorf("config.Load: erro ao ler %s: %w", path, err)
	}

	// Decodifica YAML.
	// yaml.Unmarshal faz merge: campos não presentes no YAML
	// mantêm o valor default do struct.
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("config.Load: erro ao parsear YAML: %w", err)
	}

	// Aplica overrides de variáveis de ambiente (maior precedência)
	applyEnvOverrides(&cfg)

	// Se HostID não foi definido nem no config.yaml nem por env var, use o hostname
	if cfg.Cluster.HostID == "" {
		hostname, err := os.Hostname()
		if err == nil {
			cfg.Cluster.HostID = hostname
		} else {
			cfg.Cluster.HostID = "unknown_host"
		}
	}

	return cfg, nil
}

// applyEnvOverrides aplica overrides de variáveis de ambiente.
//
// CONVENÇÃO:
//
//	Variáveis de ambiente usam o prefixo CASTLE_ROCK_ seguido
//	do path do campo em UPPERCASE com _ como separador.
//
// 12-FACTOR APP (Fator III — Config):
//
//	"An app's config is everything that is likely to vary between
//	 deploys. Config varies substantially across deploys, code does not."
//
// Em Docker/K8s, variáveis de ambiente são o mecanismo padrão
// para injetar configuração runtime sem reconstruir a imagem.
func applyEnvOverrides(cfg *Config) {
	applyGeneralEnv(cfg)
	applyPrometheusEnv(cfg)
	applyStatsEnv(cfg)
	applyClusterEnv(cfg)
	applyAlertsAndPruneEnv(cfg)
}

func applyGeneralEnv(cfg *Config) {
	if v := os.Getenv("CASTLE_ROCK_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("CASTLE_ROCK_LANGUAGE"); v != "" {
		cfg.Language = v
	}
}

func applyPrometheusEnv(cfg *Config) {
	if v := os.Getenv("CASTLE_ROCK_PROMETHEUS_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Prometheus.Port = port
		}
	}
	if v := os.Getenv("CASTLE_ROCK_PROMETHEUS_ENABLED"); v != "" {
		cfg.Prometheus.Enabled = v == "true" || v == "1"
	}
}

func applyStatsEnv(cfg *Config) {
	if v := os.Getenv("CASTLE_ROCK_STATS_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Stats.Interval = d
		}
	}
	if v := os.Getenv("CASTLE_ROCK_STATS_INCLUDE_CONTAINERS"); v != "" {
		parts := strings.Split(v, ",")
		var includes []string
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				includes = append(includes, trimmed)
			}
		}
		cfg.Stats.IncludeContainers = includes
	}
}

func applyClusterEnv(cfg *Config) {
	if v := os.Getenv("CASTLE_ROCK_CLUSTER_MODE"); v != "" {
		cfg.Cluster.Mode = v
	}
	if v := os.Getenv("CASTLE_ROCK_CLUSTER_LEADER_URL"); v != "" {
		cfg.Cluster.LeaderURL = v
	}
	if v := os.Getenv("CASTLE_ROCK_CLUSTER_HOST_ID"); v != "" {
		cfg.Cluster.HostID = v
	}
}

func applyAlertsAndPruneEnv(cfg *Config) {
	if v := os.Getenv("CASTLE_ROCK_ALERTS_ENABLED"); v != "" {
		cfg.Alerts.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("CASTLE_ROCK_PRUNE_ENABLED"); v != "" {
		cfg.Prune.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("CASTLE_ROCK_PRUNE_DISK_TRIGGER"); v != "" {
		if th, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Prune.TriggerDiskPercent = th
		}
	}
}
