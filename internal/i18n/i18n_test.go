package i18n

import (
	"reflect"
	"testing"
)

func TestMessagesPopulated(t *testing.T) {
	langs := []struct {
		name string
		msg  Messages
	}{
		{"English", En},
		{"Portuguese", Pt},
	}

	for _, l := range langs {
		t.Run(l.name, func(t *testing.T) {
			v := reflect.ValueOf(l.msg)
			typ := v.Type()

			for i := 0; i < v.NumField(); i++ {
				field := v.Field(i)
				fieldName := typ.Field(i).Name

				if field.Kind() == reflect.String {
					if field.String() == "" {
						t.Errorf("Language %q: Field %q is empty", l.name, fieldName)
					}
				}
			}
		})
	}
}

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
