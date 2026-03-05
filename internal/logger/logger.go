// Package logger provides a structured and colorized logging system
// for the Castle Rock Agent.
//
// LOGGING ARCHITECTURE:
//
//	This package uses log/slog (Go 1.21+), the structured logger from
//	the Go standard library. slog is the evolution of the old log package,
//	offering:
//
//	- Structured logs with key-value pairs (not just strings)
//	- Native log levels (DEBUG, INFO, WARN, ERROR)
//	- Customizable handlers (JSON for production, colored text for dev)
//	- Optimized performance with lazy argument evaluation
//	- Native integration with context.Context
//
// WHY SLOG INSTEAD OF LOGRUS/ZAP?
//   - slog is part of the standard library — no external dependency
//   - Stable API maintained by the Go core team
//   - Performance comparable to uber-go/zap
//   - For new projects, the Go community recommendation is to use slog
//
// TERMINAL COLORS:
//
//	We use ANSI escape codes to colorize terminal output.
//	This is a common practice in professional CLIs and dramatically
//	improves log readability during development.
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// ANSI COLOR CONSTANTS
// ─────────────────────────────────────────────────────────────────────────────
//
// ANSI codes are escape sequences interpreted by the terminal
// to change text color and style. Format: \033[<code>m
//
// Reference:
//   - 0 = Reset (back to default)
//   - 1 = Bold
//   - 2 = Dim (opaco)
//   - 3x = Foreground colors (30-37)
//   - 9x = Bright foreground colors (90-97)
const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
	colorItalic = "\033[3m"

	// Foreground colors
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorMagenta = "\033[35m"
	colorCyan    = "\033[36m"
	colorWhite   = "\033[37m"

	// Bright colors (more vibrant)
	colorBrightRed    = "\033[91m"
	colorBrightGreen  = "\033[92m"
	colorBrightYellow = "\033[93m"
	colorBrightBlue   = "\033[94m"
	colorBrightCyan   = "\033[96m"
	colorBrightWhite  = "\033[97m"
)

// ─────────────────────────────────────────────────────────────────────────────
// PRETTY HANDLER (Custom Handler for slog)
// ─────────────────────────────────────────────────────────────────────────────
//
// slog.Handler is an interface that defines HOW logs are formatted and
// written. Go provides two built-in handlers:
//   - slog.TextHandler — key=value output (for humans)
//   - slog.JSONHandler — JSON output (for log systems like ELK/Loki)
//
// We create a custom handler (PrettyHandler) that adds:
//   - ANSI colors for each log level
//   - Timestamp formatted with milliseconds
//   - Visual separators for better readability
//   - Field alignment for tabular output
//
// IMPLEMENTING THE slog.Handler INTERFACE:
//   To create a custom handler, we must implement:
//   - Enabled(ctx, level) bool — determines if the level should be logged
//   - Handle(ctx, record) error — formats and writes the log
//   - WithAttrs(attrs) Handler — creates handler with pre-defined attributes
//   - WithGroup(name) Handler — creates handler with attribute group

// PrettyHandler is our custom handler that produces colorized
// and formatted logs for the terminal.
type PrettyHandler struct {
	// mu protects concurrent writes to the writer.
	// In Go, multiple goroutines can log simultaneously,
	// so we need a mutex to avoid corrupted output.
	//
	// NOTE: sync.Mutex is preferable to channels for simple protection
	// of shared resources. Channels are for communication between
	// goroutines; mutexes are for mutual exclusion.
	mu sync.Mutex

	// w is the destination writer (normally os.Stderr).
	w io.Writer

	// level is the minimum log level to display.
	level slog.Level

	// attrs are pre-defined attributes added to every log.
	attrs []slog.Attr

	// group is the current group prefix.
	group string
}

// PrettyHandlerOptions configures the PrettyHandler.
//
// GO PATTERN — Options struct:
//
//	Instead of passing many parameters to the constructor, we use an
//	options struct. This allows:
//	- Default values for unfilled fields (Go zero values)
//	- Adding new options without breaking the existing API
//	- Clear documentation for each option
type PrettyHandlerOptions struct {
	// Level defines the minimum log level.
	// Default: slog.LevelInfo
	Level slog.Level
}

// NewPrettyHandler creates a new PrettyHandler that writes colorized
// and formatted logs to the specified writer.
//
// Usage example:
//
//	handler := logger.NewPrettyHandler(os.Stderr, &logger.PrettyHandlerOptions{
//	    Level: slog.LevelDebug,
//	})
//	slog.SetDefault(slog.New(handler))
func NewPrettyHandler(w io.Writer, opts *PrettyHandlerOptions) *PrettyHandler {
	level := slog.LevelInfo
	if opts != nil {
		level = opts.Level
	}

	return &PrettyHandler{
		w:     w,
		level: level,
	}
}

// Enabled reports whether the handler processes logs at this level.
//
// slog calls this method BEFORE constructing the Record, avoiding
// unnecessary allocations for logs that will be discarded.
// This is an important performance optimization.
func (h *PrettyHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

// Handle formats and writes a log record with ANSI colors.
//
// This is the heart of the handler — here we decide how each log appears
// in the terminal. The design prioritizes readability and scannability:
//   - Timestamp with milliseconds for precise correlation
//   - Colorized level with fixed width for alignment
//   - Message highlighted (bold)
//   - Attributes formatted as key=value with differentiated colors
func (h *PrettyHandler) Handle(_ context.Context, r slog.Record) error {
	// Formats the timestamp with milliseconds.
	// The format "15:04:05.000" follows the Go reference time convention:
	// Mon Jan 2 15:04:05 MST 2006 (Go's "reference moment").
	timeStr := r.Time.Format("2006-01-02 15:04:05.000")

	// Determine the color and label based on the log level.
	levelStr, levelColor := h.formatLevel(r.Level)

	// Build the log line with ANSI colors.
	var b strings.Builder

	// Timestamp in dim so it doesn't visually compete with the message
	fmt.Fprintf(&b, "%s%s%s ", colorDim, timeStr, colorReset)

	// Level with color and fixed width (5 chars) for alignment
	fmt.Fprintf(&b, "%s%-5s%s ", levelColor, levelStr, colorReset)

	// Vertical separator for visual clarity
	fmt.Fprintf(&b, "%s│%s ", colorDim, colorReset)

	// Main message in bold
	fmt.Fprintf(&b, "%s%s%s", colorBold, r.Message, colorReset)

	// Add pre-defined attributes (from WithAttrs)
	for _, attr := range h.attrs {
		h.appendAttr(&b, attr)
	}

	// Add Record attributes (passed in the log call)
	r.Attrs(func(a slog.Attr) bool {
		h.appendAttr(&b, a)
		return true // continue iterating
	})

	b.WriteString("\n")

	// Mutex for thread-safe writes.
	// Without this, logs from multiple goroutines could interleave.
	h.mu.Lock()
	defer h.mu.Unlock()

	_, err := h.w.Write([]byte(b.String()))
	return err
}

// WithAttrs returns a new handler with additional attributes.
//
// This method implements the "contextual logger" pattern:
// it allows creating specialized loggers that always include
// certain fields. Example:
//
//	dockerLogger := logger.With("component", "docker")
//	dockerLogger.Info("connected") // always includes component=docker
func (h *PrettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs), len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	newAttrs = append(newAttrs, attrs...)

	return &PrettyHandler{
		w:     h.w,
		level: h.level,
		attrs: newAttrs,
		group: h.group,
	}
}

// WithGroup returns a new handler with a group prefix.
func (h *PrettyHandler) WithGroup(name string) slog.Handler {
	newGroup := name
	if h.group != "" {
		newGroup = h.group + "." + name
	}

	return &PrettyHandler{
		w:     h.w,
		level: h.level,
		attrs: h.attrs,
		group: newGroup,
	}
}

// formatLevel returns the label and color for each log level.
func (h *PrettyHandler) formatLevel(level slog.Level) (string, string) {
	switch {
	case level < slog.LevelInfo:
		return "DEBUG", colorBrightCyan
	case level < slog.LevelWarn:
		return "INFO", colorBrightGreen
	case level < slog.LevelError:
		return "WARN", colorBrightYellow
	default:
		return "ERROR", colorBrightRed
	}
}

// appendAttr formats and appends an attribute to the builder.
func (h *PrettyHandler) appendAttr(b *strings.Builder, a slog.Attr) {
	// Ignore attributes with empty values.
	if a.Equal(slog.Attr{}) {
		return
	}

	key := a.Key
	if h.group != "" {
		key = h.group + "." + key
	}

	// Key in cyan, value in white for visual differentiation
	fmt.Fprintf(b, " %s%s%s=%s%v%s",
		colorCyan, key, colorReset,
		colorWhite, a.Value, colorReset,
	)
}

// ─────────────────────────────────────────────────────────────────────────────
// ─────────────────────────────────────────────────────────────────────────────
// GLOBAL INITIALIZATION
// ─────────────────────────────────────────────────────────────────────────────

// Setup configures the application's global logger.
//
// GO PATTERN — slog.SetDefault:
//
//	slog.SetDefault defines the default logger used by slog.Info(),
//	slog.Error(), etc. This allows any package to use logging
//	without receiving the logger as a parameter.
//
//	In larger projects, consider dependency injection (passing
//	*slog.Logger as a parameter). For the MVP, the global logger is acceptable.
func Setup(level string) *slog.Logger {
	// Convert level string to slog.Level
	var slogLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		slogLevel = slog.LevelDebug
	case "warn", "warning":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	handler := NewPrettyHandler(os.Stderr, &PrettyHandlerOptions{
		Level: slogLevel,
	})

	logger := slog.New(handler)
	slog.SetDefault(logger)

	return logger
}

// ─────────────────────────────────────────────────────────────────────────────
// DISPLAY FUNCTIONS (Formatted Output)
// ─────────────────────────────────────────────────────────────────────────────
//
// These functions are not structured logs — they are formatted visual output
// for the terminal. We use fmt.Fprintf for direct output instead of slog
// because they are data displays, not log events.

// PrintBanner displays the agent startup banner.
//
// A professional banner communicates software identity and version.
// In production, this helps identify which version is running
// when troubleshooting historical logs.
func PrintBanner(version, goVersion string) {
	banner := fmt.Sprintf(`
%s%s┌─────────────────────────────────────────────────────────┐%s
%s%s│%s    🏰  %s%sCastle Rock Agent%s                                %s%s│%s
%s%s│%s    Real-time Docker observability                %s%s│%s
%s%s├─────────────────────────────────────────────────────────┤%s
%s%s│%s  %sVersion:%s    %-46s %s%s│%s
%s%s│%s  %sGo:%s        %-46s %s%s│%s
%s%s│%s  %sArch:%s      %-46s %s%s│%s
%s%s│%s  %sPID:%s       %-46d %s%s│%s
%s%s└─────────────────────────────────────────────────────────┘%s
`,
		colorBold, colorBrightBlue, colorReset,
		colorBold, colorBrightBlue, colorReset, colorBold, colorBrightWhite, colorReset, colorBold, colorBrightBlue, colorReset,
		colorBold, colorBrightBlue, colorReset, colorBold, colorBrightBlue, colorReset,
		colorBold, colorBrightBlue, colorReset,
		colorBold, colorBrightBlue, colorReset, colorCyan, colorReset, version, colorBold, colorBrightBlue, colorReset,
		colorBold, colorBrightBlue, colorReset, colorCyan, colorReset, goVersion, colorBold, colorBrightBlue, colorReset,
		colorBold, colorBrightBlue, colorReset, colorCyan, colorReset, fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH), colorBold, colorBrightBlue, colorReset,
		colorBold, colorBrightBlue, colorReset, colorCyan, colorReset, os.Getpid(), colorBold, colorBrightBlue, colorReset,
		colorBold, colorBrightBlue, colorReset,
	)

	fmt.Fprint(os.Stderr, banner)
}

// PrintDockerInfo displays Docker daemon information in table format.
func PrintDockerInfo(info map[string]string) {
	fmt.Fprintf(os.Stderr, "\n%s%s  ⚙  Docker Engine%s\n", colorBold, colorMagenta, colorReset)
	fmt.Fprintf(os.Stderr, "%s  ───────────────────────────────────────────────%s\n", colorDim, colorReset)

	// Defined order for visual consistency
	keys := []string{"Server Version", "API Version", "OS/Arch", "Kernel", "Total Memory", "Containers", "Images"}
	for _, key := range keys {
		if val, ok := info[key]; ok {
			fmt.Fprintf(os.Stderr, "  %s%-16s%s %s%s%s\n",
				colorCyan, key+":", colorReset,
				colorWhite, val, colorReset,
			)
		}
	}
	fmt.Fprintln(os.Stderr)
}

// PrintContainerTable displays containers in a colorized table format.
func PrintContainerTable(containers []ContainerDisplay) {
	if len(containers) == 0 {
		fmt.Fprintf(os.Stderr, "\n%s  📭 No running containers found%s\n\n", colorDim, colorReset)
		return
	}

	fmt.Fprintf(os.Stderr, "\n%s%s  🐳  Running Containers (%d)%s\n",
		colorBold, colorGreen, len(containers), colorReset)
	fmt.Fprintf(os.Stderr, "%s  ═══════════════════════════════════════════════════════════════════%s\n",
		colorDim, colorReset)

	for i, c := range containers {
		// State with color
		var stateColor, stateIcon string
		switch c.State {
		case "running":
			stateColor = colorBrightGreen
			stateIcon = "●"
		case "paused":
			stateColor = colorBrightYellow
			stateIcon = "◉"
		case "restarting":
			stateColor = colorBrightCyan
			stateIcon = "↻"
		default:
			stateColor = colorBrightRed
			stateIcon = "○"
		}

		fmt.Fprintf(os.Stderr, "\n  %s%s%s #%d %s%s%s\n",
			stateColor, stateIcon, colorReset,
			i+1,
			colorBold, c.Name, colorReset,
		)
		fmt.Fprintf(os.Stderr, "  %s┌──────────────────────────────────────────────────────────────┐%s\n",
			colorDim, colorReset)

		// Container data
		printField("ID", c.ID)
		printField("Image", c.Image)
		printField("Status", c.Status)
		printField("State", fmt.Sprintf("%s%s%s", stateColor, c.State, colorReset))
		if c.Command != "" {
			printField("Command", c.Command)
		}
		if c.Ports != "" {
			printField("Ports", c.Ports)
		}
		if c.Created != "" {
			printField("Created", c.Created)
		}
		if len(c.Networks) > 0 {
			printField("Networks", strings.Join(c.Networks, ", "))
		}
		if len(c.Labels) > 0 {
			printField("Labels", "")
			for k, v := range c.Labels {
				fmt.Fprintf(os.Stderr, "  %s│%s     %s%s%s=%s%s%s\n",
					colorDim, colorReset,
					colorItalic, k, colorReset,
					colorDim, v, colorReset,
				)
			}
		}
		if c.SizeRw != "" {
			printField("Size (RW)", c.SizeRw)
		}

		fmt.Fprintf(os.Stderr, "  %s└──────────────────────────────────────────────────────────────┘%s\n",
			colorDim, colorReset)
	}

	fmt.Fprintln(os.Stderr)
}

// printField prints a formatted field inside the container table.
func printField(key, value string) {
	fmt.Fprintf(os.Stderr, "  %s│%s  %s%-14s%s %s%s%s\n",
		colorDim, colorReset,
		colorCyan, key+":", colorReset,
		colorWhite, value, colorReset,
	)
}

// PrintShutdown displays the shutdown message.
func PrintShutdown() {
	fmt.Fprintf(os.Stderr, "\n%s%s  🛑  Shutting down Castle Rock Agent...%s\n", colorBold, colorRed, colorReset)
	fmt.Fprintf(os.Stderr, "%s  ───────────────────────────────────────────────%s\n", colorDim, colorReset)
}

// PrintUptime displays the agent's uptime.
func PrintUptime(startTime time.Time) {
	uptime := time.Since(startTime).Round(time.Second)
	fmt.Fprintf(os.Stderr, "  %sUptime:%s          %s%s%s\n",
		colorCyan, colorReset,
		colorWhite, uptime, colorReset,
	)
}

// ContainerDisplay is a DTO used by the logger for detailed display.
// Separated from models.ContainerInfo to keep the model clean.
type ContainerDisplay struct {
	HostID        string
	ID            string
	Name          string
	Image         string
	Status        string
	State         string
	Command       string
	Ports         string
	Created       string
	Networks      []string
	Labels        map[string]string
	SizeRw        string
	Env           []string
	HealthStatus  string
	HealthLog     string
	Mounts        []string
	RestartPolicy string
	RestartCount  int
	CPULimit      float64
	MemoryLimit   int64
	Entrypoint    string
}
