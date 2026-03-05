// Package config — unit tests for the configuration loader.
package config

import (
	"os"
	"testing"
	"time"
)

// TestDefaultConfig verifies that default values are sensible.
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.LogLevel != "info" {
		t.Errorf("Default LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.Prometheus.Port != 9110 {
		t.Errorf("Default Prometheus.Port = %d, want %d", cfg.Prometheus.Port, 9110)
	}
	if !cfg.Prometheus.Enabled {
		t.Error("Default Prometheus.Enabled should be true")
	}
	if cfg.Stats.Interval != 5*time.Second {
		t.Errorf("Default Stats.Interval = %v, want %v", cfg.Stats.Interval, 5*time.Second)
	}
	if !cfg.Alerts.Enabled {
		t.Error("Default Alerts.Enabled should be true")
	}
	if len(cfg.Alerts.Rules) == 0 {
		t.Error("Default config should have alert rules")
	}
}

// TestLoadNonExistentFile verifies that a nonexistent file
// returns the default configuration without error.
func TestLoadNonExistentFile(t *testing.T) {
	cfg, err := Load("/tmp/nonexistent-castle-rock-test.yaml")
	if err != nil {
		t.Fatalf("Load should not error for nonexistent file: %v", err)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("Expected default log_level, got %q", cfg.LogLevel)
	}
}

// TestLoadValidYAML verifies parsing of a valid YAML file.
func TestLoadValidYAML(t *testing.T) {
	content := `
log_level: "debug"
prometheus:
  enabled: false
  port: 8080
stats:
  interval: 10s
alerts:
  enabled: false
`
	tmpFile, err := os.CreateTemp("", "castle-rock-test-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	cfg, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
	if cfg.Prometheus.Enabled {
		t.Error("Prometheus should be disabled")
	}
	if cfg.Prometheus.Port != 8080 {
		t.Errorf("Prometheus.Port = %d, want %d", cfg.Prometheus.Port, 8080)
	}
	if cfg.Stats.Interval != 10*time.Second {
		t.Errorf("Stats.Interval = %v, want %v", cfg.Stats.Interval, 10*time.Second)
	}
	if cfg.Alerts.Enabled {
		t.Error("Alerts should be disabled")
	}
}

// TestEnvOverrides verifies that environment variables take precedence.
func TestEnvOverrides(t *testing.T) {
	// Set env vars
	os.Setenv("CASTLE_ROCK_LOG_LEVEL", "error")
	os.Setenv("CASTLE_ROCK_PROMETHEUS_PORT", "3000")
	defer os.Unsetenv("CASTLE_ROCK_LOG_LEVEL")
	defer os.Unsetenv("CASTLE_ROCK_PROMETHEUS_PORT")

	cfg, err := Load("/tmp/nonexistent.yaml")
	if err != nil {
		t.Fatal(err)
	}

	if cfg.LogLevel != "error" {
		t.Errorf("LogLevel = %q, want %q (from env)", cfg.LogLevel, "error")
	}
	if cfg.Prometheus.Port != 3000 {
		t.Errorf("Prometheus.Port = %d, want %d (from env)", cfg.Prometheus.Port, 3000)
	}
}
