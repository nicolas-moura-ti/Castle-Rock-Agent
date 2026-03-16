# ⚙️ Configuração & Setup

O Castle Rock Agent adota a metodologia **12-Factor App**. A configuração do sistema é estritamente separada do código binário e fortemente dependente de variáveis de ambiente.

A carga de dados é executada via Viper, possuindo como fontes oficiais um arquivo de configuração no disco (YAML) e sobreposições (Overrides) via Variáveis de Ambiente.

---

## O Arquivo base `configs/config.yaml`

Por padrão, a compilação local transporta no seu pacote um arquivo `configs/config.yaml` descrevendo o funcionamento sem intervenção ou modificação.

```yaml
log_level: "info"           # debug, info, warn, error

prometheus:
  enabled: true
  port: 9110                # Servidor HTTP Exporter

stats:
  interval: 5s              # Ritmo Cardíaco (Loop) de amostragem no docker.

alerts:
  enabled: true
  rules:                    # Lista de regras internas integráveis do TUI
    - name: "Critical CPU"
      metric: "cpu_percent"
      operator: ">"
      threshold: 80.0
      duration: 2m
      severity: "critical"
```

---

## Variáveis do Sistema de Operação (ENV Variáveis)

As Variáveis passadas nativamente a máquina possuem **prioridade absoluta**, anulando para aquele boot as restrições declaradas do `config.yaml`.
O ambiente, dessa forma, pode governar inteiramente como o script subirá dispensando mapeamento de diretório.

| Variável | Descrição | Default |
|---|---|---|
| `CASTLE_ROCK_LOG_LEVEL` | Verbosity (`debug`, `info`, `warn`, `error`) | `info` |
| `CASTLE_ROCK_PROMETHEUS_PORT` | Porta Exporter HTTP | `9110` |
| `CASTLE_ROCK_PROMETHEUS_ENABLED` | Ligar/desligar interface de rastreamento | `true` |
| `CASTLE_ROCK_STATS_INTERVAL` | Intervalo Ex: `5s` | `5s` |
| `CASTLE_ROCK_STATS_INCLUDE_CONTAINERS` | Expressão/lista isolando containers a espionar | `""` (Lista vazia=TODOS) |
| `CASTLE_ROCK_ALERTS_ENABLED` | Motor de verificação e piscas de status interno | `true` |
| `CASTLE_ROCK_MODE` | Excluir tela e interface (Modo Fantasma/Docker) | `""` (TUI Ligado) |
| `CASTLE_ROCK_CLUSTER_MODE` | Papel Master/Worker: `standalone`, `leader`, `worker` | `standalone` |
| `CASTLE_ROCK_CLUSTER_HOST_ID` | Identificador a aparecer nas abas locais ou Mestra | Host OS Name |
| `CASTLE_ROCK_CLUSTER_LEADER_URL` | Se worker, IP ou endpoint a entregar dados HTTP | `http://127.0.0.1:9110` |
| `CASTLE_ROCK_CLUSTER_SHARED_SECRET` | Chave p/ Criptografia de Payload (**AES-256-GCM**) | `""` |
| `CASTLE_ROCK_CLUSTER_AUTH_TOKEN` | Token na Autenticação tipo Bearer (API) | `""` |

---

## ⚡ Otimização de Performance: Cache de Metadados

Para minimizar o uso de CPU e a sobrecarga na API do Docker, o agente implementa um **Cache de Metadados Estáticos**.

- **Como funciona:** Dados pesados e que raramente mudam, como `Entrypoint`, `Env`, `Mounts` e `Image`, são buscados apenas uma vez quando o container é detectado pela primeira vez.
- **Benefícios:** Reduz drasticamente as operações de Unmarshal de JSON e a E/S com o daemon do Docker em ambientes com centenas de containers.
- **Nota:** Se você alterar as variáveis de ambiente de um container manualmente via Docker, será necessário reiniciar o container para que o agente detecte os novos metadados.

---

## Variáveis de Interceptação da Docker Engine (Defaults)

Da mesma foram como conversa nativamente com a engine do ambiente Docker, ele respeita a tabela padrão do SDK Go do Docker:

| Variável Herdada | Descrição do SDK | Presunção Padronizada |
|---|---|---|
| `DOCKER_HOST` | Soquete a conversar e puxar dados | `unix:///var/run/docker.sock` |
| `DOCKER_API_VERSION` | Falar versão restritiva com Moby (Ex. `1.30`) | Busca automática combinando Daemon |

---

## Ordem Imutável de Precedência (Herança)

Para desvendar porquê uma ferramenta tomou a decisão de ignorar a sua restrição em YAML se um ambiente foi montado via linha de comando ou Orquestrador Docker na Nuvem:

1. A Configuração de Fábrica e Assuntos Vitais de compilação (**Hardcoded em Go**).
2. Propriedades sobrescritas do HD se atrelado fisicamente ou volumado em container (**`configs/config.yaml`**).
3. E finalmente as regras de Ouro injetadas pelo ambiente nativo com o prefixo da ferramenta (**`CASTLE_ROCK_*` Variáveis**).

---

## 🎯 Monitoramento Seletivo por String (Filtragem)

Em configurações brutas, a interceptação ocorrerá com sucesso nos recursos da máquina monitorando **absolutamente todo** container ativo existente - consumindo para esse esforço, espaço de rede ou recursos de compilação.

No caso em Nuvem/Produção, isolar o objeto de interesse evita corromper relatórios Prometheus e Grafana poluídos por sidecars corporativos com dados "inúteis" de infraestrutura da provedora.

Ajuste ou trave os containers de interesse apenas indicando "uma parte", a string de interesse. Se a frase do nome bater com algum de sua lista o dashboard adotara a entidade.

**Via config.yaml**:
```yaml
stats:
  interval: 5s
  include_containers: ["postgres", "redis", "meu-backend-api"]
```

**Via Passagem Expressa Direta Shell (Separados por vírgulas)**:
```bash
# Espiará puramente a Base e a cache ignorando demais
CASTLE_ROCK_STATS_INCLUDE_CONTAINERS="postgres,redis"
```
