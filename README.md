# Exchange Ledger Platform

A microservices-based cryptocurrency exchange ledger system demonstrating double-entry accounting, idempotency, and distributed systems patterns.

## Architecture

This monorepo contains the following services:

- **Ledger Service** - Core double-entry accounting ledger with ACID guarantees
- **Account Service** - User identity and account management
- **Asset Service** - In-memory asset registry (symbols, decimals, active status)
- **Wallet Service** - Deposit/withdrawal orchestration
- **Gateway Service** - HTTP REST API gateway

## Prerequisites

To build and run this project on a clean machine, you need:

- **Go 1.21+** - [Install Go](https://golang.org/doc/install)
- **Docker & Docker Compose** - [Install Docker](https://docs.docker.com/get-docker/)
- **Make** - Usually pre-installed on Linux/Mac, [install on Windows](https://gnuwin32.sourceforge.net/packages/make.htm)
- **golangci-lint** (optional, for linting) - `go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.55.2` or run `make tools`
- **buf** (optional, for protobuf) - `go install github.com/bufbuild/buf/cmd/buf@v1.28.1` or run `make tools`

### Quick Setup

```bash
# Clone the repository
git clone https://github.com/CLAM101/exchange-ledger-platform.git
cd exchange-ledger-platform

# Install development tools (optional)
make tools

# Verify setup
make test
make lint
```

## Quick Start

```bash
# Start all services
make up

# Run tests
make test

# Run linter
make lint

# Run demo scenario
make demo

# Stop all services
make down
```

## Project Structure

```
.
├── cmd/                    # Service entrypoints
│   ├── ledger/            # Ledger service main
│   ├── account/           # Account service main
│   ├── asset/             # Asset registry service main
│   ├── wallet/            # Wallet service main
│   └── gateway/           # Gateway service main
├── internal/              # Private application code
│   ├── platform/          # Shared platform packages
│   │   ├── observability/ # Logging, metrics, tracing
│   │   └── grpc/          # gRPC server/client helpers
│   ├── ledger/            # Ledger domain logic
│   ├── account/           # Account domain logic
│   ├── asset/             # Asset registry domain logic
│   └── wallet/            # Wallet domain logic
├── proto/                 # Protocol Buffer definitions
├── deploy/                # Deployment configurations
│   └── k8s/              # Kubernetes manifests
├── docs/                  # Documentation
└── docker-compose.yml     # Local development stack
```

## Development

See [docs/conventions.md](docs/conventions.md) for coding standards and best practices.

## Documentation

- [Architecture Overview](docs/architecture.md)
- [Ledger Design](docs/ledger.md)
- [Operations Runbook](docs/runbook.md)
- [Conventions](docs/conventions.md)

## License

See [LICENSE](LICENSE) file.
