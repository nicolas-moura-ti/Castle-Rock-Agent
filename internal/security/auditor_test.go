package security

import (
	"testing"
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
		{"Redis TCP", "6379/tcp", true},

		// Edge cases on DB ports
		{"MySQL without protocol", "3306", false}, // Function checks for "3306/"
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
