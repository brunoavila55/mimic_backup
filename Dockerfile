# Frontend build
FROM node:24-alpine AS frontend-builder

WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Go build
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Baixar dependências primeiro para aproveitar o cache do Docker
COPY go.mod go.sum ./
RUN go mod download

# Copiar o restante do código e compilar
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o mimic_bin ./cmd/mimic/main.go

# Estágio Final (Imagem menor)
FROM alpine:latest

# Instalar dependências de fuso horário
RUN apk add --no-cache tzdata
ENV TZ=America/Sao_Paulo

WORKDIR /app

# Copiar o binário compilado do estágio anterior
COPY --from=builder /app/mimic_bin .

# Copiar pastas estáticas, caso a aplicação precise renderizar HTML ou assets
COPY --from=builder /app/static ./static
COPY --from=builder /app/templates ./templates
COPY --from=frontend-builder /app/frontend/dist ./frontend/dist

# Use the React routes while keeping every legacy handler in the binary.
ENV SPA_ENABLED=true

# Executar como usuário não-privilegiado
RUN addgroup -S mimic && adduser -S mimic -G mimic && chown -R mimic:mimic /app
USER mimic

# Expor a porta
EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:3000/health || exit 1

# Executar a aplicação
CMD ["./mimic_bin"]
