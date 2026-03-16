# 📈 Observabilidade e Métricas

O Castle Rock Agent foi projetado para se integrar perfeitamente ao ecossistema moderno de observabilidade Cloud Native: **Prometheus** e **Grafana**.

Usando o arquivo `docker-compose.yml`, esta stack inteira ganha vida automaticamente em segundos.

---

## 📊 Prometheus — Exportador de Métricas

O agente opera também como um exportador (*Prometheus Exporter*), expondo dezenas de métricas nativas do Docker no endpoint `http://127.0.0.1:9110/metrics`.

### Métricas Disponíveis

| Métrica | Tipo (Prometheus) | Descrição |
|---|---|---|
| `castle_rock_container_cpu_percent` | Gauge | Percentual de uso de CPU do container |
| `castle_rock_container_memory_usage_bytes` | Gauge | Memória utilizada instantânea (em bytes) |
| `castle_rock_container_memory_limit_bytes` | Gauge | Limite (ceiling) de memória (em bytes) |
| `castle_rock_container_memory_percent` | Gauge | Percentual de memória gasta |
| `castle_rock_container_network_rx_bytes` | Gauge | Soma de bytes recebidos (Download) |
| `castle_rock_container_network_tx_bytes` | Gauge | Soma de bytes transmitidos (Upload) |
| `castle_rock_container_block_read_bytes` | Gauge | Escrita/Leitura em disco bruto |
| `castle_rock_container_block_write_bytes` | Gauge | Escrita/Leitura em disco bruto |
| `castle_rock_container_info` | Gauge | Metadados persistentes e extração de Labels Docker |

*OBS: Todas essas métricas já são carimbadas com labels de `container_id`, `container_name`, `image`, e o identificador do Host `host_id`.*

### Exemplo de Buscas (PromQL)

```promql
# CPU num container específico
castle_rock_container_cpu_percent{container_name="postgres"}

# Top 5 containers mais pesados na RAM
topk(5, castle_rock_container_memory_percent)

# Tráfego agregado recebido pelo NGINX
sum(castle_rock_container_network_rx_bytes) by (container_name)
```

---

## 📈 Grafana — Nossos 5 Dashboards Nativos

O pacote no Docker Compose provisiona automaticamente toda a base de Data Sources do Grafana e carrega **5 dashboards** pré-prontos na sua instância local. Eles se atualizam na velocidade de varredura pré-definida.

**Acesso:** http://localhost:3000 → Usuário/Senha: `admin` / `castlerock`

1. **Dashboard 1: Visão Geral (Overview):** O "Helicóptero". Média agregada do uso de processador, blocos ranckeados, velocímetros rápidos e barras comparativas.
2. **Dashboard 2: Detalhes Profundos (Zoom-In):** Um menu de Dropdown em cima filtra gráficos puramente históricos para focar em UM container específico e seu histórico pelas últimas horas.
3. **Dashboard 3: Análise Fina de Rede:** Mostra as curvas combinadas de "Download vs. Upload" (RX/TX) separadamente num gráfico unificado; essencial para caçar gargalos.
4. **Dashboard 4: Caçada ao OOM (Vazamento de Memória):** Destaca gráficos que comparam Uso vs. Limites de RAM. Quando a curva de limite se choca com a reta de Limite Estático, um OOM Kill é iminente.
5. **Dashboard 5: Central de Saúde & Alertas:** Dedicado a monitoramento contínuo, exibindo desvios picos de MAX Peak vs AVG Média.

---

## ⚠️ Camada Dupla de Alertas

Para precaver a morte de sistemas rodando a noite, O sistema de Alertas opera sob **duas camadas** protetivas:

### Camada 1: Alertas TUI Internos (Integrados)

Definidos via `configs/config.yaml`, avaliados localmente pelo Agente (que pisca em vermelho no TUI e emite sinos eventuais sem gastar métrica HTTP externa).

```yaml
alerts:
  enabled: true
  rules:
    - name: "Critical CPU"
      metric: "cpu_percent"
      operator: ">"
      threshold: 80.0
      duration: 2m           # Espera a carga persistir por 2 minutos.
      severity: "critical"
```

Ações ativadas por esse alerta:
- 🚨 Pisca o painel de status principal da TUI.
- 📋 Deixa gravada em histórico SQLite local o motivo da falha.
- Insere visualizadores no meio da grade (`🚨` ou `⚠️`).

### Camada 2: Alertas de Nível Enterprise (Prometheus)

Para o time SRE, localizados via YAML puro atrelados ao servidor de monitoria `deploy/prometheus/alert_rules.yml`. Avaliados globalmente pelo motor Prom.

```yaml
- alert: ContainerHighCPU
  expr: castle_rock_container_cpu_percent > 80
  for: 2m
  labels:
    severity: critical
  annotations:
    summary: "High CPU on container {{ $labels.container_name }}"
```

Basta ligar esta camada no conector *Alertmanager* padrão para disparar mensagens no canal do Slack da sua equipe de plataforma (PagerDuty/E-mail/Webhooks).

---

## 🌐 Múltiplos Nós e Arquitetura Cluster

Em grandes implantações operacionais, as fábricas ou as equipes raramente têm a facilidade de observar através de um TUI.

Ao invés disso, o Agente funciona organicamente em sistemas amplos como uma hierarquia em Estrela ("Star"). Uma máquina roda no formato Master (`Leader`) e empacota passivamente requisições em HTTP POST de inúmeros `Workers`.

**Execução Descentralizada Via Linha de Comando:**

```bash
# Máquina Sede Master PromQL Integrada
CASTLE_ROCK_CLUSTER_MODE=leader \
CASTLE_ROCK_CLUSTER_HOST_ID=brasilia \
CASTLE_ROCK_CLUSTER_AUTH_TOKEN="my-secret-token" \
CASTLE_ROCK_CLUSTER_SHARED_SECRET="my-aes-key" \
make run

# Em um Data Center remoto (Roda escondido apenas passando dados)
CASTLE_ROCK_CLUSTER_MODE=worker \
CASTLE_ROCK_CLUSTER_HOST_ID=saopaulo-b \
CASTLE_ROCK_CLUSTER_LEADER_URL=http://<IP_DE_BRASILIA>:9110 \
CASTLE_ROCK_CLUSTER_AUTH_TOKEN="my-secret-token" \
CASTLE_ROCK_CLUSTER_SHARED_SECRET="my-aes-key" \
make run
```

E no Grafana ou no TUI do Mestre, você conseguirá inspecionar cada "Host" usando um novo filtro da grade unificada.
