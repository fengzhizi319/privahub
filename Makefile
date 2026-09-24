.PHONY: build build-all run test test-race test-cover lint fmt fmt-check vet tidy wire clean docker proto help \
        check sec vuln secrets sbom fuzz openapi openapi-check \
        gate-check sqli-check redact-check image-check k8s-check hadolint pre-commit-install

# Build variables
APP_NAME := privahub
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.gitCommit=$(GIT_COMMIT) -X main.buildTime=$(BUILD_TIME)"

# Go parameters. CGO is REQUIRED: the SQLite driver (gorm.io/driver/sqlite ->
# mattn/go-sqlite3) is cgo-only; a CGO_ENABLED=0 binary fails at runtime.
GOCMD := go
GOBUILD := CGO_ENABLED=1 $(GOCMD) build
GOTEST := CGO_ENABLED=1 $(GOCMD) test
GOMOD := $(GOCMD) mod

# ── Security tool versions ─────────────────────────────────────────────────
# Keep in sync with .github/workflows/ci.yml. Tools run via `go run <mod>@<ver>`
# when not on PATH, so `make check` works without global installs and never
# silently skips a gate because a tool is missing.
GOLANGCI_LINT_VERSION := v2.12.2
GOVULNCHECK_VERSION   := v1.7.0
GITLEAKS_VERSION      := v8.30.1
CYCLONEDX_VERSION     := v1.12.0

GOLANGCI_LINT ?= $(shell command -v golangci-lint 2>/dev/null || echo "$(GOCMD) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)")
# govulncheck reports standard-library vulnerabilities of the toolchain that
# runs it: pin it to go.mod's `toolchain` (CI installs exactly that version).
GO_TOOLCHAIN  := $(shell sed -n 's/^toolchain //p' go.mod)
GOVULNCHECK   ?= GOTOOLCHAIN=$(GO_TOOLCHAIN) $(GOCMD) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
GITLEAKS      ?= $(shell command -v gitleaks 2>/dev/null || echo "$(GOCMD) run github.com/zricethezav/gitleaks/v8@$(GITLEAKS_VERSION)")
FUZZTIME      ?= 30s

## build: Compile the server binary (CGO, required for SQLite)
build:
	$(GOBUILD) $(LDFLAGS) -o bin/$(APP_NAME) ./cmd/server

## build-all: Compile all binaries (server, migrator, edge-agent, audit-verify, privahub-admin)
build-all:
	$(GOBUILD) $(LDFLAGS) -o bin/$(APP_NAME) ./cmd/server
	$(GOBUILD) $(LDFLAGS) -o bin/secretpad-migrator ./cmd/migrator
	$(GOBUILD) $(LDFLAGS) -o bin/secretpad-edge-agent ./cmd/edge-agent
	$(GOBUILD) -o bin/audit-verify ./cmd/audit-verify
	$(GOBUILD) -o bin/privahub-admin ./cmd/privahub-admin

## run: Build and run the server locally (dev profile)
run: build
	PRIVAHUB_PROFILE=dev ./bin/$(APP_NAME) -config ./config/privahub.yaml

## test: Run all unit tests with the race detector
test:
	$(GOTEST) -race -count=1 -coverprofile=coverage.out ./...

test-race: test

## test-cover: Run tests with coverage report
test-cover: test
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

## fmt: Format all Go source files
fmt:
	$(GOCMD) fmt ./...
	$(GOLANGCI_LINT) fmt ./...

## fmt-check: Fail if any file is not gofmt-clean
fmt-check:
	@gofmt="$$($(GOCMD) env GOROOT)/bin/gofmt"; [ -x "$$gofmt" ] || { echo "gofmt not found" >&2; exit 1; }; \
	files=$$(git ls-files --cached --others --exclude-standard -- '*.go'); [ -n "$$files" ] || { echo "no Go files" >&2; exit 1; }; \
	out=$$("$$gofmt" -l $$files) || exit 1; \
	if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi; echo "[OK] gofmt clean"

## vet: go vet
vet:
	CGO_ENABLED=1 $(GOCMD) vet ./...

## lint: golangci-lint (incl. gosec) with .golangci.yml
lint:
	CGO_ENABLED=1 $(GOLANGCI_LINT) run ./...

## vuln: govulncheck — fails on vulnerabilities reachable from our code
vuln:
	CGO_ENABLED=1 $(GOVULNCHECK) ./...

## secrets: gitleaks over the full git history (+ working tree)
secrets:
	$(GITLEAKS) git --config .gitleaks.toml --redact --no-banner --exit-code 1 .
	$(GITLEAKS) dir --config .gitleaks.toml --redact --no-banner --exit-code 1 .

## sbom: CycloneDX SBOM of the Go module graph -> sbom.cdx.json
sbom:
	$(GOCMD) run github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@$(CYCLONEDX_VERSION) mod -licenses -json -output sbom.cdx.json .

## fuzz: fuzz smoke for every FuzzXxx target (FUZZTIME=30s)
fuzz:
	FUZZTIME=$(FUZZTIME) bash scripts/check_fuzz_smoke.sh

## openapi: Regenerate internal/controller/http/apidocs/openapi.json
openapi:
	CGO_ENABLED=1 $(GOCMD) run ./cmd/openapi-gen

## openapi-check: Fail if the committed OpenAPI document is stale
openapi-check:
	@bash scripts/check_openapi_drift.sh

## gate-check: Meta-gate — gates must not silently pass when tools are missing
gate-check:
	@bash scripts/check_gate_failclosed.sh

## sqli-check: GORM SQL-injection guard
sqli-check:
	@bash scripts/check_sql_injection_guard.sh

## redact-check: No credentials / Authorization values in logs
redact-check:
	@bash scripts/check_log_redaction.sh

## image-check: Dockerfile base images pinned by digest, no :latest
image-check:
	@bash scripts/check_base_image_pins.sh

## k8s-check: K8s securityContext / NetworkPolicy / PSA + compose port binding
k8s-check:
	@bash scripts/check_k8s_manifests.sh

## hadolint: Lint the Dockerfile (requires hadolint or docker)
hadolint:
	@if command -v hadolint >/dev/null 2>&1; then hadolint deployments/docker/Dockerfile; \
	elif command -v docker >/dev/null 2>&1; then docker run --rm -i hadolint/hadolint:v2.15.1@sha256:32dac94127fd60b7b7e3fbfc65e1383b9b5e25c9bfd7b8536de7a539fe68a12d < deployments/docker/Dockerfile; \
	else echo "hadolint: install hadolint or docker" >&2; exit 1; fi

## sec: Security gates (static guards + secrets + vulnerabilities)
sec: gate-check sqli-check redact-check image-check k8s-check secrets vuln

## check: Everything CI runs for the Go backend (except fuzz/docker/trivy)
check: gate-check fmt-check vet lint sqli-check redact-check image-check k8s-check openapi-check test vuln secrets

## pre-commit-install: Install the git pre-commit hooks (.pre-commit-config.yaml)
pre-commit-install:
	pre-commit install --install-hooks

## tidy: Download and tidy module dependencies
tidy:
	$(GOMOD) tidy

## wire: Generate dependency injection code
wire:
	cd internal/wire && wire

## clean: Remove build artifacts
clean:
	rm -rf bin/ coverage.out coverage.html sbom.cdx.json

## docker: Build the Docker image (web/ must be a privaconsole checkout)
docker:
	docker build -t $(APP_NAME):$(VERSION) --build-arg VERSION=$(VERSION) --build-arg GIT_COMMIT=$(GIT_COMMIT) \
		-f deployments/docker/Dockerfile .

## proto: Generate protobuf code (requires protoc + plugins)
proto:
	protoc --go_out=. --go-grpc_out=. api/proto/v1/*.proto

## help: Show this help message
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | column -t -s ':'
