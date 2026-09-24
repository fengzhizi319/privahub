# =============================================================================
# Privahub container image (backend + bundled web console).
#
# Build context: the privahub repository root. The web console lives in a
# separate repository (privaconsole) that must be checked out at ./web:
#
#   git clone https://github.com/fengzhizi319/privaconsole.git web   # if absent
#   docker build -f deployments/docker/Dockerfile -t privahub:dev .
#
# Design:
#   * CGO is required (gorm.io/driver/sqlite -> mattn/go-sqlite3), so the
#     builder is Debian bookworm (glibc) and the runtime is distroless/cc on
#     the SAME Debian release (glibc ABI match). Do not mix with alpine/musl.
#   * Runtime is distroless:nonroot (uid/gid 65532): no shell, no package
#     manager. The HEALTHCHECK uses a dedicated static probe binary.
#   * Read-only root filesystem compatible: everything the server writes lives
#     under the declared VOLUMEs (data, certs, kuscia pool, sync, audit).
#   * Base images are pinned by digest. Refresh with
#       docker buildx imagetools inspect <image:tag>
#     (Renovate/Dependabot "docker" ecosystem can automate this). The CI gate
#     scripts/check_base_image_pins.sh fails on unpinned FROM lines.
# =============================================================================

ARG GO_IMAGE=golang:1.25.14-bookworm@sha256:3b4a11519ad929d1e1d261a12cff056f0c85b735253d7d861346b9c6f8b36437
ARG NODE_IMAGE=node:22-bookworm-slim@sha256:43ac6c60b8f89723f746e8a92ce91abd5017e627ce1ddfe4238355d3a30b772c
ARG RUNTIME_IMAGE=gcr.io/distroless/cc-debian12:nonroot@sha256:9dac0a79194e45a7da0158a9c6da57b217585af0786db3845d1f0ec1a0dd182f

# -----------------------------------------------------------------------------
# Stage 1: web console (privaconsole) -> /web/apps/secretpad/dist
# -----------------------------------------------------------------------------
FROM ${NODE_IMAGE} AS web
# npm registry used for install; the repo's .npmrc defaults to npmmirror.
ARG NPM_REGISTRY=""
ENV HUSKY=0 CI=true
WORKDIR /web
RUN corepack enable
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml web/.npmrc ./
COPY web/apps ./apps
COPY web/packages ./packages
COPY web/tooling ./tooling
COPY web/tsconfig.json ./
RUN if [ -n "$NPM_REGISTRY" ]; then export npm_config_registry="$NPM_REGISTRY"; fi \
    && corepack pnpm install --frozen-lockfile \
    && corepack pnpm run build \
    && test -f apps/secretpad/dist/index.html

# -----------------------------------------------------------------------------
# Stage 2: Go binaries (CGO, glibc)
# -----------------------------------------------------------------------------
FROM ${GO_IMAGE} AS builder
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ENV CGO_ENABLED=1 GOOS=linux GOFLAGS=-trimpath GOTOOLCHAIN=local
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY pkg ./pkg
COPY deployments/docker/healthcheck ./deployments/docker/healthcheck
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go build -ldflags="-s -w -X main.version=${VERSION} -X main.gitCommit=${GIT_COMMIT}" \
        -o /out/privahub ./cmd/server \
    && go build -ldflags="-s -w" -o /out/audit-verify ./cmd/audit-verify \
    && go build -ldflags="-s -w" -o /out/privahub-admin ./cmd/privahub-admin \
    && CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/healthcheck ./deployments/docker/healthcheck
# Pre-create the writable directory layout with the runtime uid so that
# named volumes inherit the correct ownership on first mount.
RUN mkdir -p /out/rootfs/app/data /out/rootfs/app/audit \
             /out/rootfs/app/config/certs /out/rootfs/app/config/kuscia /out/rootfs/app/config/sync \
    && chown -R 65532:65532 /out/rootfs/app \
    && chmod 0750 /out/rootfs/app/data /out/rootfs/app/audit /out/rootfs/app/config/certs \
                  /out/rootfs/app/config/kuscia /out/rootfs/app/config/sync

# -----------------------------------------------------------------------------
# Stage 3: runtime
# -----------------------------------------------------------------------------
FROM ${RUNTIME_IMAGE}
LABEL org.opencontainers.image.title="privahub" \
      org.opencontainers.image.source="https://github.com/fengzhizi319/privahub" \
      org.opencontainers.image.licenses="Apache-2.0"

WORKDIR /app
COPY --from=builder --chown=65532:65532 /out/rootfs/app /app
COPY --from=builder /out/privahub /usr/local/bin/privahub
COPY --from=builder /out/healthcheck /usr/local/bin/healthcheck
# Offline audit hash-chain verifier (docs/security/runbooks/incident-response.md)
COPY --from=builder /out/audit-verify /usr/local/bin/audit-verify
# Offline break-glass admin tool (docs/security/runbooks/break-glass-admin-reset.md)
COPY --from=builder /out/privahub-admin /usr/local/bin/privahub-admin
# Read-only application assets (owned by root, not writable by the server).
COPY config/components.json config/graph.yaml /app/config/
COPY config/components /app/config/components
COPY config/i18n /app/config/i18n
COPY config/privahub.yaml config/privahub-edge.yaml /etc/privahub/
COPY --from=web /web/apps/secretpad/dist /app/web/dist

# Production defaults. Secrets (PRIVAHUB_AUTH_JWT_SECRET,
# PRIVAHUB_CRYPTO_AES_KEY, inner_auth token, ...) MUST be injected at runtime
# from a secret store — never baked into the image. Without them the server
# refuses to start outside PRIVAHUB_PROFILE=dev|test (fail closed).
# DL3064 false positive: *_PASSWORD_FILE is a file path, not a secret value.
# hadolint ignore=DL3064
ENV PRIVAHUB_WEB_DIR=/app/web/dist \
    PRIVAHUB_DATA_DIR=/app/data \
    PRIVAHUB_DATABASE_DSN="/app/data/privahub.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)" \
    PRIVAHUB_AUTH_INITIAL_PASSWORD_FILE=/app/data/initial-admin-password \
    PRIVAHUB_AUDIT_SPOOL_DIR=/app/audit/spool \
    PRIVAHUB_AUDIT_ARCHIVE_DIR=/app/audit/archive \
    PRIVAHUB_OBSERVABILITY_LOG_FORMAT=json

# data: SQLite DB, uploads, initial admin password | audit: audit spool/anchors
# config/certs, config/kuscia, config/sync: runtime-generated node material
VOLUME ["/app/data", "/app/audit", "/app/config/certs", "/app/config/kuscia", "/app/config/sync"]

USER 65532:65532
# 8080: main HTTP API + SPA.  9001: inner (node-to-node / Kuscia) API —
# publish it only on a private network and configure server.inner_auth.
EXPOSE 8080 9001

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD ["/usr/local/bin/healthcheck"]

ENTRYPOINT ["/usr/local/bin/privahub"]
CMD ["-config", "/etc/privahub/privahub.yaml"]
