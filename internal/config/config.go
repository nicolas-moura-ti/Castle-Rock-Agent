// Package config manages the Castle Rock Agent configuration.
//
// CONFIGURATION STRATEGY (12-Factor App):
//
//	The precedence order follows the recommended pattern:
//	  1. Default values (hardcoded in code)
//	  2. YAML file (configs/config.yaml)
//	  3. Environment variables (final override)
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	yaml "gopkg.in/yaml.v3"
)

// Config is the root configuration structure for the agent.
type Config struct {
	LogLevel   string           `yaml:"log_level"`
	Language   string           `yaml:"language"`
	Prometheus PrometheusConfig `yaml:"prometheus"`
	Stats      StatsConfig      `yaml:"stats"`
	Cluster    ClusterConfig    `yaml:"cluster"`
	Alerts     AlertsConfig     `yaml:"alerts"`
	Prune      PruneConfig      `yaml:"prune"`
}

// PruneConfig configures cleanup automations.
type PruneConfig struct {
	Enabled            bool    `yaml:"enabled"`
	TriggerDiskPercent float64 `yaml:"trigger_disk_percent"`
}

// PrometheusConfig configures the HTTP metrics server.
type PrometheusConfig struct {
	Enabled bool `yaml:"enabled"`
	Port    int  `yaml:"port"`
}

// StatsConfig configures container metrics collection.
type StatsConfig struct {
	Interval          time.Duration `yaml:"interval"`
	IncludeContainers []string      `yaml:"include_containers"`
}

// ClusterConfig manages the distributed mode (Leader/Worker).
type ClusterConfig struct {
	Mode         string `yaml:"mode"`          // "standalone", "leader", "worker"
	LeaderURL    string `yaml:"leader_url"`    // HTTP endpoint for metrics submission
	HostID       string `yaml:"host_id"`       // Unique identifier for this node
	SharedSecret string `yaml:"shared_secret"` // Secret key for AES payload encryption
	AuthToken    string `yaml:"auth_token"`    // Bearer token for API authentication
}

// AlertsConfig configures the alert system.
type AlertsConfig struct {
	Enabled bool        `yaml:"enabled"`
	Rules   []AlertRule `yaml:"rules"`
}

// AlertRule defines an individual alert rule.
type AlertRule struct {
	Name      string        `yaml:"name"`
	Metric    string        `yaml:"metric"`
	Operator  string        `yaml:"operator"`
	Threshold float64       `yaml:"threshold"`
	Duration  time.Duration `yaml:"duration"`
	Severity  string        `yaml:"severity"`
}

// DefaultConfig returns the default agent configuration.
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
			Mode:         "standalone",
			LeaderURL:    "http://127.0.0.1:9110/api/v1/push",
			HostID:       "", 
			SharedSecret: "", // Resolvido: Alterado de Token para SharedSecret
			AuthToken:    "", // Resolvido: Token de autenticação separado da chave AES
		},
		Alerts: AlertsConfig{
			Enabled: true,
			Rules: []AlertRule{
				{
					Name:      "High CPU",
					Metric:    "cpu_percent",
					Operator:  ">",
					Threshold: 80.0,
					Duration:  2 * time.Minute,
					Severity:  "critical",
				},
				{
					Name:      "High Memory",
					Metric:    "memory_percent",
					Operator:  ">",
					Threshold: 85.0,
					Duration:  2 * time.Minute,
					Severity:  "critical",
				},
			},
		},
		Prune: PruneConfig{
			Enabled:            false,
			TriggerDiskPercent: 85.0,
		},
	}
}

// Load loads the configuration from a YAML file.
func Load(path string) (Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			applyEnvOverrides(&cfg)
			return cfg, nil
		}
		return cfg, fmt.Errorf("config.Load: error reading %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("config.Load: error parsing YAML: %w", err)
	}

	applyEnvOverrides(&cfg)

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
	// Resolvido: Mapeamento da variável de ambiente para o SharedSecret
	if v := os.Getenv("CASTLE_ROCK_CLUSTER_SHARED_SECRET"); v != "" {
		cfg.Cluster.SharedSecret = v
	}
	// Resolvido: Variável de ambiente isolada para o AuthToken
	if v := os.Getenv("CASTLE_ROCK_CLUSTER_AUTH_TOKEN"); v != "" {
		cfg.Cluster.AuthToken = v
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