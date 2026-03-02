package i18n

type Messages struct {
	DetailsTitle         string
	Name                 string
	Image                string
	Status               string
	Command              string
	Created              string
	Ports                string
	Networks             string
	SecurityAudit        string
	Metrics              string
	Memory               string
	NetworkDown          string
	NetworkUp            string
	LogsTitle            string
	EventsTitle          string
	WaitingEvents        string
	HelpBar              string
	TableHost            string
	TableName            string
	TableState           string
	NoContainers         string
	ErrorSmallTerminal   string
	StressTestTitle      string
	StressTestMenu       string
	ConfirmStop          string
	ConfirmRestart       string
	ServiceMapTitle      string
	ServiceMapNoCustom   string
	ServiceMapBack       string
	SecPrivileged        string
	SecRootUser          string
	SecDBPort            string
	SecSensitiveCap      string
	SecNoResourceQuotas  string
	SecWritableRootFS    string
	SecInsecurePort      string
	SecMissingNoNewPrivs string
	SecHostNetwork       string
}

var En = Messages{
	DetailsTitle:         "Details",
	Name:                 "Name:",
	Image:                "Image:",
	Status:               "Status:",
	Command:              "Command:",
	Created:              "Created:",
	Ports:                "Ports:",
	Networks:             "Networks:",
	SecurityAudit:        "Security Audit",
	Metrics:              "Metrics",
	Memory:               "Memory:",
	NetworkDown:          "Net ↓:",
	NetworkUp:            "Net ↑:",
	LogsTitle:            "Logs: %s",
	EventsTitle:          "Events",
	WaitingEvents:        "Waiting for events...",
	HelpBar:              "  ↑/k Up │ ↓/j Down │ Enter Details │ l Logs │ s Stop │ R Restart │ S Stress Test │ M Service Map │ r Refresh │ ? Help │ q Quit  ",
	TableHost:            "HOST",
	TableName:            "NAME",
	TableState:           "STATE",
	NoContainers:         "No running containers",
	ErrorSmallTerminal:   "Terminal is too small (min 60 cols).",
	StressTestTitle:      "Stress Test — Spawn stress container (30s)",
	StressTestMenu:       "[c] CPU (2 cores @ 100%)\n  [m] Memory (256MB)\n  [b] Both\n  [Esc] Cancel",
	ConfirmStop:          "stop",
	ConfirmRestart:       "restart",
	ServiceMapTitle:      "Service & Network Map",
	ServiceMapNoCustom:   "No custom networks detected.",
	ServiceMapBack:       "[M / Esc] Back to Dashboard",
	SecPrivileged:        "Privileged Mode enabled (grants near full control over host)",
	SecRootUser:          "Container runs internal processes as root user",
	SecDBPort:            "Database port (e.g. 3306, 5432) widely exposed on 0.0.0.0. Phishing risk.",
	SecSensitiveCap:      "Sensitive Linux Capabilities injected (e.g. SYS_ADMIN/NET_ADMIN)",
	SecNoResourceQuotas:  "No CPU or Memory limits configured (Risk of Host DoS)",
	SecWritableRootFS:    "Root Filesystem is writable (Immutable readonly-rootfs is best practice)",
	SecInsecurePort:      "Insecure management ports (SSH:22 or Telnet:23) exposed",
	SecMissingNoNewPrivs: "Missing 'no-new-privileges' flag (Allows SUID escalation)",
	SecHostNetwork:       "Running with --network=host (Bypasses network isolation)",
}

var Pt = Messages{
	DetailsTitle:         "Detalhes",
	Name:                 "Nome:",
	Image:                "Imagem:",
	Status:               "Status:",
	Command:              "Comando:",
	Created:              "Criado:",
	Ports:                "Portas:",
	Networks:             "Redes:",
	SecurityAudit:        "Auditoria de Segurança",
	Metrics:              "Métricas",
	Memory:               "Memória:",
	NetworkDown:          "Rede ↓:",
	NetworkUp:            "Rede ↑:",
	LogsTitle:            "Logs: %s",
	EventsTitle:          "Eventos",
	WaitingEvents:        "Aguardando eventos...",
	HelpBar:              "  ↑/k Subir │ ↓/j Descer │ Enter Detalhes │ l Logs │ s Stop │ R Restart │ S Stress │ M Map │ r Refresh │ ? Ajuda │ q Sair  ",
	TableHost:            "HOST",
	TableName:            "NOME",
	TableState:           "ESTADO",
	NoContainers:         "Nenhum container em execução",
	ErrorSmallTerminal:   "Terminal muito pequeno (min 60 cols).",
	StressTestTitle:      "Stress Test — Criar container de stress (30s)",
	StressTestMenu:       "[c] CPU (2 cores a 100%)\n  [m] Memória (256MB)\n  [b] Ambos\n  [Esc] Cancelar",
	ConfirmStop:          "parar",
	ConfirmRestart:       "reiniciar",
	ServiceMapTitle:      "Mapa de Serviços e Redes",
	ServiceMapNoCustom:   "Nenhuma rede customizada detectada.",
	ServiceMapBack:       "[M / Esc] Voltar ao Dashboard",
	SecPrivileged:        "Modo Privilegiado habilitado (dá controle quase total sobre o host)",
	SecRootUser:          "Container está rodando seus processos internos com o usuário root",
	SecDBPort:            "Porta de Banco de Dados (ex: 3306, 5432) exposta sem restrições em 0.0.0.0. Perigoso para invasões.",
	SecSensitiveCap:      "Capabilities Linux muito sensíveis foram injetadas (ex: SYS_ADMIN/NET_ADMIN)",
	SecNoResourceQuotas:  "Container sem limite de Memória ou CPU configurado (Risco de DoS no Host)",
	SecWritableRootFS:    "Root Filesystem aberto para escrita (O ideal é rodar containers imutáveis c/ readonly-rootfs)",
	SecInsecurePort:      "Portas de gerenciamento inseguras (SSH:22 ou Telnet:23) expostas pelo container",
	SecMissingNoNewPrivs: "Falta a flag 'no-new-privileges' (Permite escalonamento via binários SUID nativos do Linux)",
	SecHostNetwork:       "Rodando em modo --network=host (Isolamento de rede quebrado com o sistema principal)",
}

// Get retorna as strings carregadas baseada na lang env (en, pt).
func Get(lang string) Messages {
	if lang == "pt" || lang == "pt-BR" {
		return Pt
	}
	return En // Default to English
}
