package i18n

import (
	"testing"
)

func TestGet(t *testing.T) {
	tests := []struct {
		name     string
		lang     string
		expected Messages
	}{
		{
			name:     "Portuguese (pt)",
			lang:     "pt",
			expected: Pt,
		},
		{
			name:     "Portuguese Brazil (pt-BR)",
			lang:     "pt-BR",
			expected: Pt,
		},
		{
			name:     "English (en)",
			lang:     "en",
			expected: En,
		},
		{
			name:     "Default to English for unknown language",
			lang:     "fr",
			expected: En,
		},
		{
			name:     "Default to English for empty string",
			lang:     "",
			expected: En,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Get(tt.lang)
			if result != tt.expected {
				t.Errorf("Get(%q) = %v, want %v", tt.lang, result, tt.expected)
			}
		})
	}
}
