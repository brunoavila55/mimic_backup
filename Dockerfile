# Estágio de Build
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Baixar dependências primeiro para aproveitar o cache do Docker
COPY go.mod go.sum ./
RUN go mod download

# Copiar o restante do código e compilar
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o mimic_bin ./cmd/mimic/main.go

# Estágio Final (Imagem menor)
FROM alpine:latest

WORKDIR /app

# Copiar o binário compilado do estágio anterior
COPY --from=builder /app/mimic_bin .

# Copiar pastas estáticas, caso a aplicação precise renderizar HTML ou assets
COPY --from=builder /app/static ./static
COPY --from=builder /app/templates ./templates

# Expor a porta
EXPOSE 3000

# Executar a aplicação
CMD ["./mimic_bin"]
