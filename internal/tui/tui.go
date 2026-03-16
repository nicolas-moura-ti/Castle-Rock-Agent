// Package tui implements the interactive interface of Castle Rock Agent
// using the Bubble Tea framework (charmbracelet/bubbletea).
//
// BUBBLE TEA — ELM ARCHITECTURE:
//
//	Model → Update → View (unidirectional cycle)
//
// FEATURES:
//   - Container table with real-time metrics (CPU/MEM/NET)
//   - Real-time Docker event log
//   - Expanded details for the selected container
//   - Container log streaming (toggle with 'l')
//   - Container actions: stop ('s'), restart ('R') with confirmation
//   - Alert system with visual indicator
//   - Shortcuts: ↑↓/jk navigate, enter details, l logs, s stop, R restart, ? help, q quit
package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/nicolas-moura-ti/castle-rock-agent/internal/alerts"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/config"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/docker"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/i18n"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/logger"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/metrics"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/security"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/storage"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/topology"
	"github.com/nicolas-moura-ti/castle-rock-agent/pkg/models"
)

// ─── Estilos ─────────────────────────────────────────────────────────────────

var (
	primaryColor   = lipgloss.Color("#7C3AED")
	secondaryColor = lipgloss.Color("#06B6D4")
	successColor   = lipgloss.Color("#22C55E")
	warningColor   = lipgloss.Color("#EAB308")
	dangerColor    = lipgloss.Color("#EF4444")
	mutedColor     = lipgloss.Color("#6B7280")

	titleStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(primaryColor).Padding(0, 2)
	statusBarStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#3B3552")).Padding(0, 1)
	selectedStyle  = lipgloss.NewStyle().Bold(true).Foreground(primaryColor)
	normalStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#E2E8F0"))
	headerStyle    = lipgloss.NewStyle().Bold(true).Foreground(secondaryColor)

	stateRunning = lipgloss.NewStyle().Foreground(successColor).Bold(true)
	statePaused  = lipgloss.NewStyle().Foreground(warningColor).Bold(true)
	stateStopped = lipgloss.NewStyle().Foreground(dangerColor).Bold(true)

	cpuLowStyle  = lipgloss.NewStyle().Foreground(successColor)
	cpuMedStyle  = lipgloss.NewStyle().Foreground(warningColor)
	cpuHighStyle = lipgloss.NewStyle().Foreground(dangerColor).Bold(true)
	memLowStyle  = lipgloss.NewStyle().Foreground(successColor)
	memMedStyle  = lipgloss.NewStyle().Foreground(warningColor)
	memHighStyle = lipgloss.NewStyle().Foreground(dangerColor).Bold(true)
	netStyle     = lipgloss.NewStyle().Foreground(secondaryColor)

	alertCritStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(dangerColor).Padding(0, 1)
	alertWarnStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#000000")).Background(warningColor).Padding(0, 1)

	eventPanelStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(mutedColor).Padding(0, 1)
	logPanelStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(secondaryColor).Padding(0, 1)
	helpStyle       = lipgloss.NewStyle().Foreground(mutedColor)
	confirmStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(dangerColor).Padding(0, 2)
	stressStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#F97316")).Padding(0, 2)
)

// ─── Mensagens ───────────────────────────────────────────────────────────────

type containerListMsg struct{ containers []logger.ContainerDisplay }
type dockerEventMsg struct{ event docker.DockerEvent }
type dockerErrorMsg struct{ err error }
type statsMsg struct {
	stats map[string]models.ContainerMetrics
}
type logLineMsg struct {
	container string
	line      string
	nextCh    <-chan string
}
type LogEntry struct {
	Container string
	Text      string
}
type actionResultMsg struct {
	success   bool
	action    string
	container string
	err       error
}
type stressResultMsg struct {
	success bool
	mode    string
	err     error
}
type tickMsg time.Time

type EventLogEntry struct {
	Time   time.Time
	Icon   string
	Action string
	Name   string
}

// ─── Model ───────────────────────────────────────────────────────────────────

type Model struct {
	containers []logger.ContainerDisplay
	stats      map[string]models.ContainerMetrics
	events     []EventLogEntry
	sysInfo    map[string]string

	// Alerts
	alertEngine    *alerts.Engine
	activeAlerts   []alerts.Alert
	securityAlerts []alerts.Alert
	auditor        *security.Auditor

	// Logs
	logLines      []LogEntry
	showLogs      bool
	logContainers []string        // list of container names being displayed
	logOffset     int             // distance from end (0 = tail)
	logSearch     string          // grep filter
	searchMode    bool            // typing in search bar
	selectedIDs   map[string]bool // selected containers (space)
	showAll       bool            // show stopped containers (toggle with 'a')

	// Container actions
	confirmAction string // "stop" or "restart" — empty = no pending confirmation

	// Stress test
	showStress bool // shows the stress test menu

	// Cleanup / Prune
	showCleanup   bool
	diskUsage     docker.SystemDiskUsage
	pruning       bool
	pruneFeedback string

	// Service Map
	showMap bool
	mapData []topology.NetworkEdge
	mapper  *topology.Mapper

	// UI state
	cursor     int
	showHelp   bool
	showDetail bool
	width      int
	height     int

	// Meta
	startTime    time.Time
	version      string
	dockerClient *docker.Client
	receiver     metrics.ClusterProvider
	store        *storage.SQLiteStore
	ctx          context.Context
	cfg          config.Config
	eventCount   int
	lastUpdate   time.Time
	quitting     bool
	msg          i18n.Messages
	hostCPU      float64
	hostMem      float64
}

func NewModel(dockerClient *docker.Client, receiver metrics.ClusterProvider, ctx context.Context, sysInfo map[string]string, version string, cfg config.Config, store *storage.SQLiteStore) Model {
	var engine *alerts.Engine
	if cfg.Alerts.Enabled {
		engine = alerts.NewEngine(cfg.Alerts.Rules)
	}

	return Model{
		containers:     []logger.ContainerDisplay{},
		stats:          make(map[string]models.ContainerMetrics),
		events:         []EventLogEntry{},
		sysInfo:        sysInfo,
		alertEngine:    engine,
		activeAlerts:   []alerts.Alert{},
		securityAlerts: []alerts.Alert{},
		auditor:        security.NewAuditor(dockerClient),
		mapper:         topology.NewMapper(dockerClient),
		logLines:       []LogEntry{},
		selectedIDs:    make(map[string]bool),
		startTime:      time.Now(),
		version:        version,
		dockerClient:   dockerClient,
		receiver:       receiver,
		ctx:            ctx,
		cfg:            cfg,
		lastUpdate:     time.Now(),
		msg:            i18n.Get(cfg.Language),
	}
}

// ─── Init ────────────────────────────────────────────────────────────────────

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.fetchContainers(),
		m.fetchStats(),
		m.fetchHostStats(),
		m.watchDockerEvents(),
		m.tickCmd(),
	)
}

// ─── Update ──────────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case containerListMsg:
		return m.handleContainerListMsg(msg)
	case statsMsg:
		return m.handleStatsMsg(msg)
	case dockerEventMsg:
		return m.handleDockerEventMsg(msg)
	case dockerErrorMsg:
		m.events = append([]EventLogEntry{{
			Time: time.Now(), Icon: "❌", Action: "error", Name: msg.err.Error(),
		}}, m.events...)
	case logLineMsg:
		return m.handleLogLineMsg(msg)
	case actionResultMsg:
		return m.handleActionResultMsg(msg)
	case stressResultMsg:
		return m.handleStressResultMsg(msg)
	case diskUsageMsg:
		m.diskUsage = msg.usage
	case pruneResultMsg:
		return m.handlePruneResultMsg(msg)
	case hostStatsMsg:
		m.hostCPU = msg.cpu
		m.hostMem = msg.mem
	case tickMsg:
		return m.handleTickMsg(msg)
	}
	return m, nil
}

func (m Model) handleContainerListMsg(msg containerListMsg) (tea.Model, tea.Cmd) {
	m.containers = msg.containers
	m.lastUpdate = time.Now()
	if m.cursor >= len(m.containers) && len(m.containers) > 0 {
		m.cursor = len(m.containers) - 1
	}

	m = m.processSecurityAlerts()

	return m, nil
}

func (m Model) processSecurityAlerts() Model {
	if m.auditor == nil {
		return m
	}

	oldAlertsCount := len(m.securityAlerts)
	m.securityAlerts = m.auditor.Audit(m.ctx, m.containers)
	if len(m.securityAlerts) > oldAlertsCount {
		m.events = append([]EventLogEntry{{
			Time:   time.Now(),
			Icon:   "🛡️ ",
			Action: "SEC-AUDIT",
			Name:   fmt.Sprintf("%d issues found", len(m.securityAlerts)),
		}}, m.events...)
	}

	return m
}

func (m Model) handleStatsMsg(msg statsMsg) (tea.Model, tea.Cmd) {
	m.stats = msg.stats
	if m.alertEngine != nil {
		newAlerts := m.alertEngine.Evaluate(msg.stats)
		m.activeAlerts = m.alertEngine.GetActiveAlerts()
		for _, a := range newAlerts {
			icon := "⚠️ "
			if a.Severity == "critical" {
				icon = "🚨"
			}
			m.events = append([]EventLogEntry{{
				Time:   time.Now(),
				Icon:   icon,
				Action: "ALERT",
				Name:   alerts.FormatAlert(a),
			}}, m.events...)
		}
	}
	return m, nil
}

func (m Model) handleDockerEventMsg(msg dockerEventMsg) (tea.Model, tea.Cmd) {
	m.eventCount++
	icon := dockerEventIcon(msg.event.Action)
	m.events = append([]EventLogEntry{{
		Time: time.Now(), Icon: icon, Action: msg.event.Action, Name: msg.event.ContainerName,
	}}, m.events...)
	if m.store != nil {
		m.store.SaveEvent(m.ctx, msg.event.Action, msg.event.ContainerName, "")
	}
	if len(m.events) > 50 {
		m.events = m.events[:50]
	}
	return m, tea.Batch(m.fetchContainers(), m.fetchStats(), m.watchNextDockerEvent())
}

func dockerEventIcon(action string) string {
	switch action {
	case "start":
		return "🟢"
	case "stop":
		return "🟡"
	case "die":
		return "🔴"
	case "create":
		return "📦"
	case "destroy":
		return "🗑️ "
	case "pause":
		return "⏸️ "
	case "unpause":
		return "▶️ "
	default:
		return "📋"
	}
}

func (m Model) handleLogLineMsg(msg logLineMsg) (tea.Model, tea.Cmd) {
	m.logLines = append(m.logLines, LogEntry{Container: msg.container, Text: msg.line})
	if len(m.logLines) > 1000 {
		m.logLines = m.logLines[len(m.logLines)-1000:]
	}
	if msg.nextCh != nil {
		return m, m.waitForNextLog(msg.container, msg.nextCh)
	}
	return m, nil
}

func (m Model) handleActionResultMsg(msg actionResultMsg) (tea.Model, tea.Cmd) {
	icon := "✅"
	action := msg.action
	if !msg.success {
		icon = "❌"
		action = msg.action + " FAILED"
	}
	m.events = append([]EventLogEntry{{
		Time: time.Now(), Icon: icon, Action: action, Name: msg.container,
	}}, m.events...)
	return m, tea.Batch(m.fetchContainers(), m.fetchStats())
}

func (m Model) handleStressResultMsg(msg stressResultMsg) (tea.Model, tea.Cmd) {
	icon := "⚡"
	name := "stress-" + msg.mode + " (30s)"
	if !msg.success {
		icon = "❌"
		name = "stress FAILED: " + msg.err.Error()
	}
	m.events = append([]EventLogEntry{{
		Time: time.Now(), Icon: icon, Action: "stress", Name: name,
	}}, m.events...)
	return m, tea.Batch(m.fetchContainers(), m.fetchStats())
}

func (m Model) handlePruneResultMsg(msg pruneResultMsg) (tea.Model, tea.Cmd) {
	m.pruning = false
	if msg.err != nil {
		m.pruneFeedback = fmt.Sprintf("❌ Error cleaning %s: %s", msg.target, msg.err.Error())
		m.events = append([]EventLogEntry{{
			Time: time.Now(), Icon: "❌", Action: "prune fail", Name: msg.err.Error(),
		}}, m.events...)
	} else {
		reclaimedStr := formatBytes(msg.reclaimed)
		m.pruneFeedback = fmt.Sprintf("✅ Cleanup complete! Reclaimed %s of free space.", reclaimedStr)
		m.events = append([]EventLogEntry{{
			Time: time.Now(), Icon: "🧹 ", Action: "prune " + msg.target, Name: fmt.Sprintf("Freed %s", reclaimedStr),
		}}, m.events...)
	}
	return m, m.fetchDiskUsage()
}

func (m Model) handleTickMsg(msg tickMsg) (tea.Model, tea.Cmd) {
	m.lastUpdate = time.Time(msg)
	if !m.showLogs && !m.showMap && m.confirmAction == "" {
		return m, tea.Batch(
			m.fetchContainers(),
			m.fetchStats(),
			m.fetchHostStats(),
			m.tickCmd(),
		)
	}
	return m, m.tickCmd()
}

// ─── Key Handlers ────────────────────────────────────────────────────────────

func (m Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m, cmd, handled := m.handleModalKeys(msg); handled {
		return m, cmd
	}
	return m.handleNormalKeys(msg)
}

func (m Model) handleModalKeys(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	if m.confirmAction != "" {
		return m.handleConfirmKeys(msg)
	}
	if m.showStress {
		return m.handleStressKeys(msg)
	}
	if m.showCleanup {
		return m.handleCleanupKeys(msg)
	}
	if m.searchMode {
		return m.handleSearchKeys(msg)
	}
	return m, nil, false
}

func (m Model) handleConfirmKeys(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	switch msg.String() {
	case "y", "Y":
		action := m.confirmAction
		m.confirmAction = ""
		if m.cursor < len(m.containers) {
			c := m.containers[m.cursor]
			return m, m.executeAction(action, c.ID, c.Name), true
		}
	default:
		m.confirmAction = ""
	}
	return m, nil, true
}

func (m Model) handleStressKeys(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	switch msg.String() {
	case "c", "m", "b":
		m.showStress = false
		mode := "both"
		if msg.String() == "c" {
			mode = "cpu"
		} else if msg.String() == "m" {
			mode = "memory"
		}
		return m, m.executeStress(mode), true
	default:
		m.showStress = false
	}
	return m, nil, true
}

func (m Model) handleCleanupKeys(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	switch msg.String() {
	case "esc", "c", "C":
		m.showCleanup = false
		m.pruneFeedback = ""
	case "i":
		m.pruning = true
		m.pruneFeedback = ""
		return m, m.runPrune("images"), true
	case "v":
		m.pruning = true
		m.pruneFeedback = ""
		return m, m.runPrune("volumes"), true
	}
	return m, nil, true
}

func (m Model) handleSearchKeys(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	switch msg.String() {
	case "esc":
		if m.searchMode {
			m.searchMode = false
			m.logSearch = ""
		}
	}
	return m, nil, true
}

func (m Model) handleNormalKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "a", "A":
		m.showAll = !m.showAll
		return m, tea.Batch(m.fetchContainers(), m.fetchStats())
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "/", "up", "k", "down", "j", "f", " ":
		return m.handleNavigationKeys(msg)
	case "l", "L", "E":
		return m.handleLogKeys(msg)
	case "s", "R", "x", "S":
		return m.handleActionKeys(msg)
	case "r":
		return m, tea.Batch(m.fetchContainers(), m.fetchStats())
	case "?", "enter", "esc", "C", "m", "M":
		return m.handleViewKeys(msg)
	}
	return m, nil
}

func (m Model) handleNavigationKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "/":
		m.handleSearchToggle()
	case "up", "k":
		m.handleScrollUp()
	case "down", "j":
		m.handleScrollDown()
	case "f":
		m.handleScrollReset()
	case " ":
		m.handleSpaceSelection()
	}
	return m, nil
}

func (m *Model) handleSearchToggle() {
	if m.showLogs {
		m.searchMode = true
	}
}

func (m *Model) handleScrollUp() {
	if m.showLogs {
		m.logOffset++
	} else if m.cursor > 0 {
		m.cursor--
	}
}

func (m *Model) handleScrollDown() {
	if m.showLogs {
		if m.logOffset > 0 {
			m.logOffset--
		}
	} else if m.cursor < len(m.containers)-1 {
		m.cursor++
	}
}

func (m *Model) handleScrollReset() {
	if m.showLogs {
		m.logOffset = 0
	}
}

func (m *Model) handleSpaceSelection() {
	if !m.showLogs && !m.showDetail && !m.showMap && m.cursor < len(m.containers) {
		id := m.containers[m.cursor].ID
		if m.selectedIDs[id] {
			delete(m.selectedIDs, id)
		} else {
			m.selectedIDs[id] = true
		}
	}
}

func (m Model) handleLogKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "E":
		return m.exportLogs()
	case "L":
		return m.startMultiTailLogs()
	case "l":
		return m.toggleSingleContainerLogs()
	}
	return m, nil
}

func (m Model) exportLogs() (tea.Model, tea.Cmd) {
	if !m.showLogs || len(m.logLines) == 0 {
		return m, nil
	}
	var content strings.Builder
	lineCount := 0
	for _, entry := range m.logLines {
		if m.logSearch == "" || strings.Contains(strings.ToLower(entry.Text), strings.ToLower(m.logSearch)) {
			if len(m.logContainers) > 1 {
				content.WriteString("[" + entry.Container + "] ")
			}
			content.WriteString(entry.Text + "\n")
			lineCount++
		}
	}
	fileName := "/tmp/castle-rock-logs"
	if len(m.logContainers) == 1 {
		fileName += "-" + m.logContainers[0]
	} else {
		fileName += "-multi"
	}
	fileName += fmt.Sprintf("-%d.txt", time.Now().Unix())

	err := os.WriteFile(fileName, []byte(content.String()), 0644)
	if err != nil {
		m.events = append([]EventLogEntry{{
			Time: time.Now(), Icon: "❌", Action: "export fail", Name: err.Error(),
		}}, m.events...)
	} else {
		m.events = append([]EventLogEntry{{
			Time: time.Now(), Icon: "📤", Action: "export", Name: fmt.Sprintf("%d lines → %s", lineCount, fileName),
		}}, m.events...)
	}
	return m, nil
}

func (m Model) startMultiTailLogs() (tea.Model, tea.Cmd) {
	if m.showLogs || len(m.selectedIDs) == 0 {
		return m, nil
	}
	m.showLogs = true
	m.logOffset = 0
	m.logLines = []LogEntry{{Container: "System", Text: "Loading aggregate logs..."}}
	var names []string
	var cmds []tea.Cmd
	for _, c := range m.containers {
		if m.selectedIDs[c.ID] {
			names = append(names, c.Name)
			logCh, err := m.dockerClient.StreamContainerLogs(m.ctx, c.ID)
			if err == nil {
				cmds = append(cmds, m.waitForNextLog(c.Name, logCh))
			}
		}
	}
	m.logContainers = names
	return m, tea.Batch(cmds...)
}

func (m Model) toggleSingleContainerLogs() (tea.Model, tea.Cmd) {
	if m.showLogs {
		m.showLogs = false
		m.logLines = nil
		return m, nil
	}
	if m.cursor >= len(m.containers) {
		return m, nil
	}
	c := m.containers[m.cursor]
	m.showLogs = true
	m.logOffset = 0
	m.logLines = []LogEntry{{Container: c.Name, Text: "Loading logs..."}}
	m.logContainers = []string{c.Name}
	logCh, err := m.dockerClient.StreamContainerLogs(m.ctx, c.ID)
	if err == nil {
		return m, m.waitForNextLog(c.Name, logCh)
	}
	return m, nil
}

func (m Model) handleActionKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "s":
		if m.cursor < len(m.containers) {
			m.confirmAction = "stop"
		}
	case "R":
		if m.cursor < len(m.containers) {
			m.confirmAction = "restart"
		}
	case "x":
		if !m.showLogs && !m.showDetail && !m.showMap && m.cursor < len(m.containers) {
			return m, m.executeShell()
		}
	case "S":
		m.showStress = true
	}
	return m, nil
}

func (m Model) handleViewKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "?":
		m.showHelp = !m.showHelp
	case "enter":
		m.showDetail = !m.showDetail
	case "esc":
		m.showDetail = false
		m.showHelp = false
		m.showLogs = false
		m.logLines = nil
		m.searchMode = false
		m.logSearch = ""
		m.confirmAction = ""
		m.showStress = false
		m.showMap = false
		m.showCleanup = false
	case "C":
		if !m.showLogs && !m.showDetail && !m.showMap {
			m.showCleanup = !m.showCleanup
			if m.showCleanup {
				return m, m.fetchDiskUsage()
			}
		}
	case "m", "M":
		m.showMap = !m.showMap
		if m.showMap {
			edges, err := m.mapper.BuildMap(m.ctx)
			if err == nil {
				m.mapData = edges
			}
		}
	}
	return m, nil
}

// ─── View ────────────────────────────────────────────────────────────────────

func (m Model) View() string {
	if m.quitting {
		return "\n  👋 Castle Rock Agent encerrado.\n\n"
	}

	var b strings.Builder

	if m.width < 60 {
		b.WriteString("\n  ⚠️  " + m.msg.ErrorSmallTerminal + "\n")
		return b.String()
	}

	// Stress test menu
	if m.showStress {
		b.WriteString(m.renderHeader())
		b.WriteString("\n\n")
		b.WriteString(stressStyle.Render("  ⚡ " + m.msg.StressTestTitle + "  "))
		b.WriteString("\n\n")
		b.WriteString(lipgloss.NewStyle().MarginLeft(4).Render(m.msg.StressTestMenu + "\n"))
		return b.String()
	}

	// Interactive Prune Dashboard
	if m.showCleanup {
		b.WriteString(m.renderHeader())
		b.WriteString("\n\n")
		b.WriteString(m.renderCleanupPanel())
		return b.String()
	}

	// Action confirmation
	if m.confirmAction != "" && m.cursor < len(m.containers) {
		b.WriteString(m.renderHeader())
		b.WriteString("\n\n")
		c := m.containers[m.cursor]
		b.WriteString(confirmStyle.Render(
			fmt.Sprintf("  ⚠️  %s container '%s'? (y = confirm, any key = cancel)  ",
				m.confirmAction, c.Name),
		))
		b.WriteString("\n")
		return b.String()
	}

	// Menu Service Map (Topologia)
	if m.showMap {
		b.WriteString(m.renderHeader())
		b.WriteString("\n\n")
		b.WriteString(titleStyle.Render(" 🕸️  " + m.msg.ServiceMapTitle + " "))
		b.WriteString("\n\n")

		if len(m.mapData) == 0 {
			b.WriteString("  " + m.msg.ServiceMapNoCustom + "\n")
		} else {
			for _, net := range m.mapData {
				b.WriteString(headerStyle.Render(fmt.Sprintf("  🌐 %s (%s)", net.NetworkName, net.Driver)) + "\n")
				for _, node := range net.Nodes {
					b.WriteString(fmt.Sprintf("     ├─ %-25s [%s]\n", node.ContainerName, node.IPv4Address))
				}
				b.WriteString("\n")
			}
		}

		b.WriteString("\n  " + m.msg.ServiceMapBack + "\n")
		return b.String()
	}

	b.WriteString(m.renderHeader())
	b.WriteString("\n")
	b.WriteString(m.renderContainerTable())

	if m.showDetail && len(m.containers) > 0 {
		b.WriteString(m.renderContainerDetail())
		b.WriteString("\n")
	}

	if m.showLogs {
		b.WriteString(m.renderLogPanel())
		b.WriteString("\n")
	}

	// Alertas ativos
	if len(m.activeAlerts) > 0 {
		b.WriteString(m.renderAlerts())
		b.WriteString("\n")
	}

	b.WriteString(m.renderEventLog())
	b.WriteString("\n")
	b.WriteString(m.renderHelpBar())

	return b.String()
}

// ─── Render helpers ──────────────────────────────────────────────────────────

func (m Model) renderHeader() string {
	title := titleStyle.Render(" 🏰 Castle Rock Agent ")
	uptime := time.Since(m.startTime).Round(time.Second)

	dockerVer := "N/A"
	if v, ok := m.sysInfo["Server Version"]; ok {
		dockerVer = v
	}

	alertIndicator := ""
	if len(m.activeAlerts) > 0 {
		alertIndicator = fmt.Sprintf(" │ 🚨 %d alerts", len(m.activeAlerts))
	}

	info := statusBarStyle.Render(
		fmt.Sprintf(" v%s │ Docker %s │ ⏱  %s │ 📡 %d events%s ",
			m.version, dockerVer, uptime, m.eventCount, alertIndicator,
		),
	)

	return title + "  " + info
}

func (m Model) renderContainerTable() string {
	if len(m.containers) == 0 {
		return lipgloss.NewStyle().Foreground(mutedColor).Padding(1, 2).
			Render("📭 " + m.msg.NoContainers + "\n")
	}

	var b strings.Builder
	header := fmt.Sprintf("  %-6s %-12s %-20s %-7s %-7s %-9s %-10s %s",
		m.msg.TableHost, "ID", m.msg.TableName, "CPU%", "MEM%", "MEM", "NET", m.msg.TableState)
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n")

	for i, c := range m.containers {
		b.WriteString(m.formatContainerRow(i, c))
		b.WriteString("\n")
	}

	return b.String()
}

func (m *Model) formatContainerRow(index int, c logger.ContainerDisplay) string {
	cursor := "  "
	style := normalStyle
	if index == m.cursor {
		cursor = "▸ "
		style = selectedStyle
	}

	var state string
	switch c.State {
	case "running":
		state = stateRunning.Render("● up")
	case "paused":
		state = statePaused.Render("◉ pause")
	default:
		state = stateStopped.Render("○ " + c.State)
	}

	cpuStr := lipgloss.NewStyle().Foreground(mutedColor).Render("  -  ")
	memPctStr := lipgloss.NewStyle().Foreground(mutedColor).Render("  -  ")
	memUseStr := lipgloss.NewStyle().Foreground(mutedColor).Render("   -   ")
	netStr := lipgloss.NewStyle().Foreground(mutedColor).Render("    -     ")

	if stats, ok := m.stats[c.ID]; ok {
		cpuStr = formatCPU(stats.CPUPercent)
		memPctStr = formatMemPercent(stats.MemoryPercent)
		memUseStr = formatBytes(stats.MemoryUsage)
		netStr = netStyle.Render(fmt.Sprintf("%s/%s",
			formatBytesShort(stats.NetworkRx), formatBytesShort(stats.NetworkTx)))
	}

	alertMark := m.getContainerAlertMark(c.ID)
	healthMark := m.getContainerHealthMark(c.HealthStatus)
	alertMark += healthMark

	hostID := truncate(c.HostID, 6)
	if hostID == "" {
		hostID = "local"
	}

	name := truncate(c.Name, 18)
	id := c.ID
	if len(id) > 12 {
		id = id[:12]
	}

	alignedCPU := lipgloss.NewStyle().Width(7).Render(cpuStr)
	alignedMemPct := lipgloss.NewStyle().Width(7).Render(memPctStr)
	alignedMemUse := lipgloss.NewStyle().Width(9).Render(memUseStr)
	alignedNet := lipgloss.NewStyle().Width(10).Render(netStr)

	return style.Render(fmt.Sprintf("%s%-6s %-12s %-20s %s %s %s %s %s%s",
		cursor, hostID, id, name, alignedCPU, alignedMemPct, alignedMemUse, alignedNet, state, alertMark))
}

func (m *Model) getContainerAlertMark(id string) string {
	alertMark := ""
	for _, a := range m.activeAlerts {
		if a.ContainerID == id {
			if a.Severity == "critical" {
				return " 🚨"
			}
			alertMark = " ⚠️"
		}
	}

	if alertMark != " 🚨" {
		for _, sa := range m.securityAlerts {
			if sa.ContainerID == id {
				if sa.Severity == "critical" {
					return " 🛡️ 🚨"
				}
				if alertMark == "" {
					alertMark = " 🛡️ ⚠️"
				}
			}
		}
	}
	return alertMark
}

func (m *Model) getContainerHealthMark(healthStatus string) string {
	switch healthStatus {
	case "healthy":
		return " ❤️"
	case "unhealthy":
		return " 🩺"
	case "starting":
		return " ⏳"
	}
	return ""
}

func (m Model) renderContainerDetail() string {
	if m.cursor >= len(m.containers) {
		return ""
	}
	c := m.containers[m.cursor]

	maxW := min(m.width-6, 72)
	if maxW < 40 {
		maxW = 40
	}

	detail := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(primaryColor).
		Padding(0, 1).MarginLeft(2).Width(maxW)

	var b strings.Builder
	b.WriteString(fmt.Sprintf(" %s  %s\n",
		lipgloss.NewStyle().Bold(true).Foreground(primaryColor).Render(m.msg.DetailsTitle),
		lipgloss.NewStyle().Foreground(mutedColor).Render(c.ID)))
	b.WriteString(fmt.Sprintf(" %-9s %s\n", m.msg.Name, c.Name))
	b.WriteString(fmt.Sprintf(" %-9s %s\n", m.msg.Image, c.Image))
	b.WriteString(fmt.Sprintf(" %-9s %s\n", m.msg.Status, c.Status))
	b.WriteString(fmt.Sprintf(" %-9s %s\n", m.msg.Command, c.Command))
	b.WriteString(fmt.Sprintf(" %-9s %s\n", m.msg.Created, c.Created))
	if len(c.Ports) > 0 {
		b.WriteString(fmt.Sprintf(" %-9s %s\n", m.msg.Ports, c.Ports))
	}
	if len(c.Networks) > 0 {
		b.WriteString(fmt.Sprintf(" %-9s %s\n", m.msg.Networks, strings.Join(c.Networks, ", ")))
	}

	m.appendSecurityAlerts(&b, c.ID, maxW)
	m.appendCommandAndMounts(&b, c)
	m.appendRestartAndLimits(&b, c)
	m.appendHealthAndEnv(&b, c)
	m.appendStats(&b, c)

	return detail.Render(b.String()) + "\n"
}

func (m Model) appendSecurityAlerts(b *strings.Builder, containerID string, maxW int) {
	var cSecAlerts []alerts.Alert
	for _, a := range m.securityAlerts {
		if a.ContainerID == containerID {
			cSecAlerts = append(cSecAlerts, a)
		}
	}

	if len(cSecAlerts) > 0 {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(warningColor).Render("\n 🛡️ "+m.msg.SecurityAudit) + "\n")
		for _, a := range cSecAlerts {
			icon := "⚠️"
			style := alertWarnStyle
			if a.Severity == "critical" {
				icon = "🚨"
				style = alertCritStyle
			}
			desc := a.RuleName
			switch a.RuleName {
			case "Sec: Privileged Mode":
				desc = m.msg.SecPrivileged
			case "Sec: Root User":
				desc = m.msg.SecRootUser
			case "Sec: DB Port Exposed globally":
				desc = m.msg.SecDBPort
			case "Sec: Sensitive CAP_ADD":
				desc = m.msg.SecSensitiveCap
			case "Sec: No Resource Quotas":
				desc = m.msg.SecNoResourceQuotas
			case "Sec: Writable RootFS":
				desc = m.msg.SecWritableRootFS
			case "Sec: Insecure Port Exposed":
				desc = m.msg.SecInsecurePort
			case "Sec: Missing No-New-Privileges":
				desc = m.msg.SecMissingNoNewPrivs
			case "Sec: Host Networking Mode":
				desc = m.msg.SecHostNetwork
			}
			wrappedStyle := style.Copy().Width(maxW - 4)
			b.WriteString(wrappedStyle.Render(fmt.Sprintf(" %s %s", icon, desc)) + "\n")
		}
	}
}

func (m Model) appendCommandAndMounts(b *strings.Builder, c logger.ContainerDisplay) {
	if c.Entrypoint != "" {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(secondaryColor).Render("\n ⚙️  Command") + "\n")
		b.WriteString(lipgloss.NewStyle().Foreground(primaryColor).Render(
			fmt.Sprintf("   %s", c.Entrypoint)) + "\n")
	}

	if len(c.Mounts) > 0 {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(secondaryColor).Render("\n 📂 Volumes / Mounts") + "\n")
		for i, mt := range c.Mounts {
			if i >= 8 {
				b.WriteString(lipgloss.NewStyle().Foreground(mutedColor).Render(
					fmt.Sprintf("   ... +%d more", len(c.Mounts)-8)) + "\n")
				break
			}
			b.WriteString(lipgloss.NewStyle().Foreground(mutedColor).Render(
				fmt.Sprintf("   %s", mt)) + "\n")
		}
	}
}

func (m Model) appendRestartAndLimits(b *strings.Builder, c logger.ContainerDisplay) {
	if c.RestartPolicy != "" {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(secondaryColor).Render("\n 🔄 Restart Policy") + "\n")
		policyStr := c.RestartPolicy
		if c.RestartCount > 0 {
			policyStr += lipgloss.NewStyle().Foreground(lipgloss.Color("#BF616A")).Render(
				fmt.Sprintf("  (crashed %dx)", c.RestartCount))
		}
		b.WriteString(fmt.Sprintf("   %s\n", policyStr))
	}

	cpuLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("#BF616A")).Render("unlimited ⚠")
	memLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("#BF616A")).Render("unlimited ⚠")
	if c.CPULimit > 0 {
		cpuLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("#A3BE8C")).Render(
			fmt.Sprintf("%.1f cores", c.CPULimit))
	}
	if c.MemoryLimit > 0 {
		memLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("#A3BE8C")).Render(
			formatBytes(uint64(c.MemoryLimit)))
	}
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(secondaryColor).Render("\n 🚧 Resource Limits") + "\n")
	b.WriteString(fmt.Sprintf("   CPU: %s   MEM: %s\n", cpuLabel, memLabel))
}

func (m Model) appendHealthAndEnv(b *strings.Builder, c logger.ContainerDisplay) {
	if c.HealthStatus != "" {
		healthIcon := "✅"
		healthColor := lipgloss.Color("#A3BE8C")
		switch c.HealthStatus {
		case "unhealthy":
			healthIcon = "⚠️ "
			healthColor = lipgloss.Color("#BF616A")
		case "starting":
			healthIcon = "⏳"
			healthColor = lipgloss.Color("#EBCB8B")
		}
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(healthColor).Render(
			fmt.Sprintf("\n %s Health: %s", healthIcon, c.HealthStatus)) + "\n")
		if c.HealthLog != "" {
			b.WriteString(lipgloss.NewStyle().Foreground(mutedColor).Render(
				fmt.Sprintf("   Last check: %s", c.HealthLog)) + "\n")
		}
	}

	if len(c.Env) > 0 {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(secondaryColor).Render("\n 🔎 Env Variables") + "\n")
		for i, env := range c.Env {
			if i >= 15 {
				b.WriteString(lipgloss.NewStyle().Foreground(mutedColor).Render(
					fmt.Sprintf("   ... +%d more", len(c.Env)-15)) + "\n")
				break
			}
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				key := lipgloss.NewStyle().Foreground(primaryColor).Render(parts[0])
				val := parts[1]
				if len(val) > 50 {
					val = val[:47] + "..."
				}
				b.WriteString(fmt.Sprintf("   %s=%s\n", key, val))
			} else {
				b.WriteString(fmt.Sprintf("   %s\n", env))
			}
		}
	}
}

func (m Model) appendStats(b *strings.Builder, c logger.ContainerDisplay) {
	if stats, ok := m.stats[c.ID]; ok {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(secondaryColor).Render("\n 📊 "+m.msg.Metrics) + "\n")
		b.WriteString(fmt.Sprintf(" CPU:      %s\n", formatCPU(stats.CPUPercent)))
		b.WriteString(fmt.Sprintf(" %-9s %s / %s (%s)\n",
			m.msg.Memory, formatBytes(stats.MemoryUsage), formatBytes(stats.MemoryLimit), formatMemPercent(stats.MemoryPercent)))
		b.WriteString(fmt.Sprintf(" %-9s %-12s %-9s %s\n",
			m.msg.NetworkDown, formatBytes(stats.NetworkRx), m.msg.NetworkUp, formatBytes(stats.NetworkTx)))
	}
}

func (m Model) renderLogPanel() string {
	names := strings.Join(m.logContainers, ", ")
	title := lipgloss.NewStyle().Bold(true).Foreground(secondaryColor).MarginLeft(2).
		Render(fmt.Sprintf("📜 "+m.msg.LogsTitle, names))

	maxLines := 10
	if m.height > 50 {
		maxLines = 15
	}

	filtered := m.filterAndFormatLogs()

	start := 0
	if len(filtered) > maxLines+m.logOffset {
		start = len(filtered) - maxLines - m.logOffset
	}

	end := start + maxLines
	if end > len(filtered) {
		end = len(filtered)
	}

	var b strings.Builder
	for _, entry := range filtered[start:end] {
		truncated := entry.Text
		if len(truncated) > m.width+50 {
			truncated = truncated[:m.width+50] + "..."
		}

		prefix := ""
		if len(m.logContainers) > 1 {
			prefix = lipgloss.NewStyle().Foreground(primaryColor).Render(fmt.Sprintf("[%s] ", entry.Container))
		}

		b.WriteString("  " + prefix + lipgloss.NewStyle().Foreground(lipgloss.Color("#A0AEC0")).Render(truncated) + "\n")
	}

	maxW := min(m.width-4, 90)
	if maxW < 40 {
		maxW = 40
	}

	footer := m.buildLogFooter()
	panel := logPanelStyle.Width(maxW).MarginLeft(2)
	return title + "\n" + panel.Render(b.String()) + footer
}

func (m Model) filterAndFormatLogs() []LogEntry {
	var filtered []LogEntry
	for _, entry := range m.logLines {
		if m.logSearch == "" || strings.Contains(strings.ToLower(entry.Text), strings.ToLower(m.logSearch)) {
			text := entry.Text

			if len(text) > 30 && text[4] == '-' && text[7] == '-' && text[10] == 'T' {
				ts := text[:30]
				rest := text[30:]
				text = lipgloss.NewStyle().Foreground(lipgloss.Color("#5C6370")).Render(ts) + rest
			}

			if strings.Contains(text, `":`) {
				text = strings.ReplaceAll(text, `"level":"error"`, lipgloss.NewStyle().Foreground(lipgloss.Color("#E06C75")).Render(`"level":"error"`))
				text = strings.ReplaceAll(text, `"level":"warn"`, lipgloss.NewStyle().Foreground(lipgloss.Color("#E5C07B")).Render(`"level":"warn"`))
				text = strings.ReplaceAll(text, `"level":"info"`, lipgloss.NewStyle().Foreground(lipgloss.Color("#61AFEF")).Render(`"level":"info"`))
			}

			filtered = append(filtered, LogEntry{Container: entry.Container, Text: text})
		}
	}
	return filtered
}

func (m Model) buildLogFooter() string {
	footer := ""
	if m.searchMode {
		footer = "\n  " + lipgloss.NewStyle().Foreground(primaryColor).Render("🔍 Search: "+m.logSearch+"█")
	} else if m.logSearch != "" {
		footer = "\n  " + lipgloss.NewStyle().Foreground(mutedColor).Render("Active filter: "+m.logSearch)
	}

	if m.logOffset > 0 {
		footer += lipgloss.NewStyle().Foreground(warningColor).Render(fmt.Sprintf("  [History offset: %d]", m.logOffset))
	}
	return footer
}

func (m Model) renderAlerts() string {
	var b strings.Builder

	allAlerts := append([]alerts.Alert{}, m.activeAlerts...)
	allAlerts = append(allAlerts, m.securityAlerts...)

	if len(allAlerts) == 0 {
		return ""
	}

	maxW := min(m.width-4, 80)
	if maxW < 40 {
		maxW = 40
	}

	for _, a := range allAlerts {
		msg := ""
		ruleName := a.RuleName

		if strings.HasPrefix(ruleName, "Sec:") {
			switch ruleName {
			case "Sec: Privileged Mode":
				ruleName = "Sec: " + m.msg.AlertSecPrivileged
			case "Sec: Root User":
				ruleName = "Sec: " + m.msg.AlertSecRootUser
			case "Sec: DB Port Exposed globally":
				ruleName = "Sec: " + m.msg.AlertSecDBPort
			case "Sec: Sensitive CAP_ADD":
				ruleName = "Sec: " + m.msg.AlertSecSensitiveCap
			case "Sec: No Resource Quotas":
				ruleName = "Sec: " + m.msg.AlertSecNoQuotas
			case "Sec: Writable RootFS":
				ruleName = "Sec: " + m.msg.AlertSecWritableFS
			case "Sec: Insecure Port Exposed":
				ruleName = "Sec: " + m.msg.AlertSecInsecurePort
			case "Sec: Missing No-New-Privileges":
				ruleName = "Sec: " + m.msg.AlertSecNoNewPrivs
			case "Sec: Host Networking Mode":
				ruleName = "Sec: " + m.msg.AlertSecHostNet
			}
			msg = fmt.Sprintf(" %s: %s [%s] ", ruleName, a.ContainerName, a.Severity)
		} else {
			msg = fmt.Sprintf(" %s: %s %.1f%% > %.1f%% [%s] ",
				ruleName, a.ContainerName, a.CurrentValue, a.Threshold, a.Severity)
		}

		if a.Severity == "critical" {
			b.WriteString("  " + alertCritStyle.Width(maxW).Render(" 🚨"+msg) + "\n")
		} else {
			b.WriteString("  " + alertWarnStyle.Width(maxW).Render(" ⚠️"+msg) + "\n")
		}
	}
	return b.String()
}

func (m Model) renderEventLog() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(secondaryColor).MarginLeft(2).
		Render("📋 " + m.msg.EventsTitle)

	if len(m.events) == 0 {
		return title + "\n" + lipgloss.NewStyle().Foreground(mutedColor).MarginLeft(4).
			Render(m.msg.WaitingEvents+"\n")
	}

	count := min(5, len(m.events))
	if m.height > 45 {
		count = min(8, len(m.events))
	}

	var b strings.Builder
	for i := 0; i < count; i++ {
		e := m.events[i]
		line := fmt.Sprintf("  %s %s %-10s %s",
			lipgloss.NewStyle().Foreground(mutedColor).Render(e.Time.Format("15:04:05")),
			e.Icon,
			lipgloss.NewStyle().Foreground(warningColor).Render(e.Action),
			e.Name)
		b.WriteString(line + "\n")
	}

	maxW := min(m.width-4, 80)
	if maxW < 40 {
		maxW = 40
	}
	return title + "\n" + eventPanelStyle.Width(maxW).MarginLeft(2).Render(b.String())
}

func (m Model) renderHelpBar() string {
	var bar string

	switch {
	case m.showLogs:
		bar = "  ↑↓ scroll │ / grep │ f tail │ E export │ Esc back"
	case m.showCleanup:
		bar = "  [i] images │ [v] volumes │ Esc back"
	case m.showMap:
		bar = "  M / Esc back"
	case m.showStress:
		bar = "  [c] CPU │ [m] mem │ [b] both │ Esc cancel"
	case m.showDetail:
		bar = "  x shell │ l log │ s stop │ R restart │ Esc back"
	case m.confirmAction != "":
		bar = "  y confirm │ Esc cancel"
	default:
		bar = "  ↑↓ nav │ space select │ enter details │ l log 1 │ L log N │ a all │ x shell │ C prune │ s stop │ R restart │ S stress │ M map │ ? help │ q quit"
	}

	if m.showHelp {
		legend := "\n  Icons: 🚨 Critical Alert │ ⚠️ Warning Alert │ 🛡️ Security Issue │ ❤️ Healthy │ 🩺 Unhealthy │ ⏳ Starting"
		return lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).BorderForeground(primaryColor).
			Padding(0, 1).MarginLeft(2).
			Render(bar+legend) + "\n"
	}

	promInfo := ""
	if m.cfg.Prometheus.Enabled {
		promInfo = fmt.Sprintf(" │ prometheus :%d", m.cfg.Prometheus.Port)
	}
	return helpStyle.MarginLeft(2).Render(bar+promInfo) + "\n"
}

// ─── Render Cleanup ─────────────────────────────────────────────────────────

func (m Model) renderCleanupPanel() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(" 🧹 Interactive Prune Dashboard "))
	b.WriteString("\n\n")

	if m.pruning {
		b.WriteString(lipgloss.NewStyle().Foreground(mutedColor).MarginLeft(4).Render("⏳ Processing Docker daemon cleanup... please wait."))
		b.WriteString("\n")
		return b.String()
	}

	if m.pruneFeedback != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#A3BE8C")).Bold(true).MarginLeft(4).Render(m.pruneFeedback))
		b.WriteString("\n\n")
	}

	imgColor := lipgloss.Color("#A3BE8C")              // green
	if m.diskUsage.ImagesReclaimable > 1024*1024*500 { // >500MB
		imgColor = lipgloss.Color("#EBCB8B") // yellow
	}
	if m.diskUsage.ImagesReclaimable > 1024*1024*1024*2 { // >2GB
		imgColor = lipgloss.Color("#BF616A") // red
	}

	volColor := lipgloss.Color("#A3BE8C") // green
	if m.diskUsage.VolumesReclaimable > 1024*1024*500 {
		volColor = lipgloss.Color("#EBCB8B")
	}
	if m.diskUsage.VolumesReclaimable > 1024*1024*1024*2 {
		volColor = lipgloss.Color("#BF616A")
	}

	b.WriteString("    📦 ")
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Dangling Images: "))
	b.WriteString(lipgloss.NewStyle().Foreground(imgColor).Render(formatBytes(uint64(m.diskUsage.ImagesReclaimable))))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(mutedColor).MarginLeft(7).Render("Untagged images not used by any container."))
	b.WriteString("\n\n")

	b.WriteString("    💾 ")
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Volumes Locais Ociosos:   "))
	b.WriteString(lipgloss.NewStyle().Foreground(volColor).Render(formatBytes(uint64(m.diskUsage.VolumesReclaimable))))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(mutedColor).MarginLeft(7).Render("Persistent volumes created by Docker not attached to any container."))
	b.WriteString("\n\n\n")

	b.WriteString("    " + lipgloss.NewStyle().Background(lipgloss.Color("#4C566A")).Foreground(lipgloss.Color("#ECEFF4")).Padding(0, 1).Render(" Actions: "))
	b.WriteString("  [i] Clean Images  │  [v] Clean Volumes  │  [ESC] Back")
	b.WriteString("\n")

	return b.String()
}

// ─── Commands (tea.Cmd) ──────────────────────────────────────────────────────

func (m Model) fetchContainers() tea.Cmd {
	return func() tea.Msg {
		containers, err := m.dockerClient.ListRunningContainersDetailed(m.ctx, m.showAll)
		if err != nil {
			return dockerErrorMsg{err: err}
		}

		// Adds HostID for local containers
		for i := range containers {
			containers[i].HostID = m.cfg.Cluster.HostID
			if containers[i].HostID == "" {
				containers[i].HostID = "local"
			}
		}

		// Merge with remote containers from the Receiver
		if m.receiver != nil {
			remotes := m.receiver.GetAllContainers()
			for _, r := range remotes {
				containers = append(containers, logger.ContainerDisplay{
					HostID: r.HostID,
					ID:     r.ID,
					Name:   r.Name,
					Image:  r.Image,
					Status: r.Status,
					State:  r.State,
					Ports:  r.Ports,
				})
			}
		}

		return containerListMsg{containers: containers}
	}
}

func (m Model) fetchStats() tea.Cmd {
	return func() tea.Msg {
		containers, err := m.dockerClient.ListRunningContainers(m.ctx, m.showAll)
		if err != nil {
			return dockerErrorMsg{err: err}
		}

		stats, err := m.dockerClient.GetAllContainerStats(m.ctx, containers)
		if err != nil {
			return dockerErrorMsg{err: err}
		}

		// Merge with remote stats from the Receiver
		if m.receiver != nil {
			remoteStats := m.receiver.GetAllMetrics()
			for _, rs := range remoteStats {
				stats[rs.ContainerID] = rs
			}
		}

		return statsMsg{stats: stats}
	}
}

func (m Model) watchDockerEvents() tea.Cmd {
	return func() tea.Msg {
		eventCh, errCh := m.dockerClient.WatchEvents(m.ctx)
		select {
		case <-m.ctx.Done():
			return nil
		case err := <-errCh:
			return dockerErrorMsg{err: err}
		case event, ok := <-eventCh:
			if !ok {
				return nil
			}
			return dockerEventMsg{event: event}
		}
	}
}

func (m Model) watchNextDockerEvent() tea.Cmd {
	return m.watchDockerEvents()
}

type hostStatsMsg struct {
	cpu float64
	mem float64
}

func (m Model) fetchHostStats() tea.Cmd {
	return func() tea.Msg {
		c, _ := cpu.Percent(0, false)
		var cpuVal float64
		if len(c) > 0 {
			cpuVal = c[0]
		}

		v, _ := mem.VirtualMemory()
		var memVal float64
		if v != nil {
			memVal = v.UsedPercent
		}

		return hostStatsMsg{cpu: cpuVal, mem: memVal}
	}
}

type diskUsageMsg struct {
	usage docker.SystemDiskUsage
}

type pruneResultMsg struct {
	reclaimed uint64
	target    string
	err       error
}

func (m Model) fetchDiskUsage() tea.Cmd {
	return func() tea.Msg {
		du, err := m.dockerClient.GetDiskUsage(m.ctx)
		if err != nil {
			return dockerErrorMsg{err: err}
		}
		return diskUsageMsg{usage: du}
	}
}

func (m Model) runPrune(target string) tea.Cmd {
	return func() tea.Msg {
		var reclaimed uint64
		var err error
		switch target {
		case "images":
			reclaimed, err = m.dockerClient.PruneImages(m.ctx)
		case "volumes":
			reclaimed, err = m.dockerClient.PruneVolumes(m.ctx)
		}
		return pruneResultMsg{reclaimed: reclaimed, target: target, err: err}
	}
}

func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) waitForNextLog(containerName string, logCh <-chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-logCh
		if !ok {
			return nil // stream fechado
		}
		return logLineMsg{
			container: containerName,
			line:      line,
			nextCh:    logCh,
		}
	}
}

func (m Model) executeAction(action, containerID, containerName string) tea.Cmd {
	return func() tea.Msg {
		var err error
		switch action {
		case "stop":
			err = m.dockerClient.StopContainer(m.ctx, containerID)
		case "restart":
			err = m.dockerClient.RestartContainer(m.ctx, containerID)
		}

		return actionResultMsg{
			success:   err == nil,
			action:    action,
			container: containerName,
			err:       err,
		}
	}
}

func (m Model) executeStress(mode string) tea.Cmd {
	return func() tea.Msg {
		err := m.dockerClient.RunStressTest(m.ctx, mode, 30)
		return stressResultMsg{
			success: err == nil,
			mode:    mode,
			err:     err,
		}
	}
}

func (m Model) executeShell() tea.Cmd {
	c := m.containers[m.cursor]
	cmd := exec.Command("docker", "exec", "-it", c.Name, "/bin/sh")
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			cmdBash := exec.Command("docker", "exec", "-it", c.Name, "/bin/bash")
			return tea.ExecProcess(cmdBash, func(err2 error) tea.Msg {
				if err2 != nil {
					return dockerErrorMsg{err: fmt.Errorf("shell exec failed (sh/bash): %v", err2)}
				}
				return nil
			})()
		}
		return nil
	})
}

// ─── Formatting ──────────────────────────────────────────────────────────────

func formatCPU(p float64) string {
	s := fmt.Sprintf("%.1f%%", p)
	if p >= 80 {
		return cpuHighStyle.Render(s)
	} else if p >= 40 {
		return cpuMedStyle.Render(s)
	}
	return cpuLowStyle.Render(s)
}

func formatMemPercent(p float64) string {
	s := fmt.Sprintf("%.1f%%", p)
	if p >= 80 {
		return memHighStyle.Render(s)
	} else if p >= 50 {
		return memMedStyle.Render(s)
	}
	return memLowStyle.Render(s)
}

func formatBytes(b uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.1fGB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.1fMB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1fKB", float64(b)/float64(KB))
	}
	return fmt.Sprintf("%dB", b)
}

func formatBytesShort(b uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.0fG", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.0fM", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.0fK", float64(b)/float64(KB))
	}
	return fmt.Sprintf("%dB", b)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ─── Run ─────────────────────────────────────────────────────────────────────

func Run(dockerClient *docker.Client, receiver metrics.ClusterProvider, ctx context.Context, sysInfo map[string]string, version string, cfg config.Config, store *storage.SQLiteStore) error {
	model := NewModel(dockerClient, receiver, ctx, sysInfo, version, cfg, store)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	// Handle graceful shutdown via context
	go func() {
		<-ctx.Done()
		p.Quit()
	}()

	_, err := p.Run()
	return err
}
