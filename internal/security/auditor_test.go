package security

import (
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/go-connections/nat"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/logger"
)

func TestIsDBPort(t *testing.T) {
	tests := []struct {
		name    string
		portStr string
		want    bool
	}{
		// Valid DB ports
		{"MySQL TCP", "3306/tcp", true},
		{"MySQL UDP", "3306/udp", true},
		{"PostgreSQL TCP", "5432/tcp", true},
		{"PostgreSQL UDP", "5432/udp", true},
		{"MongoDB TCP", "27017/tcp", true},
		{"MongoDB UDP", "27017/udp", true},
		{"Redis TCP", "6379/tcp", true},
		{"Redis UDP", "6379/udp", true},

		// Edge cases on DB ports
		{"MySQL without protocol", "3306", false},   // Function checks for "3306/"
		{"MySQL partial match", "33060/tcp", false}, // Function checks for "3306/"

		// Non-DB ports
		{"HTTP", "80/tcp", false},
		{"HTTPS", "443/tcp", false},
		{"SSH", "22/tcp", false},

		// Invalid formats
		{"Empty string", "", false},
		{"Random string", "random", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDBPort(tt.portStr); got != tt.want {
				t.Errorf("isDBPort() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsGloballyExposed(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		// Globally exposed IPs
		{"IPv4 all interfaces", "0.0.0.0", true},
		{"Empty string (all interfaces)", "", true},
		{"IPv6 all interfaces", "::", true},

		// Non-globally exposed IPs
		{"Localhost IPv4", "127.0.0.1", false},
		{"Localhost IPv6", "::1", false},
		{"Private network (10.x)", "10.0.0.1", false},
		{"Private network (172.16.x)", "172.16.0.1", false},
		{"Private network (192.168.x)", "192.168.1.1", false},

		// Edge cases
		{"Invalid IP", "invalid-ip", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isGloballyExposed(tt.ip); got != tt.want {
				t.Errorf("isGloballyExposed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDBPortExposedRule_Evaluate(t *testing.T) {
	rule := &DBPortExposedRule{}
	now := time.Now()

	container := logger.ContainerDisplay{
		ID:   "test-id",
		Name: "test-db",
	}

	tests := []struct {
		name     string
		networks *types.NetworkSettings
		want     int // Number of expected alerts
	}{
		{
			name:     "NetworkSettings is nil",
			networks: nil,
			want:     0,
		},
		{
			name: "Valid DB port globally exposed",
			networks: &types.NetworkSettings{
				NetworkSettingsBase: types.NetworkSettingsBase{
					Ports: nat.PortMap{
						"3306/tcp": []nat.PortBinding{
							{HostIP: "0.0.0.0", HostPort: "3306"},
						},
					},
				},
			},
			want: 1,
		},
		{
			name: "Valid DB port globally exposed (empty HostIP)",
			networks: &types.NetworkSettings{
				NetworkSettingsBase: types.NetworkSettingsBase{
					Ports: nat.PortMap{
						"5432/tcp": []nat.PortBinding{
							{HostIP: "", HostPort: "5432"},
						},
					},
				},
			},
			want: 1,
		},
		{
			name: "Valid DB port globally exposed (IPv6)",
			networks: &types.NetworkSettings{
				NetworkSettingsBase: types.NetworkSettingsBase{
					Ports: nat.PortMap{
						"27017/tcp": []nat.PortBinding{
							{HostIP: "::", HostPort: "27017"},
						},
					},
				},
			},
			want: 1,
		},
		{
			name: "Valid DB port not globally exposed",
			networks: &types.NetworkSettings{
				NetworkSettingsBase: types.NetworkSettingsBase{
					Ports: nat.PortMap{
						"3306/tcp": []nat.PortBinding{
							{HostIP: "127.0.0.1", HostPort: "3306"},
						},
					},
				},
			},
			want: 0,
		},
		{
			name: "Non-DB port globally exposed",
			networks: &types.NetworkSettings{
				NetworkSettingsBase: types.NetworkSettingsBase{
					Ports: nat.PortMap{
						"80/tcp": []nat.PortBinding{
							{HostIP: "0.0.0.0", HostPort: "80"},
						},
					},
				},
			},
			want: 0,
		},
		{
			name: "Multiple valid DB ports globally exposed",
			networks: &types.NetworkSettings{
				NetworkSettingsBase: types.NetworkSettingsBase{
					Ports: nat.PortMap{
						"3306/tcp": []nat.PortBinding{
							{HostIP: "0.0.0.0", HostPort: "3306"},
						},
						"5432/tcp": []nat.PortBinding{
							{HostIP: "0.0.0.0", HostPort: "5432"},
						},
					},
				},
			},
			want: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := types.ContainerJSON{
				NetworkSettings: tt.networks,
			}
			got := rule.Evaluate(container, j, now)
			if len(got) != tt.want {
				t.Errorf("DBPortExposedRule.Evaluate() returned %d alerts, want %d", len(got), tt.want)
			}

			// If we expect alerts, do some basic validation
			if tt.want > 0 {
				for _, alert := range got {
					if alert.Severity != "critical" {
						t.Errorf("Expected severity 'critical', got %s", alert.Severity)
					}
					if alert.ContainerID != container.ID {
						t.Errorf("Expected ContainerID %s, got %s", container.ID, alert.ContainerID)
					}
				}
			}
		})
	}
}
