.PHONY: help up down test lint demo clean build

# Default target
help:
	@echo "Exchange Ledger Platform - Available targets:"
	@echo "  make up       - Start all services with docker-compose"
	@echo "  make down     - Stop all services and clean up"
	@echo "  make test     - Run all tests"
	@echo "  make lint     - Run linters"
	@echo "  make demo     - Run demo scenario"
	@echo "  make build    - Build all service binaries"
	@echo "  make clean    - Clean build artifacts"

# Start the development stack
up:
	@echo "Starting services..."
	docker-compose up -d
	@echo "Waiting for services to be healthy..."
	@sleep 5
	@echo "Services started. Gateway available at http://localhost:8080"

# Stop and clean up
down:
	@echo "Stopping services..."
	docker-compose down -v

# Run all tests
test:
	@echo "Running unit tests..."
	go test -v -cover ./...
	@echo "Running integration tests..."
	go test -v -tags=integration ./...

# Run linters
lint:
	@echo "Running golangci-lint..."
	$(shell go env GOPATH)/bin/golangci-lint run --timeout 5m ./...
	@echo "Running go vet..."
	go vet ./...
	@echo "Checking go mod tidy..."
	go mod tidy
	git diff --exit-code go.mod go.sum

# Build all service binaries
build:
	@echo "Building ledger service..."
	go build -o bin/ledger ./cmd/ledger
	@echo "Building account service..."
	go build -o bin/account ./cmd/account
	@echo "Building wallet service..."
	go build -o bin/wallet ./cmd/wallet
	@echo "Building gateway service..."
	go build -o bin/gateway ./cmd/gateway
	@echo "Building asset service..."
	go build -o bin/asset ./cmd/asset
	@echo "Building migrate tool..."
	go build -o bin/migrate ./cmd/migrate

# Run demo scenario
demo:
	@echo "Running demo scenario..."
	@./scripts/demo.sh

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	go clean -cache -testcache

# Install development tools
tools:
	@echo "Installing development tools..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/bufbuild/buf/cmd/buf@v1.28.1

# Generate protobuf code
proto:
	@echo "Generating protobuf code..."
	buf generate

# Run database migrations
migrate-up:
	@echo "Running database migrations..."
	go run ./cmd/migrate up

migrate-down:
	@echo "Rolling back database migrations..."
	go run ./cmd/migrate down

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...
	gofmt -s -w .

