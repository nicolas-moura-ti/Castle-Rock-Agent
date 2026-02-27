# ============================================================================
# Castle Rock Agent — Makefile
# ============================================================================
# Este Makefile fornece targets padronizados para build, teste e deploy.
# Usar Makefiles é uma prática comum em projetos Go para padronizar
# comandos entre diferentes ambientes e CI/CD pipelines.
# ============================================================================

# Variáveis
APP_NAME    := castle-rock-agent
CMD_PATH    := ./cmd/agent
BUILD_DIR   := ./bin
GO          := go
GOFLAGS     := -v
LDFLAGS     := -s -w  # Strip debug info para binário menor

# Detecta o sistema operacional para compatibilidade
UNAME_S := $(shell uname -s)

# ─────────────────────────────────────────────────────────────────────────────
# TARGETS PRINCIPAIS
# ─────────────────────────────────────────────────────────────────────────────

.PHONY: all build run test lint clean docker-build docker-run compose-up compose-down help

## all: Build padrão (default target)
all: build

## build: Compila o binário otimizado para produção
build:
	@echo "🔨 Compilando $(APP_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME) $(CMD_PATH)
	@echo "✅ Binário gerado em $(BUILD_DIR)/$(APP_NAME)"

## run: Compila e executa o agente
run:
	@echo "🚀 Executando $(APP_NAME)..."
	$(GO) run $(CMD_PATH)

## test: Executa todos os testes com cobertura
test:
	@echo "🧪 Executando testes..."
	$(GO) test -race -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out
	@echo "✅ Testes concluídos"

## lint: Executa análise estática (golangci-lint ou go vet)
lint:
	@echo "🔍 Executando análise estática..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "⚠️  golangci-lint não encontrado, usando go vet..."; \
		echo "   Instale com: brew install golangci-lint"; \
		$(GO) vet ./...; \
	fi
	@echo "✅ Análise concluída"

## clean: Remove artefatos de build
clean:
	@echo "🧹 Limpando artefatos..."
	@rm -rf $(BUILD_DIR) coverage.out coverage.html
	@echo "✅ Limpo"

## tidy: Organiza dependências do go.mod
tidy:
	@echo "📦 Organizando dependências..."
	$(GO) mod tidy
	@echo "✅ go.mod atualizado"

# ─────────────────────────────────────────────────────────────────────────────
# DOCKER TARGETS
# ─────────────────────────────────────────────────────────────────────────────

## docker-build: Constrói a imagem Docker
docker-build:
	@echo "🐳 Construindo imagem Docker..."
	docker build -t $(APP_NAME):latest .
	@echo "✅ Imagem $(APP_NAME):latest criada"

## docker-run: Executa o container Docker
docker-run:
	@echo "🐳 Executando container..."
	docker run --rm -v /var/run/docker.sock:/var/run/docker.sock $(APP_NAME):latest

## compose-up: Sobe stack completa (Agent + Prometheus + Grafana)
compose-up:
	@echo "🐳 Subindo stack de observabilidade..."
	docker compose up -d --build
	@echo "✅ Stack rodando:"
	@echo "   Grafana:    http://localhost:3000  (admin/castlerock)"
	@echo "   Prometheus: http://localhost:9090"
	@echo "   Métricas:   http://localhost:9110/metrics"

## compose-down: Para a stack completa
compose-down:
	@echo "🛑 Parando stack..."
	docker compose down

# ─────────────────────────────────────────────────────────────────────────────
# HELP
# ─────────────────────────────────────────────────────────────────────────────

## help: Exibe esta mensagem de ajuda
help:
	@echo "Castle Rock Agent — Targets disponíveis:"
	@echo ""
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
