# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install git for the versioning command
RUN apk add --no-cache git

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the rest of the application
COPY . .

# Build the application, injecting the version from git
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-X main.AppVersion=0.1.$(git rev-list --count HEAD 2>/dev/null || echo '0')" -o mimic ./cmd/mimic

# Final stage
FROM alpine:latest

WORKDIR /app

# Add required packages
RUN apk --no-cache add ca-certificates tzdata

# Copy the binary from the builder stage
COPY --from=builder /app/mimic .

# Copy necessary directories
COPY --from=builder /app/templates ./templates
COPY --from=builder /app/static ./static

# Expose the application port
EXPOSE 3000

# Run the application
CMD ["./mimic"]
