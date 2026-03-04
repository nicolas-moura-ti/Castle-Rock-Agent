// Package docker fornece um wrapper sobre a SDK oficial do Docker.
//
// Este package encapsula toda a interação com o Docker daemon, seguindo
// o princípio de Inversão de Dependência: o código de negócio (collector)
// não depende diretamente da SDK do Docker, mas sim de abstrações
// definidas neste package.
//
// Por que usar um wrapper?
//   - Facilita testes unitários (pode-se mockar a interface)
//   - Centraliza configuração e tratamento de erros
//   - Permite trocar a implementação sem afetar o resto do sistema
//   - Encapsula detalhes da API do Docker (versionamento, autenticação)
package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	// SDK oficial do Docker — esta é a biblioteca mantida pela Docker Inc.
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"

	"github.com/nicolas-moura-ti/castle-rock-agent/internal/logger"
	"github.com/nicolas-moura-ti/castle-rock-agent/pkg/models"
)

// Client encapsula o client oficial do Docker, adicionando métodos
// de alto nível específicos para o Castle Rock Agent.
//
// Por que não usar o client.Client diretamente?
//   - O client.Client tem dezenas de métodos que não precisamos
//   - Nosso Client expõe apenas a interface mínima necessária
//   - Segue o princípio de Interface Segregation (SOLID)
type Client struct {
	// cli é o client real da SDK do Docker.
	// Mantemos como campo não-exportado (minúsculo) para encapsulamento.
	cli *client.Client

	// includeContainers is a list of container names/substrings to monitor.
	includeContainers []string
}

// NewClient cria uma nova instância do Client conectada ao Docker daemon local.
//
// Usa client.NewClientWithOpts com as seguintes opções:
//   - FromEnv(): lê configurações de variáveis de ambiente (DOCKER_HOST, etc.)
//   - WithAPIVersionNegotiation(): negocia automaticamente a versão da API
//     com o daemon, evitando erros de incompatibilidade de versão.
//
// PADRÃO GO: Funções construtoras são nomeadas New<Tipo> por convenção.
// Elas retornam (*Tipo, error) — sempre verifique o erro no chamador.
func NewClient() (*Client, error) {
	// client.NewClientWithOpts aceita um número variável de opções
	// (variadic function), permitindo configuração flexível.
	//
	// WithAPIVersionNegotiation é ESSENCIAL em produção:
	//   - Sem ela, o client usa uma versão fixa da API
	//   - Se o daemon tiver uma versão diferente, todas as chamadas falham
	//   - Com negociação, o client descobre a versão do daemon e se adapta
	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		// Usamos fmt.Errorf com o verbo %w para "wrapping" de erros.
		// %w preserva o erro original, permitindo que o chamador use
		// errors.Is() e errors.As() para inspecionar a causa raiz.
		// Isso é fundamental para debugging em produção.
		return nil, fmt.Errorf("docker.NewClient: falha ao criar client: %w", err)
	}

	return &Client{cli: cli}, nil
}

// SetIncludeContainers configuration on the client.
func (c *Client) SetIncludeContainers(includes []string) {
	c.includeContainers = includes
}

// isMonitored checks if the container should be monitored.
func (c *Client) isMonitored(name string) bool {
	if len(c.includeContainers) == 0 {
		return true // No filter, monitor all
	}
	for _, include := range c.includeContainers {
		if strings.Contains(name, include) {
			return true
		}
	}
	return false
}

// Close fecha a conexão com o Docker daemon.
//
// SEMPRE chame Close() quando terminar de usar o client.
// O padrão idiomático em Go é usar defer imediatamente após a criação:
//
//	client, err := docker.NewClient()
//	if err != nil { ... }
//	defer client.Close()
func (c *Client) Close() error {
	return c.cli.Close()
}

// StopContainer para um container pelo nome ou ID.
//
// TIMEOUT:
//
//	O Docker primeiro envia SIGTERM ao PID 1 do container.
//	Se o processo não encerrar dentro do timeout, envia SIGKILL.
//	Usamos 10 segundos como timeout padrão — o mesmo do `docker stop`.
func (c *Client) StopContainer(ctx context.Context, containerID string) error {
	timeout := 10 // segundos
	stopOpts := container.StopOptions{Timeout: &timeout}

	if err := c.cli.ContainerStop(ctx, containerID, stopOpts); err != nil {
		return fmt.Errorf("docker.StopContainer: %w", err)
	}
	return nil
}

// RestartContainer reinicia um container.
//
// O Docker faz stop + start atomicamente. O timeout se aplica
// apenas à fase de stop (SIGTERM → SIGKILL).
func (c *Client) RestartContainer(ctx context.Context, containerID string) error {
	timeout := 10
	restartOpts := container.StopOptions{Timeout: &timeout}

	if err := c.cli.ContainerRestart(ctx, containerID, restartOpts); err != nil {
		return fmt.Errorf("docker.RestartContainer: %w", err)
	}
	return nil
}

// InspectContainer retorna informações detalhadas sobre um container,
// incluindo configurações de segurança, rede e volumes.
func (c *Client) InspectContainer(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	inspectJSON, err := c.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return types.ContainerJSON{}, fmt.Errorf("docker.InspectContainer: %w", err)
	}
	return inspectJSON, nil
}

// PruneSystem executa uma limpeza de segurança (Garbage Collection):
// Remove containers parados, redes vazias e imagens pendentes (danglings).
func (c *Client) PruneSystem(ctx context.Context) (uint64, error) {
	var totalReclaimed uint64

	// Prune Containers (exited)
	containersPrune, err := c.cli.ContainersPrune(ctx, filters.Args{})
	if err == nil {
		totalReclaimed += containersPrune.SpaceReclaimed
	}

	// Prune Networks
	_, _ = c.cli.NetworksPrune(ctx, filters.Args{})

	// Prune Images (dangling by default)
	imagesPrune, err := c.cli.ImagesPrune(ctx, filters.Args{})
	if err == nil {
		totalReclaimed += imagesPrune.SpaceReclaimed
	}

	return totalReclaimed, nil
}

// ListNetworks lista informações detalhadas de conectividade do Docker
func (c *Client) ListNetworks(ctx context.Context) ([]network.Inspect, error) {
	nets, err := c.cli.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("docker.ListNetworks: %w", err)
	}

	var results []network.Inspect
	for _, n := range nets {
		// Inspeciona caso a caso para pegar os containers engajados dinamicamente
		insp, err := c.cli.NetworkInspect(ctx, n.ID, network.InspectOptions{})
		if err == nil {
			results = append(results, insp)
		}
	}
	return results, nil
}

// StreamContainerLogs retorna um channel com as últimas linhas de log
// de um container, seguido de logs em tempo real (tail -f).
//
// DOCKER LOGS API:
//
//	A API /containers/{id}/logs com Follow:true mantém a conexão
//	aberta e envia novas linhas à medida que o container produz output.
//	Usamos Tail:"50" para mostrar as últimas 50 linhas primeiro.
//
// CHANNEL PATTERN:
//
//	Retornamos um channel de strings para integrar com o loop
//	de mensagens do Bubble Tea. Cada linha de log vira uma mensagem.
func (c *Client) StreamContainerLogs(ctx context.Context, containerID string) (<-chan string, error) {
	reader, err := c.cli.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Tail:       "50",
		Timestamps: true,
	})
	if err != nil {
		return nil, fmt.Errorf("docker.StreamContainerLogs: %w", err)
	}

	logCh := make(chan string, 100) // Buffer de 100 linhas

	go func() {
		defer close(logCh)
		defer reader.Close()

		// Docker logs têm um header de 8 bytes por linha no modo multiplexed.
		// Lemos byte a byte para extrair linhas completas.
		buf := make([]byte, 8192)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				n, err := reader.Read(buf)
				if err != nil {
					return
				}
				if n > 0 {
					// Remove header de 8 bytes do Docker multiplexed stream
					content := string(buf[:n])
					// Split por linhas e envia cada uma
					lines := strings.Split(content, "\n")
					for _, line := range lines {
						// Limpa caracteres de controle do header Docker
						cleaned := cleanLogLine(line)
						if cleaned != "" {
							select {
							case logCh <- cleaned:
							default:
								// Buffer cheio, descarta linha antiga
							}
						}
					}
				}
			}
		}
	}()

	return logCh, nil
}

// cleanLogLine remove o header de 8 bytes do Docker multiplexed stream.
func cleanLogLine(line string) string {
	// O header Docker tem 8 bytes: [stream_type, 0, 0, 0, size_bytes...]
	// Se a linha começa com byte <= 2, provavelmente tem header
	if len(line) > 8 && (line[0] == 1 || line[0] == 2 || line[0] == 0) {
		return strings.TrimSpace(line[8:])
	}
	return strings.TrimSpace(line)
}

// SystemDiskUsage representa um resumo simplificado de consumo do Docker no host.
type SystemDiskUsage struct {
	ImagesReclaimable  int64
	VolumesReclaimable int64
}

// GetDiskUsage retorna o uso de disco do Docker (equivalente a docker system df).
func (c *Client) GetDiskUsage(ctx context.Context) (SystemDiskUsage, error) {
	du, err := c.cli.DiskUsage(ctx, types.DiskUsageOptions{})
	if err != nil {
		return SystemDiskUsage{}, fmt.Errorf("docker.GetDiskUsage: %w", err)
	}

	var imgReclaim, volReclaim int64

	for _, img := range du.Images {
		if img.Containers == 0 { // Imagem não está sendo usada por nenhum container
			imgReclaim += img.SharedSize + (img.Size - img.SharedSize)
		}
	}

	for _, vol := range du.Volumes {
		if vol.UsageData != nil && vol.UsageData.RefCount == 0 {
			volReclaim += vol.UsageData.Size
		}
	}

	return SystemDiskUsage{
		ImagesReclaimable:  imgReclaim,
		VolumesReclaimable: volReclaim,
	}, nil
}

// PruneImages apaga todas as imagens órfãs (dangling = true)
func (c *Client) PruneImages(ctx context.Context) (uint64, error) {
	report, err := c.cli.ImagesPrune(ctx, filters.NewArgs(filters.Arg("dangling", "true")))
	if err != nil {
		return 0, err
	}
	return report.SpaceReclaimed, nil
}

// PruneVolumes apaga volumes locais que não estão atrelados a nenhum container
func (c *Client) PruneVolumes(ctx context.Context) (uint64, error) {
	report, err := c.cli.VolumesPrune(ctx, filters.NewArgs())
	if err != nil {
		return 0, err
	}
	return report.SpaceReclaimed, nil
}

// DockerEvent representa um evento de lifecycle de container.
//
// Eventos Docker são o mecanismo nativo para monitoramento em tempo real.
// Em vez de fazer polling periódico (lento e ineficiente), o Docker daemon
// envia eventos via streaming HTTP quando containers mudam de estado.
//
// Tipos de eventos relevantes para observabilidade:
//   - "start"   → container iniciou
//   - "stop"    → container foi parado (graceful)
//   - "die"     → container encerrou (pode ser crash)
//   - "create"  → container foi criado (mas pode não estar running)
//   - "destroy" → container foi removido
//   - "pause"   → container foi pausado
//   - "unpause" → container foi retomado
type DockerEvent struct {
	// Action é o tipo do evento (start, stop, die, create, destroy, etc.)
	Action string

	// ContainerID é o ID completo do container afetado.
	ContainerID string

	// ContainerName é o nome do container (sem prefixo "/").
	ContainerName string

	// Image é a imagem Docker do container.
	Image string
}

// WatchEvents escuta eventos de containers do Docker daemon em tempo real.
//
// ARQUITETURA — Event-Driven vs Polling:
//
//	Esta função usa a Docker Events API (HTTP streaming) para receber
//	notificações instantâneas de mudanças. Isso é MUITO mais eficiente
//	que polling periódico porque:
//	  - Zero CPU quando nenhum evento ocorre
//	  - Latência mínima (milissegundos vs segundos do polling)
//	  - Menos chamadas à API do Docker
//
// CHANNELS EM GO:
//
//	A função retorna um channel (<-chan DockerEvent) que é o mecanismo
//	nativo de Go para comunicação entre goroutines. O chamador lê
//	eventos deste channel usando range ou select.
//
// PADRÃO — Context-based lifecycle:
//
//	O channel é fechado automaticamente quando o context é cancelado
//	(Ctrl+C ou SIGTERM). Isso garante que a goroutine interna não
//	vaza (leak), mesmo em cenários de erro.
func (c *Client) WatchEvents(ctx context.Context) (<-chan DockerEvent, <-chan error) {
	eventCh := make(chan DockerEvent)
	errCh := make(chan error, 1) // buffer 1 para não bloquear a goroutine

	// Filtra apenas eventos de containers (ignora imagens, volumes, redes).
	// Isso reduz o volume de eventos e o processamento necessário.
	msgs, errs := c.cli.Events(ctx, types.EventsOptions{
		Filters: filters.NewArgs(
			filters.Arg("type", "container"),
			filters.Arg("event", "start"),
			filters.Arg("event", "stop"),
			filters.Arg("event", "die"),
			filters.Arg("event", "create"),
			filters.Arg("event", "destroy"),
			filters.Arg("event", "pause"),
			filters.Arg("event", "unpause"),
		),
	})

	// GOROUTINE — Processamento assíncrono de eventos:
	//   Lançamos uma goroutine para ler eventos do Docker daemon
	//   e encaminhá-los para o channel de saída. A goroutine encerra
	//   quando o context é cancelado (channel msgs é fechado pelo SDK).
	//
	//   REGRA: toda goroutine deve ter uma condição de saída clara.
	//   Aqui, a saída é garantida pelo ctx.Done() que fecha msgs.
	go func() {
		defer close(eventCh)
		defer close(errCh)

		for {
			select {
			case <-ctx.Done():
				return
			case err, ok := <-errs:
				if !ok {
					return
				}
				errCh <- err
				return
			case msg, ok := <-msgs:
				if !ok {
					return
				}
				// Extrai o nome do container do atributo "name" do evento.
				name := msg.Actor.Attributes["name"]

				if !c.isMonitored(name) {
					continue
				}

				eventCh <- DockerEvent{
					Action:        string(msg.Action),
					ContainerID:   msg.Actor.ID[:12],
					ContainerName: name,
					Image:         msg.Actor.Attributes["image"],
				}
			}
		}
	}()

	return eventCh, errCh
}

// GetAllContainerStats retorna métricas de performance de TODOS os
// containers em execução. Coleta stats de cada container em paralelo.
//
// DOCKER STATS API:
//
//	A API /containers/{id}/stats retorna um stream JSON com métricas
//	em tempo real. Usamos Stream:false para obter apenas um snapshot
//	(sem stream contínuo), o que é adequado para polling periódico.
//
// CONCORRÊNCIA COM GOROUTINES:
//
//	Coletamos stats de todos os containers em paralelo usando goroutines
//	+ sync.WaitGroup. Isso reduz a latência total de N*RTT para ~1*RTT
//	(onde RTT é o round-trip time de uma chamada à API).
func (c *Client) GetAllContainerStats(ctx context.Context) (map[string]models.ContainerMetrics, error) {
	// Primeiro, lista os containers em execução
	containerList, err := c.cli.ContainerList(ctx, container.ListOptions{All: false})
	if err != nil {
		return nil, fmt.Errorf("docker.GetAllContainerStats: falha ao listar: %w", err)
	}

	// Mapa thread-safe para resultados
	results := make(map[string]models.ContainerMetrics)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, cont := range containerList {
		// Nome do container (sem prefixo "/")
		name := ""
		if len(cont.Names) > 0 {
			name = strings.TrimPrefix(cont.Names[0], "/")
		}

		if !c.isMonitored(name) {
			continue
		}

		wg.Add(1)

		// Lança uma goroutine para cada container.
		// Capturamos 'cont' e 'name' como parâmetros para evitar race condition
		// (variável de loop compartilhada).
		go func(ctr types.Container, cName string) {
			defer wg.Done()

			stats, err := c.getContainerStats(ctx, ctr.ID)
			if err != nil {
				return // Ignora containers que falharam (podem ter parado)
			}

			stats.ContainerID = ctr.ID[:12]
			stats.ContainerName = cName
			stats.Image = ctr.Image

			mu.Lock()
			results[ctr.ID[:12]] = stats
			mu.Unlock()
		}(cont, name)
	}

	wg.Wait()
	return results, nil
}

// getContainerStats coleta métricas de UM container via Docker Stats API.
//
// CÁLCULO DE CPU%:
//
//	O Docker retorna contadores acumulados (total de nanosegundos de CPU
//	usados desde o start do container). Para calcular o percentual,
//	precisaríamos de dois snapshots e calcular o delta. Como usamos
//	Stream:false (snapshot único), calculamos uma aproximação usando
//	os contadores do sistema.
//
//	Fórmula oficial (mesma usada pelo `docker stats`):
//	  cpuDelta = container_cpu_usage - pre_container_cpu_usage
//	  systemDelta = system_cpu_usage - pre_system_cpu_usage
//	  cpu% = (cpuDelta / systemDelta) * numCPUs * 100
func (c *Client) getContainerStats(ctx context.Context, containerID string) (models.ContainerMetrics, error) {
	// Stream: false retorna um único snapshot (não um stream contínuo).
	// Isso é mais eficiente para polling periódico.
	resp, err := c.cli.ContainerStats(ctx, containerID, false)
	if err != nil {
		return models.ContainerMetrics{}, fmt.Errorf("stats: %w", err)
	}
	defer resp.Body.Close()

	var statsJSON types.StatsJSON
	if err := json.NewDecoder(resp.Body).Decode(&statsJSON); err != nil {
		return models.ContainerMetrics{}, fmt.Errorf("decode stats: %w", err)
	}

	// Cálculo de CPU percentage (fórmula do docker stats CLI)
	cpuPercent := calculateCPUPercent(&statsJSON)

	// Cálculo de memória
	memUsage := statsJSON.MemoryStats.Usage
	memLimit := statsJSON.MemoryStats.Limit
	var memPercent float64
	if memLimit > 0 {
		memPercent = float64(memUsage) / float64(memLimit) * 100.0
	}

	// Cálculo de rede (soma de todas as interfaces)
	var netRx, netTx uint64
	for _, net := range statsJSON.Networks {
		netRx += net.RxBytes
		netTx += net.TxBytes
	}

	// Cálculo de block I/O
	var blockRead, blockWrite uint64
	for _, bio := range statsJSON.BlkioStats.IoServiceBytesRecursive {
		switch bio.Op {
		case "read", "Read":
			blockRead += bio.Value
		case "write", "Write":
			blockWrite += bio.Value
		}
	}

	return models.ContainerMetrics{
		CPUPercent:    cpuPercent,
		MemoryUsage:   memUsage,
		MemoryLimit:   memLimit,
		MemoryPercent: memPercent,
		NetworkRx:     netRx,
		NetworkTx:     netTx,
		BlockRead:     blockRead,
		BlockWrite:    blockWrite,
	}, nil
}

// calculateCPUPercent calcula o percentual de CPU usando a fórmula
// oficial do Docker CLI.
//
// REFERÊNCIA: https://github.com/moby/moby/blob/master/api/types/stats.go
func calculateCPUPercent(stats *types.StatsJSON) float64 {
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage) -
		float64(stats.PreCPUStats.CPUUsage.TotalUsage)

	systemDelta := float64(stats.CPUStats.SystemUsage) -
		float64(stats.PreCPUStats.SystemUsage)

	if systemDelta > 0.0 && cpuDelta > 0.0 {
		numCPUs := float64(stats.CPUStats.OnlineCPUs)
		if numCPUs == 0 {
			numCPUs = float64(len(stats.CPUStats.CPUUsage.PercpuUsage))
		}
		if numCPUs == 0 {
			numCPUs = 1
		}
		return (cpuDelta / systemDelta) * numCPUs * 100.0
	}

	return 0.0
}

// GetSystemInfo retorna informações detalhadas sobre o Docker daemon.
//
// Estas informações são essenciais para observabilidade:
//   - Versão do servidor e da API
//   - Sistema operacional e arquitetura do host
//   - Memória total disponível
//   - Quantidade de containers e imagens
//
// Em um agente de produção, estas informações seriam exportadas como
// métricas (Prometheus gauges) e usadas para dashboards de infraestrutura.
func (c *Client) GetSystemInfo(ctx context.Context) (map[string]string, error) {
	// ServerVersion retorna a versão do Docker daemon.
	// É uma chamada leve (HTTP GET /version) e útil para health check.
	version, err := c.cli.ServerVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("docker.GetSystemInfo: falha ao obter versão: %w", err)
	}

	// Info retorna informações detalhadas do sistema Docker.
	// Inclui: OS, arquitetura, memória, CPUs, containers, imagens, etc.
	info, err := c.cli.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("docker.GetSystemInfo: falha ao obter info: %w", err)
	}

	// Formata memória total em formato legível (GB).
	memoryGB := float64(info.MemTotal) / (1024 * 1024 * 1024)

	result := map[string]string{
		"Server Version": version.Version,
		"API Version":    version.APIVersion,
		"OS/Arch":        fmt.Sprintf("%s/%s", version.Os, version.Arch),
		"Kernel":         version.KernelVersion,
		"Total Memory":   fmt.Sprintf("%.1f GB", memoryGB),
		"Containers":     fmt.Sprintf("%d total (%d running, %d stopped, %d paused)", info.Containers, info.ContainersRunning, info.ContainersStopped, info.ContainersPaused),
		"Images":         fmt.Sprintf("%d", info.Images),
	}

	return result, nil
}

// ListRunningContainers retorna uma lista de todos os containers em execução.
//
// PARÂMETRO ctx (context.Context):
//   - Permite cancelamento da operação (ex: Ctrl+C, timeout, deadline)
//   - Se o context for cancelado durante a chamada HTTP ao Docker daemon,
//     a operação é abortada imediatamente e retorna ctx.Err()
//   - REGRA DE OURO: toda função que faz I/O deve aceitar um context
//     como primeiro parâmetro
//
// RETORNO ([]models.ContainerInfo, error):
//   - Multi-value return é o padrão Go para operações que podem falhar
//   - Nunca retorne (nil, nil) — sempre retorne dados OU erro
//   - Em caso de sucesso com lista vazia, retorne ([]ContainerInfo{}, nil)
func (c *Client) ListRunningContainers(ctx context.Context) ([]models.ContainerInfo, error) {
	// container.ListOptions filtra quais containers retornar.
	// All: false retorna apenas containers em execução (Running).
	// Para incluir containers parados, use All: true.
	containers, err := c.cli.ContainerList(ctx, container.ListOptions{
		All: false, // Apenas containers em execução
	})
	if err != nil {
		return nil, fmt.Errorf("docker.ListRunningContainers: falha ao listar: %w", err)
	}

	// Pré-alocamos o slice com o tamanho exato usando make([]T, 0, cap).
	// Isso evita realocações de memória durante o append, melhorando
	// a performance. Em um agente de observabilidade, cada alocação conta.
	result := make([]models.ContainerInfo, 0, len(containers))

	for _, ctr := range containers {
		info := toContainerInfo(ctr)
		if c.isMonitored(info.Name) {
			result = append(result, info)
		}
	}

	return result, nil
}

// ListRunningContainersDetailed retorna informações detalhadas de todos
// os containers em execução, incluindo labels, redes, comando e tempo
// de criação. Usado para logging avançado.
//
// Esta versão é mais pesada que ListRunningContainers porque coleta
// dados adicionais. Use-a para display; use a versão simples para
// coleta de métricas onde performance é crítica.
func (c *Client) ListRunningContainersDetailed(ctx context.Context) ([]logger.ContainerDisplay, error) {
	containers, err := c.cli.ContainerList(ctx, container.ListOptions{
		All: false,
	})
	if err != nil {
		return nil, fmt.Errorf("docker.ListRunningContainersDetailed: falha ao listar: %w", err)
	}

	result := make([]logger.ContainerDisplay, 0, len(containers))

	for _, ct := range containers {
		cd := toContainerDisplay(ct)
		if !c.isMonitored(cd.Name) {
			continue
		}

		// Enriquece com dados do Inspect (Env, Health)
		inspect, err := c.cli.ContainerInspect(ctx, ct.ID)
		if err == nil {
			// Env vars
			if inspect.Config != nil {
				cd.Env = inspect.Config.Env

				// Entrypoint / Cmd
				if len(inspect.Config.Entrypoint) > 0 {
					cd.Entrypoint = strings.Join(inspect.Config.Entrypoint, " ")
				}
				if len(inspect.Config.Cmd) > 0 {
					if cd.Entrypoint != "" {
						cd.Entrypoint += " " + strings.Join(inspect.Config.Cmd, " ")
					} else {
						cd.Entrypoint = strings.Join(inspect.Config.Cmd, " ")
					}
				}
				if len(cd.Entrypoint) > 80 {
					cd.Entrypoint = cd.Entrypoint[:77] + "..."
				}
			}

			// Health Check
			if inspect.State != nil && inspect.State.Health != nil {
				cd.HealthStatus = string(inspect.State.Health.Status)
				if len(inspect.State.Health.Log) > 0 {
					last := inspect.State.Health.Log[len(inspect.State.Health.Log)-1]
					cd.HealthLog = strings.TrimSpace(last.Output)
					if len(cd.HealthLog) > 120 {
						cd.HealthLog = cd.HealthLog[:117] + "..."
					}
				}
			}

			// Restart Policy & Count
			if inspect.HostConfig != nil {
				cd.RestartPolicy = string(inspect.HostConfig.RestartPolicy.Name)
				if inspect.HostConfig.RestartPolicy.MaximumRetryCount > 0 {
					cd.RestartPolicy += fmt.Sprintf(":%d", inspect.HostConfig.RestartPolicy.MaximumRetryCount)
				}

				// Resource Limits
				if inspect.HostConfig.NanoCPUs > 0 {
					cd.CPULimit = float64(inspect.HostConfig.NanoCPUs) / 1e9
				}
				if inspect.HostConfig.Memory > 0 {
					cd.MemoryLimit = inspect.HostConfig.Memory
				}
			}

			// Restart Count
			if inspect.RestartCount > 0 {
				cd.RestartCount = inspect.RestartCount
			}

			// Mounts / Bind Volumes
			if len(inspect.Mounts) > 0 {
				for _, mt := range inspect.Mounts {
					label := fmt.Sprintf("%s → %s (%s)", mt.Source, mt.Destination, mt.Type)
					if len(label) > 80 {
						label = label[:77] + "..."
					}
					cd.Mounts = append(cd.Mounts, label)
				}
			}
		}

		result = append(result, cd)
	}

	return result, nil
}

// toContainerInfo converte um container da SDK do Docker para nosso DTO interno.
//
// Esta função é não-exportada (minúscula) porque é um detalhe de implementação.
// O chamador externo não precisa saber como fazemos a conversão.
func toContainerInfo(c types.Container) models.ContainerInfo {
	// Os nomes dos containers no Docker começam com "/" por razões históricas.
	// Removemos a barra para exibição mais limpa.
	name := ""
	if len(c.Names) > 0 {
		name = strings.TrimPrefix(c.Names[0], "/")
	}

	// Formata as portas expostas de forma legível.
	ports := formatPorts(c.Ports)

	return models.ContainerInfo{
		ID:     c.ID[:12], // Usamos apenas os 12 primeiros caracteres (short ID)
		Name:   name,
		Image:  c.Image,
		Status: c.Status,
		State:  c.State,
		Ports:  ports,
	}
}

// toContainerDisplay converte um container para o formato detalhado de display.
func toContainerDisplay(c types.Container) logger.ContainerDisplay {
	name := ""
	if len(c.Names) > 0 {
		name = strings.TrimPrefix(c.Names[0], "/")
	}

	// Extrai nomes das redes conectadas ao container.
	networks := make([]string, 0)
	if c.NetworkSettings != nil {
		for netName := range c.NetworkSettings.Networks {
			networks = append(networks, netName)
		}
	}

	// Filtra labels relevantes (ignora labels internos do Docker/Compose).
	labels := make(map[string]string)
	for k, v := range c.Labels {
		// Inclui apenas labels de usuário ou compose, ignora labels
		// internos muito verbosos do Docker Desktop.
		if !strings.HasPrefix(k, "desktop.docker.") &&
			!strings.HasPrefix(k, "org.opencontainers.") {
			labels[k] = v
		}
	}

	// Formata o timestamp de criação em formato legível.
	created := time.Unix(c.Created, 0).Format("2006-01-02 15:04:05")

	// Trunca o comando se muito longo para exibição.
	command := c.Command
	if len(command) > 60 {
		command = command[:57] + "..."
	}

	return logger.ContainerDisplay{
		ID:       c.ID[:12],
		Name:     name,
		Image:    c.Image,
		Status:   c.Status,
		State:    c.State,
		Command:  command,
		Ports:    formatPorts(c.Ports),
		Created:  created,
		Networks: networks,
		Labels:   labels,
	}
}

// formatPorts converte a lista de portas da API do Docker em uma
// representação legível para humanos (ex: "0.0.0.0:8080->80/tcp").
func formatPorts(ports []types.Port) string {
	if len(ports) == 0 {
		return ""
	}

	// strings.Builder é mais eficiente que concatenação com + para
	// múltiplas strings, pois evita alocações intermediárias.
	var b strings.Builder

	for i, p := range ports {
		if i > 0 {
			b.WriteString(", ")
		}

		if p.PublicPort != 0 {
			fmt.Fprintf(&b, "%s:%d->%d/%s", p.IP, p.PublicPort, p.PrivatePort, p.Type)
		} else {
			fmt.Fprintf(&b, "%d/%s", p.PrivatePort, p.Type)
		}
	}

	return b.String()
}

// RunStressTest cria um container temporário de stress test.
// Usamos alpine + stress-ng para garantir compatibilidade com
// todas as arquiteturas (arm64, amd64) e evitar erros de manifest antigo.
// O container se auto-remove após terminar.
//
// MODOS:
//   - "cpu"    → stress-ng --cpu 2 --timeout Xs
//   - "memory" → stress-ng --vm 1 --vm-bytes 256M --timeout Xs
//   - "both"   → stress-ng --cpu 2 --vm 1 --vm-bytes 256M --timeout Xs
//
// DOCKER API (ContainerCreate + ContainerStart):
//
//	Usamos a mesma API que o `docker run` usa internamente.
//	AutoRemove:true garante que o container é removido após execução,
//	evitando acúmulo de containers parados.
func (c *Client) RunStressTest(ctx context.Context, mode string, durationSec int) error {
	stressImage := "alpine:latest"
	containerName := "castle-rock-stress"

	// Monta os argumentos do stress-ng
	var stressArgs string
	switch mode {
	case "cpu":
		stressArgs = fmt.Sprintf("--cpu 2 --timeout %ds", durationSec)
	case "memory":
		// --vm-hang 0 = sleep after allocation to keep memory held without burning CPU
		stressArgs = fmt.Sprintf("--vm 1 --vm-bytes 256M --vm-hang 0 --timeout %ds", durationSec)
	case "both":
		stressArgs = fmt.Sprintf("--cpu 2 --vm 1 --vm-bytes 256M --vm-hang 0 --timeout %ds", durationSec)
	default:
		return fmt.Errorf("docker.RunStressTest: modo inválido: %s", mode)
	}

	cmd := []string{"sh", "-c", fmt.Sprintf("apk add --no-cache stress-ng && stress-ng %s", stressArgs)}

	// Tenta remover container anterior com mesmo nome (se existir)
	_ = c.cli.ContainerRemove(ctx, containerName, container.RemoveOptions{Force: true})

	// Puxa a imagem se não existir localmente
	_, _, err := c.cli.ImageInspectWithRaw(ctx, stressImage)
	if err != nil {
		reader, pullErr := c.cli.ImagePull(ctx, "docker.io/library/"+stressImage, image.PullOptions{})
		if pullErr != nil {
			return fmt.Errorf("docker.RunStressTest: falha ao baixar imagem %s: %w", stressImage, pullErr)
		}
		defer reader.Close()
		// Consome o reader para completar o pull
		buf := make([]byte, 4096)
		for {
			_, readErr := reader.Read(buf)
			if readErr != nil {
				break
			}
		}
	}

	// Cria o container
	resp, err := c.cli.ContainerCreate(ctx,
		&container.Config{
			Image: stressImage,
			Cmd:   cmd,
		},
		&container.HostConfig{
			AutoRemove: true, // Remove automaticamente após terminar
		},
		nil, nil, containerName,
	)
	if err != nil {
		return fmt.Errorf("docker.RunStressTest: falha ao criar container: %w", err)
	}

	// Inicia o container
	if err := c.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("docker.RunStressTest: falha ao iniciar container: %w", err)
	}

	return nil
}
