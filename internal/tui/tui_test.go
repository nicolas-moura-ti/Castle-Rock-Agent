// Package tui — testes unitários para funções de formatação.
//
// TESTES EM GO:
//   - Arquivos de teste terminam com _test.go
//   - Funções de teste começam com Test e recebem *testing.T
//   - Executar: go test ./internal/tui/ -v
//   - Cobertura: go test ./internal/tui/ -cover
package tui

import "testing"

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name     string
		input    uint64
		expected string
	}{
		{"zero", 0, "0B"},
		{"bytes", 512, "512B"},
		{"kilobytes", 1024, "1.0KB"},
		{"megabytes", 1024 * 1024, "1.0MB"},
		{"megabytes_decimal", 150 * 1024 * 1024, "150.0MB"},
		{"gigabytes", 1024 * 1024 * 1024, "1.0GB"},
		{"gigabytes_decimal", 8272408576, "7.7GB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatBytes(tt.input)
			if result != tt.expected {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatBytesShort(t *testing.T) {
	tests := []struct {
		name     string
		input    uint64
		expected string
	}{
		{"zero", 0, "0B"},
		{"bytes", 500, "500B"},
		{"kilobytes", 2048, "2K"},
		{"megabytes", 150 * 1024 * 1024, "150M"},
		{"gigabytes", 2 * 1024 * 1024 * 1024, "2G"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatBytesShort(tt.input)
			if result != tt.expected {
				t.Errorf("formatBytesShort(%d) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is a long string", 10, "this is..."},
		{"ab", 5, "ab"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := truncate(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
			}
		})
	}
}

func TestMin(t *testing.T) {
	if min(3, 5) != 3 {
		t.Error("min(3, 5) should be 3")
	}
	if min(10, 2) != 2 {
		t.Error("min(10, 2) should be 2")
	}
	if min(4, 4) != 4 {
		t.Error("min(4, 4) should be 4")
	}
}
