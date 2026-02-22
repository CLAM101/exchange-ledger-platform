---
name: gen-service
description: Scaffold a new microservice following the exchange-ledger-platform patterns
disable-model-invocation: true
user-invokable: true
---

# Generate Service

Generate a new microservice for the exchange-ledger-platform.

## Usage

`/gen-service <service-name>`

The service name is provided as: $ARGUMENTS

## Instructions

1. Create the service entry point at `cmd/<service-name>/main.go`
2. Follow the bootstrap pattern from existing services (see `cmd/ledger/main.go` and `cmd/account/main.go`)
3. The service must:
   - Create a cancellable context
   - Initialize observability (logger + metrics) using `internal/platform/observability`
   - Start metrics HTTP server on a dedicated port
   - Set up signal handling for graceful shutdown (SIGINT/SIGTERM)
   - Create and start gRPC server with reflection enabled
   - Handle shutdown or errors gracefully

## Port Conventions

Assign the next available ports following the pattern:
- Ledger: gRPC 9001, metrics 9091
- Account: gRPC 9002, metrics 9092
- Wallet: gRPC 9003, metrics 9093
- Gateway: HTTP 8080, metrics 9094

## Reference Files

- Bootstrap pattern: `cmd/ledger/main.go`
- Observability: `internal/platform/observability/`
- Conventions: `docs/conventions.md`

## After Generation

1. Add the service to `docker-compose.yml`
2. Add Prometheus scrape config to `deploy/prometheus.yml`
3. Update `CLAUDE.md` with the new service ports
