<div align="center">

# 🏰 Castle Rock Agent

[![Go Report Card](https://goreportcard.com/badge/github.com/nicolas-moura-ti/castle-rock-agent?style=flat-square)](https://goreportcard.com/report/github.com/nicolas-moura-ti/castle-rock-agent)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg?style=flat-square)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-v1.24.2-blue?style=flat-square&logo=go)](https://github.com/nicolas-moura-ti/castle-rock-agent)

\
*Leia em outros idiomas: [English](README.md) · [Português](README.pt-BR.md)*

> Agente de observabilidade nativo em Go para monitoramento de containers Docker com dashboard interativo, métricas Prometheus e alertas configuráveis.

</div>

---

### ⛰️ Por que "Castle Rock"?
Inspirado nas torres de vigia medievais erguidas sobre rochedos (Castle Rocks), que ofereciam **visão panorâmica absoluta** de tudo que acontecia ao redor do castelo. Assim como essas torres, este agente fica em um ponto de observação privilegiado (o Docker Socket) para monitorar, vigiar e alertar sobre a saúde de toda a sua infraestrutura de containers.

---

## 📖 Documentação Completa por Módulos

Para mergulhar em detalhes técnicos e funcionalidades específicas, por favor consulte nossa pasta de documentações oficias:

- 🖥️ **[Painel Interativo TUI & Referência de Operação](docs/TUI.pt-BR.md)** 
  *(Stress Test, Acesso a Shell `exec`, Limpeza Docker Prune, Live Tail & Grep, Diagnóstico Integrado)*
- 📈 **[Observabilidade (Prometheus & Grafana) & Motores de Alerta](docs/OBSERVABILITY.pt-BR.md)** 
  *(Lista das Enumerações de Métricas, Detalhe dos 5 Dashboards Grafana, Modo Multi-Cloud/Cluster, Conexão e Regras de Alerta Prom)*
- ⚙️ **[Configuração e Variáveis do Kernel](docs/CONFIGURATION.pt-BR.md)**
  *(Especificação do `config.yaml`, Sobrescrita e Regras Hierárquicas 12-Factor, Precedências de Ambientes Injetáveis)*

---

## 🧠 Como Funciona — Explicação Rápida

O monitoramento do agente depende da integração com outras três chaves de observadores:

1. **Castle Rock Agent (este pacote Binário):** Extrai a alma performática do sistema conectando aos arquivos de Soquete do Docker e agindo como Coletor nativo, em repasse ao interpretador.
2. **Prometheus:** Exerce trabalho escravo como o seu **Bando de Dados em Tempo Real e Histórico**. Acorda de 5s em 5s e intercepta o repassado do endpoint HTTP (`http://127.0.0.1:9110/metrics`), guardando isso sob sua própria carimbagem do tempo.
3. **Grafana:** O Painel Interativo de Relatores a Visitação do usuário onde extrairá o prometheus e transformará em **Desenhos Gráficos Visuais.**

```
Containers Nativos →  Castle Rock Agent  →   Prometheus DB Base  →  Dashboards Grafana WEB 
(Geram os danos)      (Coleta e formata)     (Armazena no tempo)    (Plástico visual a você)
```

---

## ✨ Principais Diferenciais (Features)

| Funcionalidade | Descrição Curta |
|---|---|
| **TUI Dinâmica** | Console em Shell Extensa (Fullscreen) preenchedora com Live Tables integradas. |
| **Ponto Exato em Tempo Efetivo** | Porcentuais instantâneos base de Leitura de Disco I/O e Placas de Rede acopladas limitadas. |
| **Templates Nativos Grafana** | Cinco layouts previamente escritos de Painéis analíticos e tabelas interligadas do ecossistema Grafana. |
| **Dispensador de Alertas Dual** | Notificações Operacionais que piscam cores críticas locais além dos gatilhos Webhooks externos do Prometheus integráveis. |
| **Central de Nodos em Cluster 🌐** | Capacidade Distributiva Multi-Server ligando Agentes Escravos por Polling repassando para um Painel Único de Matriz.|
| **Auditoria Tática Anti-Válvula 🛡️** | Rastreador Real-Time Shift-Left expondo "Vistas Prontas" críticas em containers como permissão de Roots não mascarada ou falta cênica em limitações preventivas. |
| **Lavador Ocioso Nativo 🧹** | Expurgo preventivo automático atrelado por Garbage Collector que deleta as lamas digitais do Docker para salvar a partição raiz. |

---

## Topologia de Rede e Arquitetura do Serviço Base

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
│         │   Motor SDK API  │         └──┤ Nó Excedente (Op) │  │
│         │  (Coleta Docker) │            └───────────────────┘  │
│         └─────────┬────────┘                                   │
│                   │                                            │
└───────────────────┼────────────────────────────────────────────┘
                    ▼
            Sistema Físico Docker Interno Root
      (unix:///var/run/docker.sock)
```

---

## O Que É Necessário Antes (Prerequisitos)

| Binários ou Serviços Básicos | Suporte Minimo | 
|---|---|
| **Go** | 1.24+ | 
| **Docker** | 20.10+ | 
| **Make** | Qualquer lib clássica | 

*(Aos amigos adeptos da Maçã - Em Mac OS atente ao consentimento silencioso de terminal da devida C tools Xcode com: `sudo xcodebuild -license accept`)*

---

## Primeiros Passos e Rodando: Quick Boot

### Formato #1: Modo Local Console Gráfico TUI (Simples Teste/Dev)

Acopla instantaneamente o motor visual da Castle na linha de comando nativa. Operação limpa onde só fará sentido para o olho local testando.

```bash
# Baixando
git clone https://github.com/nicolas-moura-ti/castle-rock-agent.git
cd castle-rock-agent

# Aciona Interface (Entrando Tela TUI Visão Master)
make run
```

### Formato #2: O Modo Definitivo Docker Compose (Central de Monitoria Mestra)

Eleva de forma isolada do HD três pilares atrelados para atuar 24/7 horas sobre a sua Nuvem sem fechar, usando apenas portas expostas (**O Castle entra no modo sombra (Headless) recolhendo silenciosamente dados ao Prome**).

```bash
docker compose up -d

# Visualização no Navegador:
# - A sua Central Grafana em Geral: http://127.0.0.1:3000  (admin / castlerock)
# - O Banco Analítico Primitivo Prom:  http://127.0.0.1:9090
# - Ver Endpoint de saída Castle Crua  http://127.0.0.1:9110/metrics
```

> 💡 **USO CONJUNTO E SÁBIO DO SISTEMA:** Nunca mate o `docker compose up -d` da máquina! Deixe as duas instâncias conviverem em paralelo! 
Sua grade Web do grafana ficará recolhendo infinitamente histórico métrico analítico 24/7/365 enquanto você de sossego abre seu TUI esporádico `make run` local com o chefe num fim de expediente unicamente pontual para rastrear vazamentos de Memória local.

---

## 🧪 Base Coberta Analítica (Desenvolvimento e Setup)

```bash
make test          # Cobertura contra corrida de Race Condicional (-race)
make lint          # Análise profunda Go contra code smell GoVet (Static Analysis vet)
make build         # Cria o tijolo empacotado Binário super pequeno (Optimized Binary)
make docker-build  # Compila uma imagem Alpine com seu Castle dentro pronta.
```

### Resposta e Cobertura Base a Nível Go Module
- `internal/tui`: Alojamento visual sobre transformações lógicas numéricas dos bytes capturados do Socket Host Docker no formato gráfico via LipGloss.
- `internal/alerts`: Análise de validações assíncronas no contexto background (Goroutines).
- `internal/config`: Precessadores limpos priorizando Injeções Ambientais contra o Root e Arquivo Serializado YAML em memória.

---

## Licença Base

Aprovado via MIT. Produzido ao povo em aberto.
