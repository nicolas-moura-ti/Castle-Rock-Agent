package logger

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestPrettyHandler_Enabled(t *testing.T) {
	// Create a handler with Info level
	h := NewPrettyHandler(nil, &PrettyHandlerOptions{Level: slog.LevelInfo})

	if h.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("Expected Debug to be disabled when level is Info")
	}

	if !h.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("Expected Info to be enabled when level is Info")
	}

	if !h.Enabled(context.Background(), slog.LevelWarn) {
		t.Error("Expected Warn to be enabled when level is Info")
	}

	if !h.Enabled(context.Background(), slog.LevelError) {
		t.Error("Expected Error to be enabled when level is Info")
	}

	// Test default level
	hDefault := NewPrettyHandler(nil, nil)
	if hDefault.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("Expected Debug to be disabled for default (Info)")
	}
	if !hDefault.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("Expected Info to be enabled for default (Info)")
	}
}

func TestPrettyHandler_Handle(t *testing.T) {
	var buf bytes.Buffer
	h := NewPrettyHandler(&buf, &PrettyHandlerOptions{Level: slog.LevelDebug})

	record := slog.NewRecord(time.Date(2023, 10, 25, 12, 0, 0, 0, time.UTC), slog.LevelInfo, "test message", 0)
	record.AddAttrs(slog.String("key1", "val1"), slog.Int("key2", 42))

	err := h.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	out := buf.String()

	// Check for timestamp
	expectedTime := "2023-10-25 12:00:00.000"
	if !strings.Contains(out, expectedTime) {
		t.Errorf("Expected output to contain time %q, got: %q", expectedTime, out)
	}

	// Check for level string
	if !strings.Contains(out, "INFO ") {
		t.Errorf("Expected output to contain level INFO, got: %q", out)
	}

	// Check for message
	if !strings.Contains(out, "test message") {
		t.Errorf("Expected output to contain message 'test message', got: %q", out)
	}

	// Check for attributes
	if !strings.Contains(out, "key1") || !strings.Contains(out, "val1") {
		t.Errorf("Expected output to contain attribute key1=val1, got: %q", out)
	}

	if !strings.Contains(out, "key2") || !strings.Contains(out, "42") {
		t.Errorf("Expected output to contain attribute key2=42, got: %q", out)
	}
}

func TestPrettyHandler_WithAttrs(t *testing.T) {
	var buf bytes.Buffer
	h := NewPrettyHandler(&buf, &PrettyHandlerOptions{Level: slog.LevelInfo})

	h2 := h.WithAttrs([]slog.Attr{slog.String("pre_key", "pre_val")})

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	record.AddAttrs(slog.String("post_key", "post_val"))

	err := h2.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	out := buf.String()

	if !strings.Contains(out, "pre_key") || !strings.Contains(out, "pre_val") {
		t.Errorf("Expected output to contain pre-defined attribute pre_key=pre_val, got: %q", out)
	}

	if !strings.Contains(out, "post_key") || !strings.Contains(out, "post_val") {
		t.Errorf("Expected output to contain record attribute post_key=post_val, got: %q", out)
	}

	// Check that empty attr is ignored
	buf.Reset()
	h3 := h2.WithAttrs([]slog.Attr{{}})
	err = h3.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
}

func TestPrettyHandler_WithGroup(t *testing.T) {
	var buf bytes.Buffer
	h := NewPrettyHandler(&buf, &PrettyHandlerOptions{Level: slog.LevelInfo})

	h2 := h.WithGroup("group1")
	h3 := h2.WithGroup("group2")

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	record.AddAttrs(slog.String("key", "val"))

	err := h3.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	out := buf.String()

	expectedKey := "group1.group2.key"
	if !strings.Contains(out, expectedKey) {
		t.Errorf("Expected output to contain grouped key %q, got: %q", expectedKey, out)
	}
}

func TestSetup(t *testing.T) {
	tests := []struct {
		name     string
		levelStr string
	}{
		{"Debug level", "debug"},
		{"Info level", "info"},
		{"Warn level", "warn"},
		{"Warning level", "warning"},
		{"Error level", "error"},
		{"Unknown level defaults to Info", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := Setup(tt.levelStr)
			if logger == nil {
				t.Error("Setup returned nil logger")
			}
		})
	}
}
