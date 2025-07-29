# Network Device Makefile

# Variables
BINARY_NAME=network-device
MAIN_PATH=cmd/device/main.go
BUILD_DIR=build
CONFIG_DIR=config
LOG_DIR=logs

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt
GOVET=$(GOCMD) vet

# Build flags
LDFLAGS=-ldflags "-X main.version=$(shell git describe --tags --always --dirty)"

.PHONY: all build clean test deps fmt vet run help install

# Default target
all: clean deps fmt vet test build

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	@rm -rf $(BUILD_DIR)
	@rm -rf $(LOG_DIR)
	@echo "Clean complete"

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

# Format code
fmt:
	@echo "Formatting code..."
	$(GOFMT) ./...

# Vet code
vet:
	@echo "Vetting code..."
	$(GOVET) ./...

# Install dependencies
install:
	@echo "Installing dependencies..."
	$(GOGET) -u all

# Run the application
run: build
	@echo "Running $(BINARY_NAME)..."
	@mkdir -p $(LOG_DIR)
	./$(BUILD_DIR)/$(BINARY_NAME) run

# Run with custom config
run-config: build
	@echo "Running $(BINARY_NAME) with config..."
	@mkdir -p $(LOG_DIR)
	./$(BUILD_DIR)/$(BINARY_NAME) run --config $(CONFIG_DIR)/device.yaml --verbose

# Run multiple instances for testing
run-multi: build
	@echo "Starting multiple device instances..."
	@mkdir -p $(LOG_DIR)
	@echo "Starting device-001 on port 8080..."
	@./$(BUILD_DIR)/$(BINARY_NAME) run --device-id device-001 --tcp-port 8080 --udp-port 8081 &
	@sleep 2
	@echo "Starting device-002 on port 8090..."
	@./$(BUILD_DIR)/$(BINARY_NAME) run --device-id device-002 --tcp-port 8090 --udp-port 8091 --config $(CONFIG_DIR)/device.yaml &
	@echo "Both devices started. Use 'make stop-multi' to stop them."

# Stop multiple instances
stop-multi:
	@echo "Stopping device instances..."
	@pkill -f "$(BINARY_NAME)" || true
	@echo "Devices stopped"

# Show device status
status: build
	./$(BUILD_DIR)/$(BINARY_NAME) status

# Show device config
config: build
	./$(BUILD_DIR)/$(BINARY_NAME) config

# Build and run TCP client example
client-tcp:
	@echo "Running TCP client example..."
	$(GOCMD) run examples/client/tcp_client.go localhost:8080

# Build and run UDP discovery client example
client-discovery:
	@echo "Running UDP discovery client example..."
	$(GOCMD) run examples/client/udp_discovery_client.go

# Create distribution package
dist: clean build
	@echo "Creating distribution package..."
	@mkdir -p $(BUILD_DIR)/dist/$(BINARY_NAME)
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(BUILD_DIR)/dist/$(BINARY_NAME)/
	@cp -r $(CONFIG_DIR) $(BUILD_DIR)/dist/$(BINARY_NAME)/
	@cp README.md $(BUILD_DIR)/dist/$(BINARY_NAME)/
	@cd $(BUILD_DIR)/dist && tar -czf $(BINARY_NAME).tar.gz $(BINARY_NAME)
	@echo "Distribution package created: $(BUILD_DIR)/dist/$(BINARY_NAME).tar.gz"

# Development setup
dev-setup: deps
	@echo "Setting up development environment..."
	@mkdir -p $(LOG_DIR)
	@echo "Development environment ready"

# Monitor logs
logs:
	@echo "Monitoring logs..."
	@mkdir -p $(LOG_DIR)
	@touch $(LOG_DIR)/device.log
	tail -f $(LOG_DIR)/device.log

# Check network connectivity
check-network:
	@echo "Checking network connectivity..."
	@echo "TCP port 8080:"
	@lsof -i :8080 || echo "Port 8080 is free"
	@echo "UDP port 8081:"
	@lsof -i :8081 || echo "Port 8081 is free"
	@echo "HTTP monitoring port 8082:"
	@lsof -i :8082 || echo "Port 8082 is free"
	@echo "UDP discovery port 9999:"
	@lsof -i :9999 || echo "Port 9999 is free"

# Docker build (if you want to containerize)
docker-build:
	@echo "Building Docker image..."
	docker build -t $(BINARY_NAME):latest .

# Show help
help:
	@echo "Available targets:"
	@echo "  all          - Clean, deps, fmt, vet, test, and build"
	@echo "  build        - Build the binary"
	@echo "  clean        - Clean build artifacts"
	@echo "  test         - Run tests"
	@echo "  deps         - Download dependencies"
	@echo "  fmt          - Format code"
	@echo "  vet          - Vet code"
	@echo "  run          - Build and run the application"
	@echo "  run-config   - Run with custom config file"
	@echo "  run-multi    - Start multiple device instances"
	@echo "  stop-multi   - Stop multiple device instances"
	@echo "  status       - Show device status"
	@echo "  config       - Show device configuration"
	@echo "  client-tcp   - Run TCP client example"
	@echo "  client-discovery - Run UDP discovery client example"
	@echo "  dist         - Create distribution package"
	@echo "  dev-setup    - Setup development environment"
	@echo "  logs         - Monitor log files"
	@echo "  check-network - Check network port availability"
	@echo "  docker-build - Build Docker image"
	@echo "  help         - Show this help message"