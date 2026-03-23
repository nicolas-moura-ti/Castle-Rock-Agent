package tui

import (
	"os"
	"strings"
	"testing"
)

func TestExportLogsSecure(t *testing.T) {
	// Setup a model with some logs
	m := Model{
		showLogs:      true,
		logLines:      []LogEntry{{Container: "test-container", Text: "Hello, secure world!"}},
		logContainers: []string{"test-container"},
	}

	// Trigger exportLogs
	res, _ := m.exportLogs()
	newM := res.(Model)

	// Find the filename in the event log
	var fileName string
	for _, event := range newM.events {
		if event.Action == "export" {
			parts := strings.Split(event.Name, " → ")
			if len(parts) == 2 {
				fileName = parts[1]
			}
		}
	}

	if fileName == "" {
		t.Fatal("Export failed or filename not found in events")
	}

	// Ensure the file exists
	info, err := os.Stat(fileName)
	if err != nil {
		t.Fatalf("Exported file does not exist: %v", err)
	}
	defer os.Remove(fileName)

	// Verify permissions (os.CreateTemp usually creates with 0600 on Linux)
	// On some systems it might be different, but it should definitely NOT be 0644 if we want it secure.
	// Actually, Go's os.CreateTemp documentation says:
	// "The file is created with mode 0600 (readable and writable only by the owner)."
	mode := info.Mode().Perm()
	if mode != 0600 {
		t.Errorf("Expected file permissions 0600, got %v", mode)
	}

	// Verify content
	content, err := os.ReadFile(fileName)
	if err != nil {
		t.Fatalf("Failed to read exported file: %v", err)
	}
	if !strings.Contains(string(content), "Hello, secure world!") {
		t.Errorf("File content mismatch. Got: %s", string(content))
	}
}
