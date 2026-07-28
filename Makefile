.PHONY: build run test lint clean docker wire tidy

# Build variables
APP_NAME := privahub
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.gitCommit=$(GIT_COMMIT) -X main.buildTime=$(BUILD_TIME)"

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOMOD := $(GOCMD) mod

## build: Compile the server binary
build:
	CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o bin/$(APP_NAME) ./cmd/server

## build-all: Compile all binaries (server, migrator, edge-agent)
build-all:
	CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o bin/$(APP_NAME) ./cmd/server
	CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o bin/secretpad-migrator ./cmd/migrator
	CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o bin/secretpad-edge-agent ./cmd/edge-agent

## run: Build and run the server locally
run: build
	./bin/$(APP_NAME) -config ./config/privahub.yaml

## test: Run all unit tests
test:
	$(GOTEST) -v -race -coverprofile=coverage.out ./...

## test-cover: Run tests with coverage report
test-cover: test
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

## lint: Run golangci-lint
lint:
	golangci-lint run ./...

## fmt: Format all Go source files
fmt:
	$(GOCMD) fmt ./...
	goimports -w .

## tidy: Download and tidy module dependencies
tidy:
	$(GOMOD) tidy

## wire: Generate dependency injection code
wire:
	cd internal/wire && wire

## clean: Remove build artifacts
clean:
	rm -rf bin/ coverage.out coverage.html

## docker: Build Docker image
docker:
	docker build -t $(APP_NAME):$(VERSION) -f deployments/docker/Dockerfile .

## proto: Generate protobuf code (requires protoc + plugins)
proto:
	protoc --go_out=. --go-grpc_out=. api/proto/v1/*.proto

## help: Show this help message
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | column -t -s ':'
