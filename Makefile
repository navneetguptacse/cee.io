# CEE (Code Execution Engine) Makefile
# High-performance, Judge0-compatible code execution engine in Go

BINARY_NAME ?= cee
BIN_DIR     ?= ./bin
BUILD_PATH  ?= $(BIN_DIR)/$(BINARY_NAME)
MAIN_SRC    ?= ./cmd/cee
GO          ?= go
RAW_VERSION ?= $(shell cat VERSION 2>/dev/null || echo "1.0.0")
VERSION     ?= $(shell echo $(RAW_VERSION) | sed 's/^v*//' | sed 's/^/v/')
LDFLAGS     ?= -s -w -X main.Version=$(VERSION) -X cee.io/pkg/api.Version=$(VERSION)
PORT        ?= 3000
INSTALL_DIR ?= /usr/local/bin
EC2_HOST    ?= 100.52.188.50
EC2_USER    ?= ubuntu
EC2_DIR     ?= ~/cee.io

.PHONY: all help build dist install deploy clean test test-race fmt vet diag server worker docker-build docker-up docker-down docker-prod-up docker-prod-down

all: build

help:
	@echo "CEE (Code Execution Engine) - Available Targets"
	@echo ""
	@echo "Build & Installation:"
	@echo "  make build             Compile the stripped static binary to $(BUILD_PATH)"
	@echo "  make dist              Compile cross-platform binaries for distribution to $(BIN_DIR)/dist"
	@echo "  make install           Install $(BINARY_NAME) binary to $(INSTALL_DIR)"
	@echo "  make clean             Remove compiled binaries and temporary build files"
	@echo ""
	@echo "Testing & Quality:"
	@echo "  make test              Run all unit and integration test suites"
	@echo "  make test-race         Run tests with Go race detector enabled"
	@echo "  make fmt               Format all Go source files with gofmt"
	@echo "  make vet               Run go vet static analysis across packages"
	@echo "  make diag              Run CEE built-in self-diagnostic suite"
	@echo ""
	@echo "Running Services:"
	@echo "  make server            Build and run local CEE API server on port $(PORT)"
	@echo "  make worker            Build and run standalone background worker"
	@echo ""
	@echo "Docker & Compose:"
	@echo "  make docker-build      Build language runner images (Python, Node, GCC, etc.)"
	@echo "  make docker-up         Start local services with Docker Compose"
	@echo "  make docker-down       Stop local Docker Compose services"
	@echo "  make docker-prod-up    Start production cluster (CEE, Redis, Caddy, Prometheus)"
	@echo "  make docker-prod-down  Stop production Docker Compose cluster"
	@echo ""

build:
	@mkdir -p $(BIN_DIR)
	$(GO) build -ldflags="$(LDFLAGS)" -o $(BUILD_PATH) $(MAIN_SRC)
	@echo "Build completed: $(BUILD_PATH)"

dist:
	@mkdir -p $(BIN_DIR)/dist
	GOOS=darwin GOARCH=arm64 $(GO) build -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/dist/cee-darwin-arm64 $(MAIN_SRC)
	GOOS=darwin GOARCH=amd64 $(GO) build -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/dist/cee-darwin-amd64 $(MAIN_SRC)
	GOOS=linux GOARCH=amd64 $(GO) build -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/dist/cee-linux-amd64 $(MAIN_SRC)
	GOOS=linux GOARCH=arm64 $(GO) build -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/dist/cee-linux-arm64 $(MAIN_SRC)
	GOOS=windows GOARCH=amd64 $(GO) build -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/dist/cee-windows-amd64.exe $(MAIN_SRC)
	@echo "Cross-platform binaries successfully created in $(BIN_DIR)/dist:"
	@ls -lh $(BIN_DIR)/dist

install: build
	@echo "Installing $(BINARY_NAME) to $(INSTALL_DIR)..."
	@install -m 755 $(BUILD_PATH) $(INSTALL_DIR)/$(BINARY_NAME) 2>/dev/null || \
		(echo "Permission denied. Please run with sudo: sudo make install" && exit 1)
	@echo "Installed successfully: $(INSTALL_DIR)/$(BINARY_NAME)"

clean:
	@rm -rf $(BIN_DIR)
	@echo "Cleaned build artifacts."

test:
	$(GO) test ./tests/...

test-race:
	$(GO) test -v -race ./tests/...

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

diag: build
	$(BUILD_PATH) test

server: build
	$(BUILD_PATH) server --port $(PORT)

worker: build
	$(BUILD_PATH) worker

docker-build:
	@chmod +x ./scripts/build-images.sh
	./scripts/build-images.sh

docker-up:
	docker compose up -d

docker-down:
	docker compose down

docker-prod-up:
	docker compose -f docker-compose.prod.yml up -d --build

docker-prod-down:
	docker compose -f docker-compose.prod.yml down

deploy: dist
	@echo "Deploying distribution binaries and CEE to $(EC2_USER)@$(EC2_HOST):$(EC2_DIR)..."
	ssh $(EC2_USER)@$(EC2_HOST) "mkdir -p $(EC2_DIR)/bin/dist"
	scp ./bin/dist/* $(EC2_USER)@$(EC2_HOST):$(EC2_DIR)/bin/dist/
	scp ./install.sh $(EC2_USER)@$(EC2_HOST):$(EC2_DIR)/
	ssh $(EC2_USER)@$(EC2_HOST) "cd $(EC2_DIR) && docker compose -f docker-compose.prod.yml up -d --build"
	@echo "Deployment complete! Checking health and install.sh..."
	@curl -fsSL http://$(EC2_HOST)/health || echo "Note: Check server health"
	@curl -fsSL http://$(EC2_HOST)/install.sh > /dev/null && echo "[OK] /install.sh verified on remote server"

