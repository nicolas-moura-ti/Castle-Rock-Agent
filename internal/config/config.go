// Package config manages the Castle Rock Agent configuration.
//
// CONFIGURATION STRATEGY (12-Factor App):
//
//	The precedence order follows the recommended pattern:
//	  1. Default values (hardcoded in code)
//	  2. YAML file (configs/config.yaml)
//	  3. Environment variables (final override)
//
//	This pattern is most used in cloud-native applications because:
//	  - Default values ensure the app works without config
//	  - YAML allows complex and documented config
//	  - ENV vars allow runtime override (Docker, K8s, CI/CD)
//
// YAML vs JSON vs TOML:
//
//	We use YAML because:
//	  - It is the de facto standard in DevOps (K8s, Ansible, Docker Compose)
//	  - Supports comments (JSON does not)
//	  - More readable than TOML for nested structures
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration structure for the agent.
//
// YAML TAGS:
//
//	The `yaml:"name"` tags define how the field is mapped in the YAML file.
//	Without the tag, yaml.v3 uses the field name in lowercase.
type Config struct {
	// LogLevel defines the minimum log level (debug, info, warn, error).
	LogLevel string `yaml:"log_level"`

	// Language defines the UI language (en, pt).
	Language string `yaml:"language"`

	// Prometheus contains metrics exporter settings.
	Prometheus PrometheusConfig `yaml:"prometheus"`

	// Stats contains metrics collection settings.
	Stats StatsConfig `yaml:"stats"`

	// Cluster contains Multi-Host mode settings.
	Cluster ClusterConfig `yaml:"cluster"`

	// Alerts contains alert rule settings.
	Alerts AlertsConfig `yaml:"alerts"`

	// Prune contains the Automatic Garbage Collector settings.
	Prune PruneConfig `yaml:"prune"`
}

// PruneConfig configures cleanup automations.
type PruneConfig struct {
	Enabled            bool    `yaml:"enabled"`
	TriggerDiskPercent float64 `yaml:"trigger_disk_percent"`
}

// PrometheusConfig configures the HTTP metrics server.
type PrometheusConfig struct {
	// Enabled activates/deactivates the Prometheus exporter.
	Enabled bool `yaml:"enabled"`

	// Port is the HTTP server port (/metrics).
	Port int `yaml:"port"`
}

// StatsConfig configures container metrics collection.
type StatsConfig struct {
	// Interval is the time between collections (e.g. "5s", "10s", "1m").
	Interval time.Duration `yaml:"interval"`

	// IncludeContainers is a list of container names/substrings to monitor.
	// If empty, all containers are monitored.
	IncludeContainers []string `yaml:"include_containers"`
}

// ClusterConfig manages the distributed mode (Leader/Worker).
type ClusterConfig struct {
	Mode      string `yaml:"mode"`       // "standalone", "leader", "worker"
	LeaderURL string `yaml:"leader_url"` // HTTP endpoint for metrics submission (worker mode)
	HostID    string `yaml:"host_id"`    // Unique identifier for this node
	Token     string `yaml:"token"`      // Authentication token for cluster communication
}

// AlertsConfig configures the alert system.
type AlertsConfig struct {
	// Enabled activates/deactivates alerts.
	Enabled bool `yaml:"enabled"`

	// Rules defines the alert rules.
	Rules []AlertRule `yaml:"rules"`
}

// AlertRule defines an individual alert rule.
//
// YAML example:
//
//	rules:
//	  - name: "High CPU"
//	    metric: "cpu_percent"
//	    threshold: 80.0
//	    duration: "5m"
//	    severity: "critical"
type AlertRule struct {
	// Name is the descriptive name for the alert.
	Name string `yaml:"name"`

	// Metric is the monitored metric (cpu_percent, memory_percent).
	Metric string `yaml:"metric"`

	// Operator is the comparison operator (>, <, >=, <=, ==).
	Operator string `yaml:"operator"`

	// Threshold is the limit value that triggers the alert.
	Threshold float64 `yaml:"threshold"`

	// Duration is how long the condition must persist
	// before triggering the alert (e.g. "5m", "30s").
	Duration time.Duration `yaml:"duration"`

	// Severity is the gravity level (info, warning, critical).
	Severity string `yaml:"severity"`
}

// DefaultConfig returns the default agent configuration.
//
// BEST PRACTICES:
//   - Always provide sensible defaults
//   - The app should work without a config file
//   - Defaults should be safe (non-privileged port, info log level)
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
			HostID:    "", // If empty, uses os.Hostname() at runtime
			Token:     "", // Empty by default (requires configuration for secure use)
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
				{
					Name:      "Medium CPU",
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

// Load loads the configuration from a YAML file.
// If the file does not exist, returns the default configuration.
//
// FLOW:
//  1. Start with defaults
//  2. If YAML file exists, merge (partial override)
//  3. Apply environment variable overrides
//  4. Validate final result
func Load(path string) (Config, error) {
	cfg := DefaultConfig()

	// Try to read the YAML file
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File does not exist — use defaults + env vars
			applyEnvOverrides(&cfg)
			return cfg, nil
		}
		return cfg, fmt.Errorf("config.Load: error reading %s: %w", path, err)
	}

	// Decode YAML.
	// yaml.Unmarshal performs a merge: fields not present in the YAML
	// keep their default struct value.
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("config.Load: error parsing YAML: %w", err)
	}

	// Apply environment variable overrides (highest precedence)
	applyEnvOverrides(&cfg)

	// If HostID was not defined in config.yaml or env var, use hostname
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

// applyEnvOverrides applies environment variable overrides.
//
// CONVENTION:
//
//	Environment variables use the CASTLE_ROCK_ prefix followed
//	by the field path in UPPERCASE with _ as separator.
//
// 12-FACTOR APP (Factor III — Config):
//
//	"An app's config is everything that is likely to vary between
//	 deploys. Config varies substantially across deploys, code does not."
//
// In Docker/K8s, environment variables are the standard mechanism
// for injecting runtime configuration without rebuilding the image.
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
	if v := os.Getenv("CASTLE_ROCK_CLUSTER_TOKEN"); v != "" {
		cfg.Cluster.Token = v
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
