// Package config — unit tests for the configuration loader.
package config

import (
	"os"
	"path/filepath"
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

// TestLoad verifies the configuration loader behavior under various conditions.
func TestLoad(t *testing.T) {
	tests := []struct {
		name       string
		fileBody   string
		envVars    map[string]string
		wantErr    bool
		wantAssert func(*testing.T, Config)
	}{
		{
			name:     "nonexistent file returns default config",
			fileBody: "", // indicates no file
			wantAssert: func(t *testing.T, cfg Config) {
				if cfg.LogLevel != "info" {
					t.Errorf("Expected default log_level, got %q", cfg.LogLevel)
				}
			},
		},
		{
			name: "valid YAML file",
			fileBody: `
log_level: "debug"
prometheus:
  enabled: false
  port: 8080
stats:
  interval: 10s
cluster:
  shared_secret: "my-secret-yaml-token"
  auth_token: "my-auth-yaml-token"
alerts:
  enabled: false
`,
			wantAssert: func(t *testing.T, cfg Config) {
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
				if cfg.Cluster.SharedSecret != "my-secret-yaml-token" {
					t.Errorf("Cluster.SharedSecret = %q, want %q", cfg.Cluster.SharedSecret, "my-secret-yaml-token")
				}
				if cfg.Cluster.AuthToken != "my-auth-yaml-token" {
					t.Errorf("Cluster.AuthToken = %q, want %q", cfg.Cluster.AuthToken, "my-auth-yaml-token")
				}
			},
		},
		{
			name: "environment variables take precedence",
			envVars: map[string]string{
				"CASTLE_ROCK_LOG_LEVEL":             "error",
				"CASTLE_ROCK_PROMETHEUS_PORT":       "3000",
				"CASTLE_ROCK_CLUSTER_SHARED_SECRET": "env-secret-token",
				"CASTLE_ROCK_CLUSTER_AUTH_TOKEN":    "env-auth-token",
			},
			wantAssert: func(t *testing.T, cfg Config) {
				if cfg.LogLevel != "error" {
					t.Errorf("LogLevel = %q, want %q (from env)", cfg.LogLevel, "error")
				}
				if cfg.Prometheus.Port != 3000 {
					t.Errorf("Prometheus.Port = %d, want %d (from env)", cfg.Prometheus.Port, 3000)
				}
				if cfg.Cluster.SharedSecret != "env-secret-token" {
					t.Errorf("Cluster.SharedSecret = %q, want %q (from env)", cfg.Cluster.SharedSecret, "env-secret-token")
				}
				if cfg.Cluster.AuthToken != "env-auth-token" {
					t.Errorf("Cluster.AuthToken = %q, want %q (from env)", cfg.Cluster.AuthToken, "env-auth-token")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			path := filepath.Join(t.TempDir(), "nonexistent.yaml")
			if tt.fileBody != "" {
				path = filepath.Join(t.TempDir(), "config.yaml")
				if err := os.WriteFile(path, []byte(tt.fileBody), 0644); err != nil {
					t.Fatal(err)
				}
			}

			cfg, err := Load(path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Load() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && tt.wantAssert != nil {
				tt.wantAssert(t, cfg)
			}
		})
	}
}
