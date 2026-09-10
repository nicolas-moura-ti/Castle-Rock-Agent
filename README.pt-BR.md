<div align="center">

# 🏰 Castle Rock Agent

[![CI](https://github.com/nicolas-moura-ti/Castle-Rock-Agent/actions/workflows/ci.yml/badge.svg)](https://github.com/nicolas-moura-ti/Castle-Rock-Agent/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/nicolas-moura-ti/castle-rock-agent?style=flat-square)](https://goreportcard.com/report/github.com/nicolas-moura-ti/castle-rock-agent)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg?style=flat-square)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-v1.25.0-blue?style=flat-square&logo=go)](https://github.com/nicolas-moura-ti/castle-rock-agent)

\
*Leia em outros idiomas: [English](README.md) · [Português](README.pt-BR.md)*

> Agente de observabilidade nativo em Go para monitoramento de containers Docker com dashboard interativo (TUI), métricas Prometheus e alertas configuráveis.

</div>

---

### ⛰️ Por que "Castle Rock"?
Inspirado nas torres de vigia medievais erguidas sobre rochedos (Castle Rocks), que ofereciam **visão panorâmica absoluta** de tudo que acontecia ao redor do castelo. Assim como essas torres, este agente fica em um ponto de observação privilegiado (o Docker Socket) para monitorar, vigiar e alertar sobre a saúde de toda a sua infraestrutura de containers.

---

## 📖 Documentação Completa por Módulos

Para mergulhar em detalhes técnicos e funcionalidades específicas, consulte nossos guias dedicados:

- 🖥️ **[Painel Interativo TUI & Referência de Operações](docs/TUI.pt-BR.md)** 
  *(Stress Test, Acesso a Shell `exec`, Limpeza Docker Prune, Live Tail & Grep, Diagnóstico Integrado)*
- 📈 **[Observabilidade (Prometheus & Grafana) & Motores de Alerta](docs/OBSERVABILITY.pt-BR.md)** 
  *(Lista de Métricas, Detalhes dos 5 Dashboards Grafana, Modo Cluster Leader/Worker, Regras de Alerta)*
- ⚙️ **[Configuração & Variáveis de Ambiente](docs/CONFIGURATION.pt-BR.md)**
  *(Especificação do `config.yaml`, Sobrescrita 12-Factor, Precedência de Configuração e Cache de Metadados)*

---

## 🧠 Como Funciona — Explicação Rápida

O fluxo de observabilidade opera através da integração de três componentes principais:

1. **Castle Rock Agent (este projeto):** Conecta-se diretamente ao Docker Socket via SDK oficial, calcula os consumos de CPU, Memória, Disco e Rede em tempo real e atua como o **coletor e exportador** nativo.
2. **Prometheus:** Atua como o **banco de dados de séries temporais (TSDB)**. A cada 5 segundos realiza scraping do endpoint HTTP (`http://agent:9110/metrics`) e armazena o histórico das métricas.
3. **Grafana:** A camada de **visualização**. Consulta o Prometheus e renderiza painéis visuais com gráficos de desempenho, tendências e alertas.

```
Containers Docker → Castle Rock Agent → Prometheus → Grafana
 (geram métricas)    (coleta e exporta)   (armazena)     (visualização)
```

---

## ✨ Principais Funcionalidades (Features)

| Funcionalidade | Descrição |
|---|---|
| **TUI Dinâmica** | Dashboard em tela cheia no terminal com tabelas em tempo real, métricas, eventos e logs ao vivo |
| **Métricas em Tempo Real** | CPU%, Memória%, I/O de Rede e I/O de Disco exportados continuamente |
| **Dashboards Grafana Nativos** | 5 painéis pré-configurados prontos para uso (Visão Geral, Detalhes de Container, Rede, Memória e Alertas) |
| **Camada Dupla de Alertas** | Notificações visuais imediatas no TUI e integração externa via Alertmanager no Prometheus |
| **Modo Cluster 🌐** | Arquitetura remota Leader/Worker para monitorar múltiplos servidores simultaneamente com criptografia segura (HKDF + AES-GCM) |
| **Auditoria de Segurança 🛡️** | Identificação em tempo real de vulnerabilidades e más práticas (modo privilegiado, usuário root e portas expostas) |
| **Auto Prune 🧹** | Limpeza inteligente e nativa de imagens órfãs e volumes não utilizados para preservar espaço em disco |

---

## 🏗️ Topologia e Arquitetura

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
│         │   Docker Client  │         └──┤ Worker Node (Ag.) │  │
│         │  (Official SDK)  │            └───────────────────┘  │
│         └─────────┬────────┘                                   │
│                   │                                            │
└───────────────────┼────────────────────────────────────────────┘
                    ▼
            Docker Engine API
       (unix:///var/run/docker.sock)
```

---

## 📋 Pré-requisitos

| Dependência | Versão Mínima | 
|---|---|
| **Go** | 1.25+ | 
| **Docker** | 20.10+ | 
| **Make** | Qualquer versão padrão | 

*(Observação para macOS: se for compilar localmente, garanta que a licença das Command Line Tools esteja aceita via `sudo xcodebuild -license accept`)*

---

## 🚀 Como Executar (Quick Start)

### Modo 1: TUI Local (Desenvolvimento & Diagnóstico Rápido)

Executa diretamente no seu terminal conectado ao Docker daemon local através de uma interface interativa rica:

```bash
# Clonar o repositório
git clone https://github.com/nicolas-moura-ti/castle-rock-agent.git
cd castle-rock-agent

# Executar (abre o dashboard interativo)
make run
```

### Modo 2: Docker Compose (Stack Completa de Observabilidade)

Sobe o **Castle Rock Agent em modo Headless**, o **Prometheus** e o **Grafana** em containers isolados. Ideal para monitoramento contínuo 24/7.

```bash
# Copiar arquivo de exemplo de variáveis de ambiente
cp .env.example .env

# Subir a stack completa
docker compose up -d

# Endereços de acesso:
# - Grafana:    http://localhost:3000 (login: admin / castlerock)
# - Prometheus: http://localhost:9090
# - Métricas:   http://localhost:9110/metrics
```

> 💡 **USO CONJUNTO RECOMENDADO:** Você pode manter a stack do `docker compose up -d` rodando em segundo plano coletando métricas continuamente para o Grafana e, quando desejar inspecionar detalhes a fundo ou realizar testes de carga (*Stress Test*), basta abrir um terminal e rodar `make run`. As duas instâncias operam em paralelo sem interferência.

---

## 🧪 Desenvolvimento e Testes

```bash
make test          # Executa testes com detector de race conditions (-race) e cobertura
make lint          # Análise estática de código com golangci-lint
make build         # Compila binário otimizado para produção
make docker-build  # Constrói imagem Docker baseada em Alpine
```

### Cobertura de Testes (Módulos Principais):
- `internal/tui`: Formatação de métricas e lógica de renderização
- `internal/alerts`: Avaliação do motor de alertas e limites temporais
- `internal/config`: Definição de padrões, carga de YAML e precedência de variáveis de ambiente

---

## 📄 Licença

Distribuído sob a licença [MIT](LICENSE).
