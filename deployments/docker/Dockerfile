# Multi-stage Dockerfile for Privahub
# Produces a minimal static binary image (< 25MB)

# Stage 1: Build
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=$(git describe --tags --always 2>/dev/null || echo dev)" \
    -o /privahub ./cmd/server

# Stage 2: Runtime
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 1000 privahub

COPY --from=builder /privahub /usr/local/bin/privahub
COPY config/privahub.yaml /etc/privahub/privahub.yaml

USER privahub
EXPOSE 8080 9090 9091

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
    CMD wget -qO- http://localhost:8080/api/v1alpha1/healthz || exit 1

ENTRYPOINT ["privahub"]
CMD ["-config", "/etc/privahub/privahub.yaml"]
