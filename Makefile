.PHONY: all build build-web build-tui build-all test lint fmt clean help

# Configuration
BINARY_NAME=voce
TUI_NAME=voce-tui
BIN_DIR=bin
CMD_PATH=./cmd/voce
WEB_DIR=clients/web
TUI_DIR=clients/voce-tui

all: build-all

# 1. Backend Build (depends on web-build for embedding)
build: build-web
	@echo "Building backend $(BINARY_NAME)..."
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(BINARY_NAME) $(CMD_PATH)

# 2. Frontend Build
build-web:
	@echo "Building frontend..."
	@cd $(WEB_DIR) && pnpm install && pnpm build

# 3. TUI Client Build
build-tui:
	@echo "Building TUI client..."
	@mkdir -p $(BIN_DIR)
	@cd $(TUI_DIR) && cargo build --release
	@cp $(TUI_DIR)/target/release/$(TUI_NAME) $(BIN_DIR)/

# 4. Build Everything
build-all: build build-tui

# Testing
test: test-backend test-tui

test-backend:
	@echo "Running backend tests..."
	go test -v ./...

test-tui:
	@echo "Running TUI tests..."
	@cd $(TUI_DIR) && cargo test

# Linting
lint: lint-backend lint-web lint-tui

lint-backend:
	@echo "Running golangci-lint..."
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint is not installed."; \
		exit 1; \
	fi

lint-web:
	@echo "Running frontend lint..."
	@cd $(WEB_DIR) && pnpm lint

lint-tui:
	@echo "Running TUI lint..."
	@cd $(TUI_DIR) && cargo clippy -- -D warnings

# Utility
fmt:
	@echo "Formatting code..."
	go fmt ./...
	@cd $(TUI_DIR) && cargo fmt

tidy:
	@echo "Cleaning up go.mod..."
	go mod tidy

clean:
	@echo "Cleaning up build artifacts..."
	rm -rf $(BIN_DIR)
	rm -rf $(WEB_DIR)/dist
	@cd $(TUI_DIR) && cargo clean

help:
	@echo "Voce Multi-Project Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make build      - Build backend (includes frontend embedding)"
	@echo "  make build-web  - Build frontend dist only"
	@echo "  make build-tui  - Build TUI client"
	@echo "  make build-all  - Build all components (Backend, Web, TUI)"
	@echo "  make test       - Run all project tests"
	@echo "  make lint       - Run all linters"
	@echo "  make clean      - Remove all build artifacts"
