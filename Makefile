.PHONY: help build run clean docker-build docker-push test fmt lint ebpf-build

# Variables
BINARY_NAME := traffic-collector
MAIN_PATH := ./cmd/collector
BUILD_DIR := bin
IMAGE_NAME := traffic-collector
REGISTRY ?= docker.io/jarpsimoes
VERSION ?= latest
GO := go
CLANG := clang
LLC := llc

# Go build variables
LDFLAGS := -ldflags "-s -w -X main.Version=$(VERSION)"

.DEFAULT_GOAL := help

help: ## Display this help screen
	@grep -h -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.PHONY: deps
deps: ## Download dependencies
	@echo "Downloading dependencies..."
	$(GO) mod download
	$(GO) mod tidy

clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@rm -f pkg/ebpf/*.o

.PHONY: ebpf-build
ebpf-build: ## Build eBPF programs
	@echo "Building eBPF programs..."
	@mkdir -p $(BUILD_DIR)
	$(CLANG) -O2 -g -target bpf -c pkg/ebpf/bpf/program.c -o $(BUILD_DIR)/program.o
	@echo "eBPF program built: $(BUILD_DIR)/program.o"

build: ebpf-build ## Build the binary
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Binary built: $(BUILD_DIR)/$(BINARY_NAME)"

run: build ## Build and run the collector (requires root)
	@echo "Running $(BINARY_NAME)..."
	@sudo $(BUILD_DIR)/$(BINARY_NAME)

test: ## Run tests
	@echo "Running tests..."
	$(GO) test -v -race -coverprofile=coverage.out ./...

fmt: ## Format code
	@echo "Formatting code..."
	$(GO) fmt ./...

lint: ## Run linter
	@echo "Running linter..."
	@which golangci-lint > /dev/null || go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	golangci-lint run ./...

.PHONY: docker-build
docker-build: ## Build Docker image
	@echo "Building Docker image $(REGISTRY)/$(IMAGE_NAME):$(VERSION)..."
	docker build -t $(REGISTRY)/$(IMAGE_NAME):$(VERSION) \
	  --build-arg VERSION=$(VERSION) \
	  --build-arg BUILD_DATE=$$(date -u +'%Y-%m-%dT%H:%M:%SZ') \
	  --build-arg VCS_REF=$$(git rev-parse --short HEAD) \
	  -f Dockerfile .
	@echo "Docker image built: $(REGISTRY)/$(IMAGE_NAME):$(VERSION)"

.PHONY: docker-push
docker-push: docker-build ## Build and push Docker image
	@echo "Pushing Docker image $(REGISTRY)/$(IMAGE_NAME):$(VERSION)..."
	docker push $(REGISTRY)/$(IMAGE_NAME):$(VERSION)
	@echo "Docker image pushed: $(REGISTRY)/$(IMAGE_NAME):$(VERSION)"

.PHONY: helm-lint
helm-lint: ## Lint Helm chart
	@echo "Linting Helm chart..."
	@which helm > /dev/null || { echo "Helm not found"; exit 1; }
	helm lint charts/traffic-collector

.PHONY: helm-template
helm-template: ## Generate Helm chart templates
	@echo "Generating Helm chart templates..."
	helm template traffic-collector charts/traffic-collector

.PHONY: helm-install-dev
helm-install-dev: ## Install Helm chart in development mode
	@echo "Installing Helm chart in development mode..."
	@which helm > /dev/null || { echo "Helm not found"; exit 1; }
	helm install traffic-collector charts/traffic-collector \
	  -f charts/traffic-collector/values-dev.yaml \
	  --namespace monitoring \
	  --create-namespace

.PHONY: version
version: ## Show version
	@echo "Version: $(VERSION)"
