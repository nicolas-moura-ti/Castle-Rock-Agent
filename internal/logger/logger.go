// Package logger fornece um sistema de logging estruturado e colorido
// para o Castle Rock Agent.
//
// ARQUITETURA DE LOGGING:
//
//	Este package utiliza log/slog (Go 1.21+), o logger estruturado da
//	biblioteca padrão do Go. slog é a evolução do antigo log package,
//	oferecendo:
//
//	- Logs estruturados com key-value pairs (não apenas strings)
//	- Níveis de log nativos (DEBUG, INFO, WARN, ERROR)
//	- Handlers customizáveis (JSON para produção, texto colorido para dev)
//	- Performance otimizada com lazy evaluation de argumentos
//	- Integração nativa com context.Context
//
// POR QUE SLOG EM VEZ DE LOGRUS/ZAP?
//   - slog é parte da standard library — sem dependência externa
//   - API estável e mantida pelo time core do Go
//   - Performance comparável ao uber-go/zap
//   - Em projetos novos, a recomendação da comunidade Go é usar slog
//
// CORES NO TERMINAL:
//
//	Usamos códigos ANSI escape para colorir o output no terminal.
//	Isso é uma prática comum em CLIs profissionais e melhora
//	drasticamente a legibilidade dos logs durante desenvolvimento.
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
// CONSTANTES DE COR ANSI
// ─────────────────────────────────────────────────────────────────────────────
//
// Códigos ANSI são sequências de escape interpretadas pelo terminal
// para alterar a cor e o estilo do texto. O formato é: \033[<código>m
//
// Referência:
//   - 0 = Reset (volta ao padrão)
//   - 1 = Bold (negrito)
//   - 2 = Dim (opaco)
//   - 3x = Foreground colors (30-37)
//   - 9x = Bright foreground colors (90-97)
const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
	colorItalic = "\033[3m"

	// Cores de foreground
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorBlue    = "\033[34m"
	colorMagenta = "\033[35m"
	colorCyan    = "\033[36m"
	colorWhite   = "\033[37m"

	// Cores bright (mais vibrantes)
	colorBrightRed    = "\033[91m"
	colorBrightGreen  = "\033[92m"
	colorBrightYellow = "\033[93m"
	colorBrightBlue   = "\033[94m"
	colorBrightCyan   = "\033[96m"
	colorBrightWhite  = "\033[97m"

	// Background colors
	colorBgRed    = "\033[41m"
	colorBgGreen  = "\033[42m"
	colorBgYellow = "\033[43m"
	colorBgBlue   = "\033[44m"
)

// ─────────────────────────────────────────────────────────────────────────────
// PRETTY HANDLER (Handler Customizado para slog)
// ─────────────────────────────────────────────────────────────────────────────
//
// slog.Handler é uma interface que define COMO os logs são formatados e
// escritos. O Go fornece dois handlers built-in:
//   - slog.TextHandler — output key=value (para humanos)
//   - slog.JSONHandler — output JSON (para sistemas de log como ELK/Loki)
//
// Nós criamos um handler customizado (PrettyHandler) que adiciona:
//   - Cores ANSI para cada nível de log
//   - Timestamp formatado com milissegundos
//   - Separadores visuais para melhor legibilidade
//   - Alinhamento de campos para output tabular
//
// IMPLEMENTAR A INTERFACE slog.Handler:
//   Para criar um handler customizado, precisamos implementar:
//   - Enabled(ctx, level) bool — determina se o nível deve ser logado
//   - Handle(ctx, record) error — formata e escreve o log
//   - WithAttrs(attrs) Handler — cria handler com atributos pré-definidos
//   - WithGroup(name) Handler — cria handler com grupo de atributos

// PrettyHandler é nosso handler customizado que produz logs coloridos
// e formatados para o terminal.
type PrettyHandler struct {
	// mu protege escritas concorrentes no writer.
	// Em Go, múltiplas goroutines podem logar simultaneamente,
	// então precisamos de um mutex para evitar output corrompido.
	//
	// NOTA: sync.Mutex é preferível a channels para proteção simples
	// de recursos compartilhados. Channels são para comunicação entre
	// goroutines; mutexes são para exclusão mútua.
	mu sync.Mutex

	// w é o writer de destino (normalmente os.Stderr).
	w io.Writer

	// level é o nível mínimo de log a ser exibido.
	level slog.Level

	// attrs são atributos pré-definidos adicionados a cada log.
	attrs []slog.Attr

	// group é o prefixo de grupo atual.
	group string
}

// PrettyHandlerOptions configura o PrettyHandler.
//
// PADRÃO GO — Options struct:
//
//	Em vez de passar muitos parâmetros para o construtor, usamos uma
//	struct de opções. Isso permite:
//	- Valores default para campos não preenchidos (zero values do Go)
//	- Adicionar novas opções sem quebrar a API existente
//	- Documentação clara de cada opção
type PrettyHandlerOptions struct {
	// Level define o nível mínimo de log.
	// Default: slog.LevelInfo
	Level slog.Level
}

// NewPrettyHandler cria um novo PrettyHandler que escreve logs
// coloridos e formatados para o writer especificado.
//
// Exemplo de uso:
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

// Enabled reporta se o handler processa logs neste nível.
//
// slog chama este método ANTES de construir o Record, evitando
// alocações desnecessárias para logs que serão descartados.
// Isso é uma otimização de performance importante.
func (h *PrettyHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

// Handle formata e escreve um registro de log com cores ANSI.
//
// Este é o coração do handler — aqui decidimos como cada log aparece
// no terminal. O design prioriza legibilidade e scannability:
//   - Timestamp com milissegundos para correlação precisa
//   - Nível colorido e com largura fixa para alinhamento
//   - Mensagem em destaque (bold)
//   - Atributos formatados como key=value com cores diferenciadas
func (h *PrettyHandler) Handle(_ context.Context, r slog.Record) error {
	// Formata o timestamp com milissegundos.
	// O formato "15:04:05.000" segue a convenção Go de reference time:
	// Mon Jan 2 15:04:05 MST 2006 (o "momento de referência" do Go).
	timeStr := r.Time.Format("2006-01-02 15:04:05.000")

	// Determina a cor e o label baseado no nível de log.
	levelStr, levelColor := h.formatLevel(r.Level)

	// Constrói a linha de log com cores ANSI.
	var b strings.Builder

	// Timestamp em dim para não competir visualmente com a mensagem
	fmt.Fprintf(&b, "%s%s%s ", colorDim, timeStr, colorReset)

	// Nível com cor e largura fixa (5 chars) para alinhamento
	fmt.Fprintf(&b, "%s%-5s%s ", levelColor, levelStr, colorReset)

	// Separador vertical para clareza visual
	fmt.Fprintf(&b, "%s│%s ", colorDim, colorReset)

	// Mensagem principal em bold
	fmt.Fprintf(&b, "%s%s%s", colorBold, r.Message, colorReset)

	// Adiciona atributos pré-definidos (do WithAttrs)
	for _, attr := range h.attrs {
		h.appendAttr(&b, attr)
	}

	// Adiciona atributos do Record (passados na chamada de log)
	r.Attrs(func(a slog.Attr) bool {
		h.appendAttr(&b, a)
		return true // continua iterando
	})

	b.WriteString("\n")

	// Mutex para escritas thread-safe.
	// Sem isso, logs de múltiplas goroutines poderiam se misturar.
	h.mu.Lock()
	defer h.mu.Unlock()

	_, err := h.w.Write([]byte(b.String()))
	return err
}

// WithAttrs retorna um novo handler com atributos adicionais.
//
// Este método implementa o padrão de "logger contextual":
// permite criar loggers especializados que sempre incluem
// certos campos. Exemplo:
//
//	dockerLogger := logger.With("component", "docker")
//	dockerLogger.Info("connected") // sempre inclui component=docker
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

// WithGroup retorna um novo handler com um prefixo de grupo.
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

// formatLevel retorna o label e a cor para cada nível de log.
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

// appendAttr formata e adiciona um atributo ao builder.
func (h *PrettyHandler) appendAttr(b *strings.Builder, a slog.Attr) {
	// Ignora atributos com valor vazio.
	if a.Equal(slog.Attr{}) {
		return
	}

	key := a.Key
	if h.group != "" {
		key = h.group + "." + key
	}

	// Key em cyan, value em branco para diferenciação visual
	fmt.Fprintf(b, " %s%s%s=%s%v%s",
		colorCyan, key, colorReset,
		colorWhite, a.Value, colorReset,
	)
}

// ─────────────────────────────────────────────────────────────────────────────
// INICIALIZAÇÃO GLOBAL
// ─────────────────────────────────────────────────────────────────────────────

// Setup configura o logger global da aplicação.
//
// PADRÃO GO — slog.SetDefault:
//
//	slog.SetDefault define o logger padrão usado por slog.Info(),
//	slog.Error(), etc. Isso permite que qualquer package use logging
//	sem precisar receber o logger como parâmetro.
//
//	Em projetos maiores, considere injeção de dependência (passar
//	*slog.Logger como parâmetro). Para o MVP, o logger global é aceitável.
func Setup(level string) *slog.Logger {
	// Converte string de nível para slog.Level
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
// FUNÇÕES DE DISPLAY (Output Formatado)
// ─────────────────────────────────────────────────────────────────────────────
//
// Estas funções não são logs estruturados — são output visual formatado
// para o terminal. Usamos fmt.Fprintf para output direto em vez de slog
// porque são displays de dados, não eventos de log.

// PrintBanner exibe o banner de inicialização do agente.
//
// Um banner profissional comunica identidade e versão do software.
// Em produção, isso ajuda a identificar qual versão está rodando
// quando se faz troubleshooting em logs históricos.
func PrintBanner(version, goVersion string) {
	banner := fmt.Sprintf(`
%s%s┌─────────────────────────────────────────────────────────┐%s
%s%s│%s    🏰  %s%sCastle Rock Agent%s                                %s%s│%s
%s%s│%s    Observabilidade Docker em tempo real                %s%s│%s
%s%s├─────────────────────────────────────────────────────────┤%s
%s%s│%s  %sVersão:%s    %-46s %s%s│%s
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

// PrintDockerInfo exibe informações do Docker daemon em formato de tabela.
func PrintDockerInfo(info map[string]string) {
	fmt.Fprintf(os.Stderr, "\n%s%s  ⚙  Docker Engine%s\n", colorBold, colorMagenta, colorReset)
	fmt.Fprintf(os.Stderr, "%s  ───────────────────────────────────────────────%s\n", colorDim, colorReset)

	// Ordem definida para consistência visual
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

// PrintContainerTable exibe containers em formato de tabela colorida.
func PrintContainerTable(containers []ContainerDisplay) {
	if len(containers) == 0 {
		fmt.Fprintf(os.Stderr, "\n%s  📭 Nenhum container em execução encontrado%s\n\n", colorDim, colorReset)
		return
	}

	fmt.Fprintf(os.Stderr, "\n%s%s  🐳  Containers em Execução (%d)%s\n",
		colorBold, colorGreen, len(containers), colorReset)
	fmt.Fprintf(os.Stderr, "%s  ═══════════════════════════════════════════════════════════════════%s\n",
		colorDim, colorReset)

	for i, c := range containers {
		// Estado com cor
		stateColor := colorBrightGreen
		stateIcon := "●"
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

		// Dados do container
		printField("ID", c.ID)
		printField("Imagem", c.Image)
		printField("Status", c.Status)
		printField("Estado", fmt.Sprintf("%s%s%s", stateColor, c.State, colorReset))
		if c.Command != "" {
			printField("Comando", c.Command)
		}
		if c.Ports != "" {
			printField("Portas", c.Ports)
		}
		if c.Created != "" {
			printField("Criado", c.Created)
		}
		if len(c.Networks) > 0 {
			printField("Redes", strings.Join(c.Networks, ", "))
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
			printField("Tamanho (RW)", c.SizeRw)
		}

		fmt.Fprintf(os.Stderr, "  %s└──────────────────────────────────────────────────────────────┘%s\n",
			colorDim, colorReset)
	}

	fmt.Fprintln(os.Stderr)
}

// printField imprime um campo formatado dentro da tabela de container.
func printField(key, value string) {
	fmt.Fprintf(os.Stderr, "  %s│%s  %s%-14s%s %s%s%s\n",
		colorDim, colorReset,
		colorCyan, key+":", colorReset,
		colorWhite, value, colorReset,
	)
}

// PrintShutdown exibe a mensagem de encerramento.
func PrintShutdown() {
	fmt.Fprintf(os.Stderr, "\n%s%s  🛑  Encerrando Castle Rock Agent...%s\n", colorBold, colorRed, colorReset)
	fmt.Fprintf(os.Stderr, "%s  ───────────────────────────────────────────────%s\n", colorDim, colorReset)
}

// PrintUptime exibe o tempo de execução do agente.
func PrintUptime(startTime time.Time) {
	uptime := time.Since(startTime).Round(time.Second)
	fmt.Fprintf(os.Stderr, "  %sUptime:%s          %s%s%s\n",
		colorCyan, colorReset,
		colorWhite, uptime, colorReset,
	)
}

// ContainerDisplay é um DTO usado pelo logger para exibição detalhada.
// Separado de models.ContainerInfo para manter o model limpo.
type ContainerDisplay struct {
	ID       string
	Name     string
	Image    string
	Status   string
	State    string
	Command  string
	Ports    string
	Created  string
	Networks []string
	Labels   map[string]string
	SizeRw   string
}
