// Package metrics fornece o exportador de métricas Prometheus.
//
// PROMETHEUS — O PADRÃO DE OBSERVABILIDADE:
//
//	Prometheus é o sistema de monitoramento mais usado em ambientes
//	cloud-native (CNCF graduated project). Ele funciona no modelo PULL:
//	  1. A aplicação expõe métricas em um endpoint HTTP (/metrics)
//	  2. O Prometheus server faz scraping periódico desse endpoint
//	  3. As métricas são armazenadas em uma time-series database
//	  4. Grafana visualiza as métricas em dashboards
//
// TIPOS DE MÉTRICAS PROMETHEUS:
//   - Gauge: valor que sobe e desce (ex: CPU%, memória usada)
//   - Counter: valor que só cresce (ex: total de requests)
//   - Histogram: distribuição de valores (ex: latência de requests)
//   - Summary: como histogram, mas calcula percentis do lado do client
//
// Neste exportador, usamos GaugeVec (gauge com labels) porque métricas
// de containers são valores instantâneos que variam continuamente.
package metrics

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/nicolas-moura-ti/castle-rock-agent/internal/docker"
	"github.com/nicolas-moura-ti/castle-rock-agent/pkg/models"
)

// Exporter gerencia as métricas Prometheus e o servidor HTTP.
//
// DESIGN:
//
//	O Exporter roda em background (goroutine própria), coletando
//	métricas periodicamente e atualizando os gauges do Prometheus.
//	O servidor HTTP expõe /metrics para scraping do Prometheus.
type Exporter struct {
	dockerClient *docker.Client
	interval     time.Duration
	port         int
	log          *slog.Logger

	// Métricas Prometheus
	// GaugeVec permite múltiplas séries com labels diferentes.
	// Labels: container_id, container_name, image
	cpuPercent    *prometheus.GaugeVec
	memoryUsage   *prometheus.GaugeVec
	memoryLimit   *prometheus.GaugeVec
	memoryPercent *prometheus.GaugeVec
	networkRx     *prometheus.GaugeVec
	networkTx     *prometheus.GaugeVec
	blockRead     *prometheus.GaugeVec
	blockWrite    *prometheus.GaugeVec

	// Info gauge (sempre 1, carrega metadata como labels)
	containerInfo *prometheus.GaugeVec

	// Controle de lifecycle
	mu        sync.Mutex
	lastStats map[string]models.ContainerMetrics
}

// containerLabels define os labels usados em todas as métricas.
// Labels são dimensões que permitem filtrar e agrupar métricas.
//
// Exemplo de query PromQL:
//
//	castle_rock_container_cpu_percent{container_name="postgres"}
var containerLabels = []string{"container_id", "container_name", "image"}

// NewExporter cria um novo exportador Prometheus.
//
// CONVENÇÃO PROMETHEUS para nomes de métricas:
//   - Prefixo: nome da aplicação (castle_rock_)
//   - Sufixo: unidade (_bytes, _percent, _total)
//   - Snake_case sempre
//   - Referência: https://prometheus.io/docs/practices/naming/
func NewExporter(dockerClient *docker.Client, interval time.Duration, port int, log *slog.Logger) *Exporter {
	e := &Exporter{
		dockerClient: dockerClient,
		interval:     interval,
		port:         port,
		log:          log,
		lastStats:    make(map[string]models.ContainerMetrics),

		cpuPercent: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "castle_rock",
			Subsystem: "container",
			Name:      "cpu_percent",
			Help:      "Percentual de uso de CPU do container",
		}, containerLabels),

		memoryUsage: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "castle_rock",
			Subsystem: "container",
			Name:      "memory_usage_bytes",
			Help:      "Uso de memória do container em bytes",
		}, containerLabels),

		memoryLimit: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "castle_rock",
			Subsystem: "container",
			Name:      "memory_limit_bytes",
			Help:      "Limite de memória do container em bytes",
		}, containerLabels),

		memoryPercent: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "castle_rock",
			Subsystem: "container",
			Name:      "memory_percent",
			Help:      "Percentual de uso de memória do container",
		}, containerLabels),

		networkRx: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "castle_rock",
			Subsystem: "container",
			Name:      "network_rx_bytes",
			Help:      "Total de bytes recebidos pela rede",
		}, containerLabels),

		networkTx: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "castle_rock",
			Subsystem: "container",
			Name:      "network_tx_bytes",
			Help:      "Total de bytes transmitidos pela rede",
		}, containerLabels),

		blockRead: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "castle_rock",
			Subsystem: "container",
			Name:      "block_read_bytes",
			Help:      "Total de bytes lidos do disco",
		}, containerLabels),

		blockWrite: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "castle_rock",
			Subsystem: "container",
			Name:      "block_write_bytes",
			Help:      "Total de bytes escritos no disco",
		}, containerLabels),

		containerInfo: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "castle_rock",
			Subsystem: "container",
			Name:      "info",
			Help:      "Informações do container (valor sempre 1, metadata em labels)",
		}, containerLabels),
	}

	// Registra todas as métricas no registry padrão do Prometheus.
	// O registry é o repositório central de todas as métricas.
	prometheus.MustRegister(
		e.cpuPercent,
		e.memoryUsage,
		e.memoryLimit,
		e.memoryPercent,
		e.networkRx,
		e.networkTx,
		e.blockRead,
		e.blockWrite,
		e.containerInfo,
	)

	return e
}

// Start inicia o exportador: servidor HTTP + loop de coleta.
//
// GOROUTINES DE LONGA DURAÇÃO:
//
//	Lançamos duas goroutines:
//	1. Servidor HTTP (bloqueia na porta configurada)
//	2. Loop de coleta (ticker periódico)
//
//	Ambas são controladas pelo context — quando cancelado, ambas encerram.
func (e *Exporter) Start(ctx context.Context) {
	// Goroutine 1: Servidor HTTP para /metrics
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		// Health check endpoint
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"status":"ok"}`)
		})

		addr := fmt.Sprintf(":%d", e.port)
		server := &http.Server{Addr: addr, Handler: mux}

		e.log.Info("Prometheus metrics server iniciado",
			slog.String("endpoint", fmt.Sprintf("http://localhost%s/metrics", addr)),
			slog.Int("port", e.port),
		)

		// Goroutine para shutdown graceful do HTTP server
		go func() {
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			server.Shutdown(shutdownCtx)
		}()

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			e.log.Error("Erro no servidor Prometheus",
				slog.String("error", err.Error()),
			)
		}
	}()

	// Goroutine 2: Loop de coleta periódica
	go func() {
		// Coleta inicial
		e.collect(ctx)

		ticker := time.NewTicker(e.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				e.collect(ctx)
			}
		}
	}()
}

// collect executa uma coleta de métricas e atualiza os gauges Prometheus.
func (e *Exporter) collect(ctx context.Context) {
	stats, err := e.dockerClient.GetAllContainerStats(ctx)
	if err != nil {
		e.log.Debug("Falha ao coletar stats para Prometheus",
			slog.String("error", err.Error()),
		)
		return
	}

	// Reseta métricas antigas (containers que pararam)
	e.cpuPercent.Reset()
	e.memoryUsage.Reset()
	e.memoryLimit.Reset()
	e.memoryPercent.Reset()
	e.networkRx.Reset()
	e.networkTx.Reset()
	e.blockRead.Reset()
	e.blockWrite.Reset()
	e.containerInfo.Reset()

	// Atualiza métricas para cada container
	for _, s := range stats {
		labels := prometheus.Labels{
			"container_id":   s.ContainerID,
			"container_name": s.ContainerName,
			"image":          s.Image,
		}

		e.cpuPercent.With(labels).Set(s.CPUPercent)
		e.memoryUsage.With(labels).Set(float64(s.MemoryUsage))
		e.memoryLimit.With(labels).Set(float64(s.MemoryLimit))
		e.memoryPercent.With(labels).Set(s.MemoryPercent)
		e.networkRx.With(labels).Set(float64(s.NetworkRx))
		e.networkTx.With(labels).Set(float64(s.NetworkTx))
		e.blockRead.With(labels).Set(float64(s.BlockRead))
		e.blockWrite.With(labels).Set(float64(s.BlockWrite))
		e.containerInfo.With(labels).Set(1)
	}

	// Salva stats para acesso externo (pela TUI)
	e.mu.Lock()
	e.lastStats = stats
	e.mu.Unlock()
}

// GetLastStats retorna o último snapshot de métricas coletado.
// Thread-safe — pode ser chamado de qualquer goroutine.
func (e *Exporter) GetLastStats() map[string]models.ContainerMetrics {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Retorna uma cópia para evitar race conditions
	result := make(map[string]models.ContainerMetrics, len(e.lastStats))
	for k, v := range e.lastStats {
		result[k] = v
	}
	return result
}
