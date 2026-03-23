# 🖥️ TUI — Dashboard Interativo

Quando você roda `make run`, o agente abre um dashboard fullscreen, desenvolvido puramente em terminal, para monitoramento e operações em tempo real no Docker.

```
 🏰 Castle Rock Agent    v0.3.0 │ Docker 29.2.1 │ ⏱ 2m30s │ 📡 5 events

    ID             NOME                 CPU%    MEM%    MEM       NET ↓/↑     ESTADO
 ▸  9b157cd0abf6   postgres             2.3%    45.2%   350MB     12M/5M      ● up
    abc123def456   redis                0.1%    12.8%   98MB      3M/1M       ● up
    fed987654321   nginx                0.5%    5.1%    42MB      50M/45M     ● up

 📋 Events
  ╭──────────────────────────────────────────────────────╮
  │  16:05:32 🟢 start      nginx                       │
  │  16:05:30 📦 create     nginx                       │
  │  16:04:15 🔴 die        old-container               │
  ╰──────────────────────────────────────────────────────╯

  ↑↓ nav │ space select │ enter details │ l log 1 │ L log N │ a all │ x shell │ C prune │ s stop │ R restart │ S stress │ M map │ ? help │ q quit
```

> 💡 **Barra de Ajuda Contextual:** A barra inferior muda seus atalhos automaticamente dependendo da tela atual. Por exemplo, ao visualizar logs, ela mostra `↑↓ scroll │ / grep │ f tail │ E export │ Esc back`. No Dashboard de Prune, mostra `[i] images │ [v] volumes │ Esc back`.

---

## Atalhos de Teclado

| Tecla | Ação |
|---|---|
| `↑` / `k` | Navegar para cima (ou scroll nos logs) |
| `↓` / `j` | Navegar para baixo (ou scroll nos logs) |
| `Enter` | Expandir detalhes curados do container (métricas, labels, networks, ports) |
| `a` / `A` | Ligar/desligar a visualização de containers inativos/parados |
| `l` | Ligar/desligar logs em tempo real do container **selecionado** |
| `Espaço` | Selecionar/Deselecionar um container para Multi-Tailing |
| `Shift+L` | Juntar logs de **todos os selecionados** em um lugar só |
| `/` | Live Grep (filtro rápido) nos logs |
| `f` | Voltar para o fim dos logs (auto-scroll) |
| `E` | Exportar logs atuais para um arquivo temporário seguro (`/tmp/castle-rock-logs-*.txt`) |
| `x` | Abrir Shell Interativo dentro do container (`/bin/sh` ou `/bin/bash`) |
| `C` | Dashboard Interativo de Prune (Limpeza instantânea de imagens/volumes) |
| `s` | **Stop** container (pede confirmação com `y`) |
| `R` | **Restart** container (pede confirmação com `y`) |
| `S` | **Modo de Stress Test** (CPU/Memória para simular vizinho barulhento) |
| `M` | **Service Map** (Topologia visual de redes Docker) |
| `r` | Refresh manual da lista |
| `?` | Exibir/ocultar ajuda detalhada |
| `Esc` | Fechar painéis abertos e voltar |
| `q` / `Ctrl+C` | Encerrar o agente com desligamento gracioso |

---

## Cores e Ícones

### Sinais de Uso de Recursos
| Cor | Significado |
|---|---|
| 🟢 Verde | Normal (CPU < 40%, MEM < 50%) |
| 🟡 Amarelo | Cuidado (CPU 40-80%, MEM 50-80%) |
| 🔴 Vermelho | Crítico (CPU > 80%, MEM > 80%) |

### Ícones de Status
| Ícone | Significado |
|---|---|
| 🚨 | **Alerta Crítico:** Ultrapassou os limites máximos configurados |
| ⚠️ | **Alerta de Atenção:** Ultrapassou limites de warning |
| 🛡️ | **Aviso de Segurança:** Anti-pattern detectado (ex. modo root, sem limites) |
| ❤️ | **Saudável:** Healthcheck do Docker está passando |
| 🩺 | **Doente (Unhealthy):** Healthcheck está falhando (requer atenção!) |
| ⏳ | **Iniciando:** Container ainda está subindo (healthcheck inicializando) |

---

## ⚡ Modo de Stress Test (Vizinho Barulhento)

O agente possui uma função didática embutida para estressar a máquina e fazer as métricas/alertas dispararem no Grafana em tempo real.

Ao pressionar `S` no TUI, você cria um container temporário construído via código (`alpine` + `stress-ng` nativo) que injeta carga no host:
- `c` **CPU:** Estressa 2 núcleos a 100%
- `m` **Memória:** Aloca e trava exatamente 256MB
- `b` **Ambos:** Aplica carga dupla

*O container dura exatamente 30 segundos e se autodestrói (`AutoRemove: true`), então você pode ver a curva de alerta disparando e, logo depois, se recuperando.*

### ⚠️ Limitações do Stress Test
- **Máquina Virtual vs Host:** No macOS e Windows (Docker Desktop), a carga máxima apontará para os limites de recurso da Virtual Machine do Docker, não a máquina física real.
- **Conectividade:** Exige internet na 1ª execução para baixar a imagem `alpine`.

---

## 📜 Advanced Logs Viewer

A TUI inclui um **Visualizador de Logs Avançado** nativo repleto de recursos modernos para que você não precise sair do painel.

### 📖 Como ler os Logs (`l`, `L`, `f`)

- **`l` (Log de 1 Container):** Coloque a setinha `▸` sobre um container e pressione `l` (L minúsculo) para acompanhá-lo.
- **`L` (Log Multi/Agrupado):** Aperte espaço para selecionar vários. Depois aperte `Shift+L`. O TUI agregará as saídas, separando por tags de `[container]`.
- **`f` (Follow):** A tela corre sozinha. Se apertar pra cima `↑`, ela pausa. Para voltar a seguir, aperte `f`.

### Funções Chave
1. **🔍 Live Grep (Busca Rápida):** Pressione `/` e digite para filtrar logs em tempo real na tela.
2. **🔀 Multi-Tailing:** Acompanhe `N` containers agrupados.
3. **⏪ Paginação do Histórico:** Volte atrás na história com `↑/↓` (ou `k/j`).
4. **🎨 Highlighting de JSON:** O visualizador colore automaticamente saídas JSON em `error`, `warn` e `info`.
5. **⏱️ Timestamps Precisos:** Renderiza o tempo exato em ISO8601 acinzentado no início da linha para correlacionar logs com gráficos.
6. **📤 Quick Export:** Aperte `E` enquanto vê um log para exportar um retrato daquele buffer instantaneamente para um arquivo temporário seguro em `/tmp` (ex: `castle-rock-logs-[nome]-[random].txt`).

---

## 💼 Ferramentas Avançadas

Apesar de ser de terminal, o agente incorpora funcionalidades de ferramentas de operação premium:

### 1. 🖥️ Saúde do Host (Máquina Host)
O agente reporta a CPU/Memória da Máquina Hospedeira física (Node) no painel esquerdo. Ao invés de adivinhar por que o cluster está lento a partir do Docker puro, você enxerga a RAM total logo de cara.

### 2. 💻 Shell Interativo Rápido (`x`)
Ao invés de digitar `docker exec -it ID /bin/sh`, apenas posicione-se no container e aperte `x`. O TUI se esconde temporariamente, lhe dá um prompt real no container e, ao digitar `exit`, te devolve suavemente ao painel de monitoramento.

### 3. 🧹 Dashboard de Limpeza (Prune)
Se ficar sem espaço em disco, aperte `C`. Ele abre o painel interativo de prune, dizendo com precisão o peso Gygabytes das Imagens Orfãs e Volumes Persistentes ociosos. Aperte `i` ou `v` para evictá-los em tempo-real.

---

## 🔎 Diagnósticos Fáceis (Detalhes)

Se apertar `Enter`, o painel exibe:

1. **🔎 Inspetor de Variáveis de Ambiente:** Mostra as env vars passadas ao container (até as com segredos ofuscados). Excelente para debugar strings erradas de conexão de banco.
2. **🏥 Badges de Health Check:** Traz a saída `stdout` do script de health check do docker-compose para saber rapidamente pq algo está quebrando.
3. **📂 Bind Mounts Rápidos:** Mostra o clássico `origem → destino`.
4. **🔄 Restart Policy & Crash Count:** Acende vermelho `(crashed X vezes)` se houver loops contínuos de restart.
5. **🚧 Alerta de Limites:** Aponta em vermelho brilhante caso os containers não tenham memory_limits — evitando catástrofes de OOM nas máquinas da firma.
