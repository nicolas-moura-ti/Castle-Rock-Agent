// Package tui implementa a interface interativa do Castle Rock Agent
// usando o framework Bubble Tea (charmbracelet/bubbletea).
//
// BUBBLE TEA — ARQUITETURA ELM:
//
//	Model → Update → View (ciclo unidirecional)
//
// FEATURES:
//   - Tabela de containers com métricas em tempo real (CPU/MEM/NET)
//   - Log de eventos Docker em tempo real
//   - Detalhes expandidos do container selecionado
//   - Logs streaming do container (toggle com 'l')
//   - Container actions: stop ('s'), restart ('R') com confirmação
//   - Sistema de alertas com indicador visual
//   - Atalhos: ↑↓/jk navegar, enter detalhes, l logs, s stop, R restart, ? help, q sair
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/nicolas-moura-ti/castle-rock-agent/internal/alerts"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/cluster"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/config"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/docker"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/i18n"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/logger"
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
type alertsMsg struct{ alerts []alerts.Alert }
type logLineMsg struct{ line string }
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
	logLines     []string
	showLogs     bool
	logContainer string // container cujos logs estão sendo exibidos

	// Container actions
	confirmAction string // "stop" ou "restart" — vazio = sem confirmação pendente

	// Stress test
	showStress bool // mostra o menu de stress test

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
	receiver     *cluster.Receiver
	store        *storage.SQLiteStore
	ctx          context.Context
	cfg          config.Config
	eventCount   int
	lastUpdate   time.Time
	quitting     bool
	msg          i18n.Messages
}

func NewModel(dockerClient *docker.Client, receiver *cluster.Receiver, ctx context.Context, sysInfo map[string]string, version string, cfg config.Config, store *storage.SQLiteStore) Model {
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
		logLines:       []string{},
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
		m.watchDockerEvents(),
		m.tickCmd(),
	)
}

// ─── Update ──────────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		// Se tem confirmação pendente, trata primeiro
		if m.confirmAction != "" {
			switch msg.String() {
			case "y", "Y":
				action := m.confirmAction
				m.confirmAction = ""
				if m.cursor < len(m.containers) {
					c := m.containers[m.cursor]
					return m, m.executeAction(action, c.ID, c.Name)
				}
			default:
				// Qualquer outra tecla cancela
				m.confirmAction = ""
			}
			return m, nil
		}

		// Menu de stress test
		if m.showStress {
			switch msg.String() {
			case "c":
				m.showStress = false
				return m, m.executeStress("cpu")
			case "m":
				m.showStress = false
				return m, m.executeStress("memory")
			case "b":
				m.showStress = false
				return m, m.executeStress("both")
			default:
				// Qualquer outra tecla cancela
				m.showStress = false
			}
			return m, nil
		}

		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			if m.showLogs {
				m.showLogs = false
				m.logLines = nil
			}
		case "down", "j":
			if m.cursor < len(m.containers)-1 {
				m.cursor++
			}
			if m.showLogs {
				m.showLogs = false
				m.logLines = nil
			}
		case "r":
			return m, tea.Batch(m.fetchContainers(), m.fetchStats())
		case "?":
			m.showHelp = !m.showHelp
		case "enter":
			m.showDetail = !m.showDetail
		case "esc":
			m.showDetail = false
			m.showHelp = false
			m.showLogs = false
			m.logLines = nil
			m.confirmAction = ""
			m.showStress = false
			m.showMap = false
		case "l":
			// Toggle logs do container selecionado
			if m.showLogs {
				m.showLogs = false
				m.logLines = nil
			} else if m.cursor < len(m.containers) {
				m.showLogs = true
				m.logLines = []string{"Carregando logs..."}
				m.logContainer = m.containers[m.cursor].Name
				return m, m.streamLogs(m.containers[m.cursor].ID)
			}
		case "s":
			// Stop container — pede confirmação
			if m.cursor < len(m.containers) {
				m.confirmAction = "stop"
			}
		case "R":
			// Restart container — pede confirmação (R maiúsculo)
			if m.cursor < len(m.containers) {
				m.confirmAction = "restart"
			}
		case "S":
			// Abre menu de stress test
			m.showStress = true
		case "m", "M":
			// Toggle Service Map (Redes)
			m.showMap = !m.showMap
			if m.showMap {
				edges, err := m.mapper.BuildMap(m.ctx)
				if err == nil {
					m.mapData = edges
				}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case containerListMsg:
		m.containers = msg.containers
		m.lastUpdate = time.Now()
		if m.cursor >= len(m.containers) && len(m.containers) > 0 {
			m.cursor = len(m.containers) - 1
		}
		if m.auditor != nil {
			oldAlertsCount := len(m.securityAlerts)
			m.securityAlerts = m.auditor.Audit(m.ctx, m.containers)

			// Simple notification logic if new alerts appeared (just an event log entry)
			if len(m.securityAlerts) > oldAlertsCount {
				m.events = append([]EventLogEntry{{
					Time:   time.Now(),
					Icon:   "🛡️ ",
					Action: "SEC-AUDIT",
					Name:   fmt.Sprintf("%d issues found", len(m.securityAlerts)),
				}}, m.events...)
			}
		}

	case statsMsg:
		m.stats = msg.stats
		// Avalia alertas
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

	case dockerEventMsg:
		m.eventCount++
		icon := "📋"
		switch msg.event.Action {
		case "start":
			icon = "🟢"
		case "stop":
			icon = "🟡"
		case "die":
			icon = "🔴"
		case "create":
			icon = "📦"
		case "destroy":
			icon = "🗑️ "
		case "pause":
			icon = "⏸️ "
		case "unpause":
			icon = "▶️ "
		}
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

	case dockerErrorMsg:
		m.events = append([]EventLogEntry{{
			Time: time.Now(), Icon: "❌", Action: "error", Name: msg.err.Error(),
		}}, m.events...)

	case logLineMsg:
		m.logLines = append(m.logLines, msg.line)
		if len(m.logLines) > 200 {
			m.logLines = m.logLines[len(m.logLines)-200:]
		}

	case actionResultMsg:
		icon := "✅"
		action := msg.action
		if !msg.success {
			icon = "❌"
			action = msg.action + " FAILED"
		}
		m.events = append([]EventLogEntry{{
			Time: time.Now(), Icon: icon, Action: action, Name: msg.container,
		}}, m.events...)
		// Re-fetch após ação
		return m, tea.Batch(m.fetchContainers(), m.fetchStats())

	case stressResultMsg:
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

	case tickMsg:
		return m, tea.Batch(m.tickCmd(), m.fetchStats())
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

	// Menu de stress test
	if m.showStress {
		b.WriteString(m.renderHeader())
		b.WriteString("\n\n")
		b.WriteString(stressStyle.Render("  ⚡ " + m.msg.StressTestTitle + "  "))
		b.WriteString("\n\n")
		b.WriteString(lipgloss.NewStyle().MarginLeft(4).Render(m.msg.StressTestMenu + "\n"))
		return b.String()
	}

	// Confirmação de ação
	if m.confirmAction != "" && m.cursor < len(m.containers) {
		b.WriteString(m.renderHeader())
		b.WriteString("\n\n")
		c := m.containers[m.cursor]
		b.WriteString(confirmStyle.Render(
			fmt.Sprintf("  ⚠️  %s container '%s'? (y = confirmar, qualquer tecla = cancelar)  ",
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
	header := fmt.Sprintf("  %-3s %-6s %-12s %-20s %-7s %-7s %-9s %-10s %s",
		"", m.msg.TableHost, "ID", m.msg.TableName, "CPU%", "MEM%", "MEM", "NET", m.msg.TableState)
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n")

	for i, c := range m.containers {
		cursor := "  "
		style := normalStyle
		if i == m.cursor {
			cursor = "▸ "
			style = selectedStyle
		}

		state := c.State
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

		// Indicador de alerta no container
		alertMark := ""
		for _, a := range m.activeAlerts {
			if a.ContainerID == c.ID {
				if a.Severity == "critical" {
					alertMark = " 🚨"
				} else {
					alertMark = " ⚠️"
				}
				break
			}
		}
		// Indicador de segurança caso ainda não tenha critical
		if alertMark != " 🚨" {
			for _, sa := range m.securityAlerts {
				if sa.ContainerID == c.ID {
					if sa.Severity == "critical" {
						alertMark = " 🛡️ 🚨"
					} else if alertMark == "" {
						alertMark = " 🛡️ ⚠️"
					}
					break
				}
			}
		}

		hostID := truncate(c.HostID, 6)
		if hostID == "" {
			hostID = "local"
		}

		name := truncate(c.Name, 18)
		id := c.ID
		if len(id) > 12 {
			id = id[:12]
		}

		line := fmt.Sprintf("%s%-6s %-12s %-20s %-7s %-7s %-9s %-10s %s%s",
			cursor, hostID, id, name, cpuStr, memPctStr, memUseStr, netStr, state, alertMark)

		b.WriteString(style.Render(line))
		b.WriteString("\n")
	}

	return b.String()
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

	var cSecAlerts []alerts.Alert
	for _, a := range m.securityAlerts {
		if a.ContainerID == c.ID {
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

	if stats, ok := m.stats[c.ID]; ok {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(secondaryColor).Render("\n 📊 "+m.msg.Metrics) + "\n")
		b.WriteString(fmt.Sprintf(" CPU:      %s\n", formatCPU(stats.CPUPercent)))
		b.WriteString(fmt.Sprintf(" %-9s %s / %s (%s)\n",
			m.msg.Memory, formatBytes(stats.MemoryUsage), formatBytes(stats.MemoryLimit), formatMemPercent(stats.MemoryPercent)))
		b.WriteString(fmt.Sprintf(" %-9s %-12s %-9s %s\n",
			m.msg.NetworkDown, formatBytes(stats.NetworkRx), m.msg.NetworkUp, formatBytes(stats.NetworkTx)))
	}

	return detail.Render(b.String()) + "\n"
}

func (m Model) renderLogPanel() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(secondaryColor).MarginLeft(2).
		Render(fmt.Sprintf("📜 "+m.msg.LogsTitle, m.logContainer))

	maxLines := 10
	if m.height > 50 {
		maxLines = 15
	}

	start := 0
	if len(m.logLines) > maxLines {
		start = len(m.logLines) - maxLines
	}

	var b strings.Builder
	for _, line := range m.logLines[start:] {
		truncated := line
		if len(truncated) > m.width-10 {
			truncated = truncated[:m.width-13] + "..."
		}
		b.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color("#A0AEC0")).Render(truncated) + "\n")
	}

	maxW := min(m.width-4, 90)
	if maxW < 40 {
		maxW = 40
	}
	panel := logPanelStyle.Width(maxW).MarginLeft(2)
	return title + "\n" + panel.Render(b.String())
}

func (m Model) renderAlerts() string {
	var b strings.Builder

	allAlerts := append([]alerts.Alert{}, m.activeAlerts...)
	allAlerts = append(allAlerts, m.securityAlerts...)

	for _, a := range allAlerts {
		msg := ""
		if strings.HasPrefix(a.RuleName, "Sec:") {
			msg = fmt.Sprintf(" %s: %s [%s] ", a.RuleName, a.ContainerName, a.Severity)
		} else {
			msg = fmt.Sprintf(" %s: %s %.1f%% > %.1f%% [%s] ",
				a.RuleName, a.ContainerName, a.CurrentValue, a.Threshold, a.Severity)
		}

		if a.Severity == "critical" {
			b.WriteString("  " + alertCritStyle.Render("🚨"+msg) + "\n")
		} else {
			b.WriteString("  " + alertWarnStyle.Render("⚠️"+msg) + "\n")
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
	if m.showHelp {
		return lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).BorderForeground(primaryColor).
			Padding(0, 1).MarginLeft(2).
			Render(m.msg.HelpBar) + "\n"
	}
	promInfo := ""
	if m.cfg.Prometheus.Enabled {
		promInfo = fmt.Sprintf(" │ prometheus :%d", m.cfg.Prometheus.Port)
	}
	// Usamos uma versão encurtada do HelpBar normal ou o próprio HelpBar truncado
	return helpStyle.MarginLeft(2).Render(
		m.msg.HelpBar+promInfo,
	) + "\n"
}

// ─── Comandos (tea.Cmd) ──────────────────────────────────────────────────────

func (m Model) fetchContainers() tea.Cmd {
	return func() tea.Msg {
		containers, err := m.dockerClient.ListRunningContainersDetailed(m.ctx)
		if err != nil {
			return dockerErrorMsg{err: err}
		}

		// Adiciona HostID local
		for i := range containers {
			containers[i].HostID = m.cfg.Cluster.HostID
			if containers[i].HostID == "" {
				containers[i].HostID = "local"
			}
		}

		// Funde com containers remotos do Receiver
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
		stats, err := m.dockerClient.GetAllContainerStats(m.ctx)
		if err != nil {
			return dockerErrorMsg{err: err}
		}

		// Funde com stats remotos do Receiver
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

func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) streamLogs(containerID string) tea.Cmd {
	return func() tea.Msg {
		logCh, err := m.dockerClient.StreamContainerLogs(m.ctx, containerID)
		if err != nil {
			return dockerErrorMsg{err: err}
		}
		// Lê a primeira linha para iniciar o stream
		line, ok := <-logCh
		if !ok {
			return logLineMsg{line: "(sem logs)"}
		}

		// Continua lendo em background
		go func() {
			for l := range logCh {
				_ = l // As próximas linhas serão lidas via polling
			}
		}()

		return logLineMsg{line: line}
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

// ─── Formatação ──────────────────────────────────────────────────────────────

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

func Run(dockerClient *docker.Client, receiver *cluster.Receiver, ctx context.Context, sysInfo map[string]string, version string, cfg config.Config, store *storage.SQLiteStore) error {
	model := NewModel(dockerClient, receiver, ctx, sysInfo, version, cfg, store)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}
