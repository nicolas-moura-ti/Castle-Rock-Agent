# ============================================================================
# STAGE 1: Build
# ============================================================================
# Multi-stage build — best practice para containers Go:
#   - Stage 1: compila com todas as ferramentas Go (~800MB)
#   - Stage 2: copia apenas o binário para uma imagem mínima (~10MB)
FROM golang:1.25-alpine AS builder

# Instala certificados CA para chamadas HTTPS
RUN apk add --no-cache ca-certificates

WORKDIR /app

# Copia go.mod e go.sum primeiro para cache de dependências.
# Docker cacheia cada layer — se go.mod não mudar, as dependências
# não são baixadas novamente, acelerando builds subsequentes.
COPY go.mod go.sum ./
RUN go mod download

# Copia o código fonte
COPY . .

# Compila o binário estático.
# CGO_ENABLED=0 = binário completamente estático (sem glibc)
# -ldflags "-s -w" = remove debug info, reduz tamanho
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /app/castle-rock-agent \
    ./cmd/agent

# ============================================================================
# STAGE 2: Runtime
# ============================================================================
# Imagem mínima (~5MB) com apenas o binário + configs
FROM alpine:3.19

LABEL maintainer="nicolas-moura-ti"
LABEL description="Castle Rock Agent — Docker Observability Agent"
LABEL version="0.3.0"

# Usuário não-root (princípio de menor privilégio)
RUN addgroup -S agent && adduser -S agent -G agent

WORKDIR /app

# Binário compilado
COPY --from=builder /app/castle-rock-agent .

# Configuração padrão (pode ser overrideada via volume mount)
COPY --from=builder /app/configs ./configs

# Certificados CA + wget para health check
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
RUN apk add --no-cache wget

USER agent

# Expõe a porta padrão do Prometheus exporter
EXPOSE 9110

# HEALTHCHECK verifica se o servidor HTTP de métricas está respondendo.
# O Docker marca o container como "unhealthy" após 3 falhas consecutivas.
# Isso é visível com `docker ps` e usado pelo Docker Compose / Swarm / K8s.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://localhost:9110/health || exit 1

# ENTRYPOINT em forma exec para receber sinais diretamente
ENTRYPOINT ["./castle-rock-agent"]
