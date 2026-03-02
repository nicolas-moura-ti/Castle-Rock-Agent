# 🏰 Castle Rock Agent

*Leia em outros idiomas: [English](README.md), [Português](README.pt-BR.md).*

> Agente de observabilidade nativo em Go para monitoramento de containers Docker com dashboard interativo, métricas Prometheus e alertas configuráveis.

### ⛰️ Por que "Castle Rock"?
Inspirado nas torres de vigia medievais erguidas sobre rochedos (Castle Rocks), que ofereciam **visão panorâmica absoluta** de tudo que acontecia ao redor do castelo. Assim como essas torres, este agente fica em um ponto de observação privilegiado (o Docker Socket) para monitorar, vigiar e alertar sobre a saúde de toda a sua infraestrutura de containers.

---

## 🧠 Como Funciona — Explicação Simples

Imagine que você tem vários containers Docker rodando (banco de dados, API, nginx...). Você quer saber: **Quanta CPU cada um está usando? Está consumindo muita memória? A rede está normal?**

O problema é que o Docker sozinho tem a memória de um peixinho dourado. Ele sabe o que está acontecendo **agora**, mas não faz ideia do que aconteceu há 5 minutos. E também não tem como gerar gráficos bonitos para você mostrar na reunião de sexta-feira.

Para resolver isso, usamos **3 peças** que trabalham juntas:

### 🏰 Castle Rock Agent (este projeto)
É o **coletor**. Ele se conecta ao Docker, pergunta "como cada container está?", e traduz essas informações para um formato padronizado. Sem ele, o Prometheus não teria como acessar os dados do Docker.

**Analogia:** É como um termômetro que mede a temperatura e expõe a leitura em uma tela.

### 📊 Prometheus
É o **banco de dados de métricas**. A cada 5 segundos ele visita o agente (`http://agent:9110/metrics`), coleta os números e **guarda com timestamp**. Assim você pode perguntar: "Qual foi a CPU do postgres às 14h30?"

**Analogia:** É como um caderno onde alguém anota a temperatura a cada 5 segundos. Depois de uma semana, você tem um histórico completo.

### 📈 Grafana
É o **painel de visualização**. Ele lê os dados do Prometheus e gera **gráficos, gauges e tabelas** em tempo real. É onde você realmente "vê" o que está acontecendo.

**Analogia:** É como pegar aquele caderno de anotações e transformar em um gráfico bonito na tela.

### Fluxo de dados

```
Docker Containers → Castle Rock Agent → Prometheus → Grafana
(geram métricas)    (coleta e traduz)   (armazena)    (visualiza)
```

**Por que não usar uma ferramenta só?** Porque na prática cada peça faz UMA coisa muito bem. Essa separação é o padrão da indústria — é assim que empresas como Google, Netflix e Uber monitoram seus sistemas.

---

## ✨ Features

| Feature | Descrição |
|---|---|
| **TUI Interativa** | Dashboard fullscreen com [Bubble Tea](https://github.com/charmbracelet/bubbletea) — tabela de containers, métricas, eventos |
| **Métricas Tempo Real** | CPU%, Memória%, Network I/O, Disk I/O via Docker Stats API |
| **Prometheus Exporter** | Expõe 9 métricas em `/metrics` para scraping (porta 9110) |
| **Grafana Dashboard** | Dashboard pré-configurado com 6 painéis (CPU, Memória, Rede, Gauges) |
| **Alertas Configuráveis** | Regras customizáveis com threshold + duração (similar ao Alertmanager) |
| **Container Actions** | Stop e restart de containers direto da TUI com confirmação |
| **Logs Streaming** | Visualizar logs do container em tempo real (como `docker logs -f`) |
| **Docker Events** | Eventos de lifecycle (start, stop, die) com ícones e cores |
| **Cluster Mode 🌐** | Arquitetura Multi-Host (Leader/Worker) para agregar métricas de múltiplos servidores |
| **SQLite Historian 🗄️** | Banco de dados SQLite Puro-Go para persistir o histórico de alertas e eventos (porque a memória do admin não deve ser tão curta quanto a RAM). |
| **Auto Prune 🧹** | Um Garbage Collector próprio pro Docker. Vigia o uso de disco do host e roda um `docker system prune` nativo antes que seu servidor vá para o espaço. |
| **Service Map 🕸️** | Aperte `M` para inspecionar visualmente a topologia da sua Rede Docker e ver quem está na mesma rede de quem. |
| **Auditoria de Segurança 🛡️** | Scanning "Shift-Left" em tempo real. O motor intercepta 9 anomalias críticas de segurança (ex: usuário root, modo privilegiado, portas de DB expostas globalmente). |
| **i18n & Internacionalização 🌍** | O agente agora fala fluentemente Inglês (`en`) e Português (`pt`). Modifique no `config.yaml` ou via env var `CASTLE_ROCK_LANGUAGE=pt`. |
| **Config YAML + ENV** | Configuração via `config.yaml` com override por variáveis de ambiente |

---

## Arquitetura

```
┌────────────────────────────────────────────────────────────────┐
│                    Castle Rock Agent v0.3.0                    │
│                                                                │
│  ┌─────────────┐   ┌──────────────┐   ┌─────────────────────┐  │
│  │     TUI     │   │  Prometheus  │   │    Alert Engine     │  │
│  │ (bubbletea) │   │  HTTP :9110  │   │   (rules + state)   │  │
│  └──────┬──────┘   └──────┬───────┘   └──────────┬──────────┘  │
│         │                 │                      │             │
│         │                 │  ┌────────────────┐  │             │
│         │                 │◄─┤ Cluster (Push) │  │             │
│         │                 │  └───────▲────────┘  │             │
│         └─────────┬───────┴──────────┼───────────┘             │
│                   ▼                  │ (HTTP POST)             │
│         ┌──────────────────┐         │  ┌───────────────────┐  │
│         │  Docker Client   │         └──┤ Worker Node (Ag.) │  │
│         │  (SDK oficial)   │            └───────────────────┘  │
│         └─────────┬────────┘                                   │
│                   │                                            │
└───────────────────┼────────────────────────────────────────────┘
                    ▼
            Docker Engine API
      (unix:///var/run/docker.sock)
```

### Estrutura de Diretórios

```
castle-rock-agent/
├── cmd/agent/main.go              # Entrypoint — bootstrapping e modos (TUI/headless)
├── internal/
│   ├── docker/client.go           # Docker SDK: containers, stats, events, logs, actions
│   ├── tui/tui.go                 # Dashboard interativo (Bubble Tea + lipgloss)
│   ├── metrics/prometheus.go      # Exportador Prometheus com 9 GaugeVec
│   ├── alerts/alerts.go           # Motor de alertas (regras, pending→firing→resolved)
│   ├── config/config.go           # Loader YAML + env vars (12-Factor App)
│   ├── logger/logger.go           # slog customizado com cores ANSI
│   └── collector/container.go     # Interface Collector (extensível)
├── pkg/models/container.go        # DTOs: ContainerInfo, ContainerMetrics, ContainerDisplay
├── configs/config.yaml            # Configuração YAML documentada
├── deploy/
│   ├── prometheus/
│   │   ├── prometheus.yml         # Scrape config
│   │   └── alert_rules.yml        # Regras de alerta Prometheus
│   └── grafana/provisioning/
│       ├── datasources/           # Auto-config Prometheus como datasource
│       └── dashboards/            # Dashboard JSON pré-configurado
├── docker-compose.yml             # Stack completa: Agent + Prometheus + Grafana
├── Dockerfile                     # Multi-stage build (Go 1.24 → Alpine)
├── Makefile                       # Targets padronizados
└── go.mod / go.sum
```

---

## Pré-requisitos

| Dependência | Versão Mínima | Verificação |
|---|---|---|
| **Go** | 1.24+ | `go version` |
| **Docker** | 20.10+ | `docker --version` |
| **Docker Desktop** ou **Engine** | Rodando | `docker info` |
| **Make** | qualquer | `make --version` |

### macOS — Xcode Command Line Tools

```bash
# Instalar (se necessário)
xcode-select --install

# Aceitar licença (OBRIGATÓRIO após instalação/atualização)
sudo xcodebuild -license accept
```

> ⚠️ **Sem aceitar a licença, o `go build` falhará** com `"please accept the Xcode license as the root user"`.

---

## Quick Start

### Modo 1: TUI Local (desenvolvimento)

```bash
# Clone
git clone https://github.com/nicolas-moura-ti/castle-rock-agent.git
cd castle-rock-agent

# Instalar dependências
make tidy

# Executar (abre dashboard interativo)
make run
```

### Modo 2: Docker Compose (stack completa de observabilidade)

```bash
# Levanta Agent + Prometheus + Grafana
docker compose up -d

# Acesse:
# - Grafana:    http://localhost:3000  (admin / castlerock)
# - Prometheus: http://localhost:9090
# - Métricas:   http://localhost:9110/metrics
```

Para parar:

```bash
docker compose down
```

### Comparação dos modos

| | `make run` | `docker compose up` |
|---|---|---|
| **Onde roda** | Direto no seu Mac | Dentro de containers Docker |
| **TUI (dashboard terminal)** | ✅ Sim | ❌ Não (headless) |
| **Prometheus coletando** | ❌ Não | ✅ Sim |
| **Grafana com gráficos** | ❌ Não | ✅ Sim |
| **Por que usar** | Desenvolvimento e análise rápida | Monitoramento 24/7 e alertas visuais |

> 💡 **USO EM PARALELO:** Você pode (e deve!) rodar os dois modos ao mesmo tempo!
> Deixe o `docker compose up -d` rodando no fundo coletando métricas para o Grafana, e sempre que quiser inspecionar a fundo ou testar cargas ("Stress Test"), abra um terminal e rode `make run`. Um não interfere no outro e ambos monitoram o mesmo ambiente de forma cooperativa.

### Modo 3: Multi-Host Cluster 🌐

O agente suporta rodar de forma distribuída para monitorar múltiplos servidores de uma vez.
Você sobe **um Leader** e múltiplos **Workers** (agentes remotos que enviam dados para o Leader).

```bash
# Servidor Principal (Leader)
CASTLE_ROCK_CLUSTER_MODE=leader CASTLE_ROCK_CLUSTER_HOST_ID=matriz make run

# Em outro servidor (Worker) - Sem TUI, rodando em background
CASTLE_ROCK_CLUSTER_MODE=worker \
CASTLE_ROCK_CLUSTER_HOST_ID=filial-sp \
CASTLE_ROCK_CLUSTER_LEADER_URL=http://<IP_DO_LEADER>:9110 \
make run
```

Os containers do Worker aparecerão automaticamente na TUI do Leader com a coluna "HOST" indicando a origem, e o Prometheus do Leader vai expor as métricas usando a tag `host_id` para o Grafana.

---

## 🖥️ TUI — Dashboard Interativo

Ao executar `make run`, o agente abre um dashboard fullscreen:

```
 🏰 Castle Rock Agent    v0.3.0 │ Docker 29.2.1 │ ⏱ 2m30s │ 📡 5 events

    ID             NOME                 CPU%    MEM%    MEM       NET ↓/↑     ESTADO
 ▸  9b157cd0abf6   postgres             2.3%    45.2%   350MB     12M/5M      ● up
    abc123def456   redis                0.1%    12.8%   98MB      3M/1M       ● up
    fed987654321   nginx                0.5%    5.1%    42MB      50M/45M     ● up

 📋 Eventos
  ╭──────────────────────────────────────────────────────╮
  │  16:05:32 🟢 start      nginx                       │
  │  16:05:30 📦 create     nginx                       │
  │  16:04:15 🔴 die        old-container               │
  ╰──────────────────────────────────────────────────────╯

  ↑↓ navegar │ enter detalhes │ l logs │ s stop │ R restart │ r refresh │ ? ajuda │ q sair
```

### Atalhos de Teclado

| Tecla | Ação |
|---|---|
| `↑` / `k` | Navegar para cima |
| `↓` / `j` | Navegar para baixo |
| `Enter` | Expandir detalhes do container (métricas, labels, redes, portas) |
| `l` | Toggle logs em tempo real do container selecionado |
| `s` | **Stop** container (pede confirmação `y`) |
| `R` | **Restart** container (pede confirmação `y`) |
| `S` | **Stress Test Mode** (CPU/Memória para simular Noisy Neighbor) |
| `r` | Refresh manual da lista |
| `?` | Exibir/ocultar ajuda detalhada |
| `Esc` | Fechar panels abertos |
| `q` / `Ctrl+C` | Sair do agente |

### Cores dos indicadores

| Cor | Significado |
|---|---|
| 🟢 Verde | Normal (CPU < 40%, MEM < 50%) |
| 🟡 Amarelo | Atenção (CPU 40-80%, MEM 50-80%) |
| 🔴 Vermelho | Crítico (CPU > 80%, MEM > 80%) |

---

## ⚡ Stress Test Mode (Noisy Neighbor)

O agente possui uma funcionalidade didática embutida para estressar a máquina e ver as métricas/alertas disparando no Grafana em tempo real.

Ao pressionar `S` na TUI, você cria um container temporário construído via código (`alpine` + `stress-ng` nativo) que injeta carga no host:
- `c` **CPU:** Stressa 2 cores a 100%
- `m` **Memória:** Aloca e trava 256MB inteiros (sem queimar CPU)
- `b` **Ambos:** Aplica carga dupla

*O container dura exatamente 30 segundos e se auto-destrói (`AutoRemove: true`), para que você possa ver a curva de disparo dos alertas e, logo em seguida, a recuperação (resolved).*

### ⚠️ Limitações do Stress Test
- **Máquina Virtual vs Host:** No macOS e Windows (Docker Desktop), a carga máxima apontada pelo agente reflete os limites de recursos da Virtual Machine do Docker alocada, e não a máquina física inteira.
- **Conectividade Inicial:** Exige acesso à internet na 1ª execução para baixar a imagem oficial `alpine` (apenas ~5MB).
- **Sem Docker Proxy (Somente Leitura):** O teste não funciona no modo headless acoplado ao `docker-socket-proxy`. O proxy possui a flag `POST=0` travada por segurança (não permite criar containers). Por isso, sugerimos usar a TUI via `make run` diretamente no SO real local para usá-lo com sucesso.

---

## 📊 Prometheus — Métricas Exportadas

O agente expõe métricas no formato Prometheus em `http://localhost:9110/metrics`.

### Métricas Disponíveis

| Métrica | Tipo | Descrição |
|---|---|---|
| `castle_rock_container_cpu_percent` | Gauge | Percentual de uso de CPU |
| `castle_rock_container_memory_usage_bytes` | Gauge | Memória utilizada (bytes) |
| `castle_rock_container_memory_limit_bytes` | Gauge | Limite de memória (bytes) |
| `castle_rock_container_memory_percent` | Gauge | Percentual de memória usada |
| `castle_rock_container_network_rx_bytes` | Gauge | Bytes recebidos pela rede |
| `castle_rock_container_network_tx_bytes` | Gauge | Bytes transmitidos pela rede |
| `castle_rock_container_block_read_bytes` | Gauge | Bytes lidos do disco |
| `castle_rock_container_block_write_bytes` | Gauge | Bytes escritos no disco |
| `castle_rock_container_info` | Gauge | Metadata do container (labels) |

Todas as métricas possuem os labels: `container_id`, `container_name`, `image`.

### Exemplo de query PromQL

```promql
# CPU de um container específico
castle_rock_container_cpu_percent{container_name="postgres"}

# Top 5 containers por memória
topk(5, castle_rock_container_memory_percent)

# Tráfego de rede total
sum(castle_rock_container_network_rx_bytes) by (container_name)
```

### Testar localmente

```bash
# Enquanto o agente roda (make run ou docker compose up)
curl -s http://localhost:9110/metrics | grep castle_rock

# Health check
curl http://localhost:9110/health
```

---

## 📈 Grafana — 5 Dashboards Pré-configurados

O Docker Compose provisiona automaticamente **5 dashboards** no Grafana. Todos atualizam em tempo real (5s).

**Acesso:** http://localhost:3000 → Login: `admin` / `castlerock`

---

### Dashboard 1: Overview

Visão geral de todos os containers em uma única tela.

| Painel | Tipo | Descrição |
|---|---|---|
| Containers Ativos | Stat | Contagem total de containers monitorados |
| CPU Média | Stat | Média de CPU% entre todos os containers |
| Memória Média | Stat | Média de memória% entre todos os containers |
| Memória Total Usada | Stat | Soma de RAM usada por todos (em bytes) |
| Tráfego de Rede Total | Stat | Soma de RX + TX de todos os containers |
| CPU % por Container | Time Series | Histórico de CPU com linhas suaves e gradiente |
| CPU Gauge | Gauge | Velocímetros com thresholds verde/amarelo/vermelho |
| Memória % | Time Series | Histórico de memória com thresholds |
| Uso de Memória (stacked) | Time Series | Uso empilhado — mostra contribuição de cada container |
| Memória Bar Gauge | Bar Gauge | Barras horizontais com gradiente por container |
| Network RX | Time Series | Bytes recebidos por container |
| Network TX | Time Series | Bytes transmitidos por container |
| Disk Read | Time Series | Leitura de disco por container |
| Disk Write | Time Series | Escrita de disco por container |
| Containers Monitorados | Table | Tabela filtrável com ID, nome e imagem |

---

### Dashboard 2: Container Detail

Deep dive em **um container específico**. Um dropdown no topo permite selecionar qual container analisar.

| Painel | Tipo | Descrição |
|---|---|---|
| CPU Atual | Gauge | CPU% em tempo real com thresholds |
| Memória Atual | Gauge | Memória% em tempo real com thresholds |
| RAM Usada | Stat | Memória usada em bytes |
| RAM Limite | Stat | Limite de memória configurado |
| Network Total | Stat | Tráfego total (RX + TX) |
| CPU % Histórico | Time Series | CPU ao longo do tempo com zonas de warning/critical |
| Memória % Histórico | Time Series | Memória ao longo do tempo com zonas de alerta |
| Memória Uso vs Limite | Time Series | Duas linhas: uso real vs limite configurado |
| Network I/O | Time Series | Download (RX) e Upload (TX) no mesmo gráfico |
| Disk I/O | Time Series | Leitura e escrita de disco |

---

### Dashboard 3: Network Analysis

Análise focada em tráfego de rede.

| Painel | Tipo | Descrição |
|---|---|---|
| Total Download ↓ | Stat | Soma de bytes recebidos |
| Total Upload ↑ | Stat | Soma de bytes transmitidos |
| Tráfego Total | Stat | RX + TX combinados |
| Containers com Rede | Stat | Quantos containers têm tráfego > 0 |
| Download Time Series | Time Series | Bytes RX ao longo do tempo |
| Download Top Containers | Bar Gauge | Ranking de quem mais baixa dados |
| Upload Time Series | Time Series | Bytes TX ao longo do tempo |
| Upload Top Containers | Bar Gauge | Ranking de quem mais envia dados |
| Tráfego Empilhado | Time Series | Todos os RX/TX empilhados para ver o total |

---

### Dashboard 4: Memory Deep Dive

Análise profunda de memória — útil para detectar leaks e risco de OOM kill.

| Painel | Tipo | Descrição |
|---|---|---|
| RAM Total Usada | Stat | Soma de memória usada por todos os containers |
| RAM Total Limite | Stat | Soma de limites de memória configurados |
| Média Memória % | Stat | Média de utilização de memória |
| Max Memória % | Stat | Container com maior uso de memória |
| Memória % Todos | Time Series | Histórico de todos os containers com linhas de threshold |
| Uso Empilhado | Time Series | Memória usada empilhada (quem consome mais) |
| Ranking Memória % | Bar Gauge | Barras horizontais com gradiente verde→vermelho |
| Uso vs Limite | Time Series | Para cada container: uso real vs limite (quando se aproximam, risco de OOM) |

---

### Dashboard 5: Alerts & Health

Monitoramento de saúde e violações de threshold.

| Painel | Tipo | Descrição |
|---|---|---|
| Containers CPU > 80% | Stat | Quantos containers estão em estado crítico de CPU |
| Containers MEM > 85% | Stat | Quantos containers estão em estado crítico de memória |
| Containers CPU > 50% | Stat | Quantos containers estão em warning de CPU |
| Containers Saudáveis | Stat | Quantos estão abaixo de 50% em CPU e memória |
| CPU Ranking | Bar Gauge | Barras do maior para o menor uso de CPU |
| Memória Ranking | Bar Gauge | Barras do maior para o menor uso de memória |
| CPU Máxima | Time Series | Pico de CPU ao longo do tempo com zonas de alerta |
| Memória Máxima | Time Series | Pico de memória com risco de OOM |
| CPU Média vs Máxima | Time Series | Compara a média com o pico — identifica outliers |
| Memória Média vs Máxima | Time Series | Mesma comparação para memória |

---

## ⚠️ Alertas Configuráveis

O sistema de alertas funciona em **duas camadas**:

### Camada 1: Alertas na TUI (interno)

Definidos em `configs/config.yaml`, avaliados pelo motor interno do agente:

```yaml
alerts:
  enabled: true
  rules:
    - name: "CPU Crítica"
      metric: "cpu_percent"
      operator: ">"
      threshold: 80.0
      duration: 2m           # Só dispara após 2 min acima do threshold
      severity: "critical"

    - name: "Memória Alta"
      metric: "memory_percent"
      operator: ">"
      threshold: 70.0
      duration: 5m
      severity: "warning"
```

Quando um alerta dispara, ele aparece:
- 🚨 Na barra de status da TUI
- 📋 No log de eventos com detalhes
- Com indicador visual no container na tabela

### Camada 2: Alertas no Prometheus (externo)

Definidos em `deploy/prometheus/alert_rules.yml`, avaliados pelo Prometheus:

```yaml
- alert: ContainerHighCPU
  expr: castle_rock_container_cpu_percent > 80
  for: 2m
  labels:
    severity: critical
  annotations:
    summary: "CPU alta no container {{ $labels.container_name }}"
```

Estes podem ser conectados ao **Alertmanager** para notificações via Slack, email, PagerDuty, etc.

---

## ⚙️ Configuração

### Arquivo `configs/config.yaml`

```yaml
log_level: "info"           # debug, info, warn, error

prometheus:
  enabled: true
  port: 9110                # Porta do servidor HTTP

stats:
  interval: 5s              # Intervalo de coleta de métricas

alerts:
  enabled: true
  rules:                    # Regras de alerta (ver seção Alertas)
    - name: "CPU Crítica"
      metric: "cpu_percent"
      operator: ">"
      threshold: 80.0
      duration: 2m
      severity: "critical"
```

### Variáveis de Ambiente (override)

Variáveis de ambiente têm **prioridade máxima** sobre o arquivo YAML:

| Variável | Descrição | Default |
|---|---|---|
| `CASTLE_ROCK_LOG_LEVEL` | Nível de log | `info` |
| `CASTLE_ROCK_PROMETHEUS_PORT` | Porta do Prometheus | `9110` |
| `CASTLE_ROCK_PROMETHEUS_ENABLED` | Ativar/desativar Prometheus | `true` |
| `CASTLE_ROCK_STATS_INTERVAL` | Intervalo de coleta | `5s` |
| `CASTLE_ROCK_ALERTS_ENABLED` | Ativar/desativar alertas | `true` |
| `CASTLE_ROCK_MODE` | `headless` = sem TUI (Docker/K8s) | `` (TUI) |
| `CASTLE_ROCK_CLUSTER_MODE` | `standalone`, `leader`, `worker` | `standalone` |
| `CASTLE_ROCK_CLUSTER_HOST_ID` | Identificador na TUI/Grafana | Nome nativo da máquina |
| `CASTLE_ROCK_CLUSTER_LEADER_URL` | URL de destino (modo worker) | `http://127.0.0.1:9110` |

### Variáveis Docker padrão

| Variável | Descrição | Default |
|---|---|---|
| `DOCKER_HOST` | Endereço do Docker daemon | `unix:///var/run/docker.sock` |
| `DOCKER_API_VERSION` | Versão da API | auto-negociação |

### Ordem de Precedência (12-Factor App)

```
1. Defaults hardcoded (código Go)
2. configs/config.yaml (override parcial)
3. Variáveis de ambiente CASTLE_ROCK_* (override final)
```

---

## 🐳 Docker Compose — Stack de Observabilidade

O `docker-compose.yml` levanta 3 serviços:

```
┌────────────────────────────────────────────────────────┐
│                  Docker Compose Stack                  │
│                                                        │
│  ┌──────────────┐   ┌────────────┐   ┌──────────────┐  │
│  │ Castle Rock  │ → │ Prometheus │ → │   Grafana    │  │
│  │ Agent :9110  │   │   :9090    │   │    :3000     │  │
│  │  (headless)  │   │ (scraping) │   │ (dashboards) │  │
│  └──────┬───────┘   └────────────┘   └──────────────┘  │
│         │                                              │
│         ▼                                              │
│   Docker Socket                                        │
│  (monitoramento)                                       │
└────────────────────────────────────────────────────────┘
```

### Docker Socket e Permissões

O agente precisa de acesso ao Docker socket para monitorar containers:

```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock:ro  # Somente leitura
```

> ⚠️ **Por que `user: root` no docker-compose?**
>
> O Docker socket (`/var/run/docker.sock`) é de propriedade do `root`. Agentes de monitoramento como o **cAdvisor** (Google), **node-exporter** (Prometheus) e o **Datadog Agent** também rodam como root para acessar o socket.
>
> O volume é montado como `:ro` (read-only) para segurança. O agente NÃO modifica nada no Docker daemon — apenas lê informações.
>
> Em produção, considere alternativas:
> - Docker rootless mode
> - Proxy socket como [tecnativa/docker-socket-proxy](https://github.com/Tecnativa/docker-socket-proxy)

### Modo Headless

No Docker Compose, o agente roda em **modo headless** (sem TUI):

```bash
CASTLE_ROCK_MODE=headless  # Definido no docker-compose.yml
```

Neste modo, o agente atua apenas como servidor de métricas Prometheus, sem tentar abrir o dashboard interativo (que requer terminal TTY).

---

## Targets do Makefile

| Target | Descrição |
|---|---|
| `make build` | Compila o binário otimizado |
| `make run` | Compila e executa (TUI interativa) |
| `make test` | Executa testes com `-race` e cobertura |
| `make lint` | Análise estática (`go vet`) |
| `make clean` | Remove binários |
| `make tidy` | Organiza dependências |
| `make docker-build` | Constrói imagem Docker |
| `make docker-run` | Executa via Docker |

---

## 🧪 Testes

```bash
# Executar todos os testes
go test ./... -v

# Com cobertura
go test ./... -cover

# Com race detector
go test ./... -race
```

### Cobertura de testes

| Package | Testes | Cobertura |
|---|---|---|
| `internal/tui` | formatBytes, truncate, min | Formatação de métricas |
| `internal/alerts` | evaluateCondition, metrics, fire/resolve | Motor de alertas completo |
| `internal/config` | defaults, YAML loading, env overrides | Config loader |

---

## Troubleshooting

### ❌ `Agreeing to the Xcode and Apple SDKs license requires admin privileges`

```bash
sudo xcodebuild -license accept
```

Se o Xcode não estiver instalado: `xcode-select --install`

### ❌ `Cannot connect to the Docker daemon`

- **macOS**: Abra o **Docker Desktop** e aguarde o ícone ficar verde
- **Linux**: `sudo systemctl start docker`
- **Verificar**: `docker info`

### ❌ `unable to get image [...]: Cannot connect to the Docker daemon`

Este erro comum (principalmente no macOS) significa que a ferramenta não achou o socket do Docker no caminho esperado (ex: `~/.docker/run/docker.sock`).
- Verifique se o **Docker Desktop** (ou OrbStack/Colima) está aberto e completamente carregado.
- Vá nas configurações do Docker Desktop: `Settings` > `Advanced` e marque a opção **"Allow the default Docker socket to be used"** (pode exigir senha).
- Se usar **OrbStack**, rode no terminal: `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock"`

### ❌ `permission denied` no Docker socket

**No host (Linux):**
```bash
sudo usermod -aG docker $USER
newgrp docker  # ou re-login
```

**No Docker Compose:**
Já resolvido com `user: root` no `docker-compose.yml`.

### ❌ `go: command not found`

```bash
# macOS
brew install go

# Linux
wget https://go.dev/dl/go1.24.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.24.0.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
```

### ⚠️ `make: *** [run] Error 1` após Ctrl+C

**Comportamento normal.** O Go encerra com exit code 130 (SIGINT) e o `make` interpreta como erro. O agente encerrou corretamente (graceful shutdown).

### ⚠️ Containers não aparecem

1. Verifique se containers estão rodando: `docker ps`
2. Verifique o Docker context: `docker context ls`
3. Reinicie o agente: `make run`

### ⚠️ Grafana sem dados

1. Verifique se o agent está rodando: `docker compose ps`
2. Verifique as métricas: `curl http://localhost:9110/metrics`
3. Verifique o Prometheus: http://localhost:9090/targets (status deve ser "UP")

---

## Conceitos Técnicos

### Bubble Tea (Arquitetura Elm)

O dashboard usa o framework [Bubble Tea](https://github.com/charmbracelet/bubbletea) que segue a arquitetura Elm:

```
Model → Update → View (ciclo unidirecional)
```

- **Model**: struct imutável com todo o estado
- **Update**: função pura que processa mensagens e retorna novo estado
- **View**: função pura que renderiza o estado como string
- **Cmd**: operações assíncronas (Docker API, timers) que produzem mensagens

### Docker Stats API — Cálculo de CPU%

O cálculo de CPU% usa a fórmula oficial do Docker CLI:

```
cpuDelta = cpu_usage.total - pre_cpu_usage.total
systemDelta = system_cpu_usage - pre_system_cpu_usage
cpu% = (cpuDelta / systemDelta) × numCPUs × 100
```

### Context e Graceful Shutdown

Todas as goroutines compartilham um `context.Context` cancelável:

```
Ctrl+C → SIGINT → context cancelado → todas as goroutines encerram
                                     → Docker client fecha
                                     → HTTP server para
                                     → recursos liberados via defer
```

### 12-Factor App — Configuração

O agente segue o fator III (Config) da [12-Factor App](https://12factor.net/config):
configuração separada do código, com precedência: defaults → YAML → env vars.

---

## 🚀 Próximos Passos (Roadmap)

Planejamentos futuros para evoluir o Agente de Observabilidade:

- **WebSockets / gRPC para o Cluster:** Substituir as requisições HTTP REST do Worker->Leader atuais por conexões persistentes bidirecionais, reduzindo latência em clusters massivos.
- **Auto-Discovery de nós:** Serviço de descoberta local para workers e leaders se acharem via multicast/DNS.

---

## Licença

MIT
# Castle-Rock-Agent
