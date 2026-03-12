# CLAUDE.md - Exchange Ledger Platform

## Project Overview

Microservices-based cryptocurrency exchange ledger demonstrating double-entry accounting with ACID guarantees, idempotency patterns, and production-ready observability in Go.

## Architecture

```
┌─────────────────┐
│ Gateway :8080   │  ← HTTP REST (external clients)
│ (metrics :9094) │
└────────┬────────┘
         │ gRPC
         ▼
┌─────────────────┐      ┌─────────────────┐
│ Wallet :9003    │─────→│ Account :9002   │
│ (metrics :9093) │      │ (metrics :9092) │
└────────┬────────┘      └─────────────────┘
         │ gRPC                   │
         ├───────────────┐        │
         ▼               ▼        │
┌─────────────────┐ ┌──────────────────┐
│ Ledger :9001    │ │ Asset :9004      │
│ (metrics :9091) │ │ (metrics :9095)  │
└────────┬────────┘ └──────────────────┘
         │                (in-memory)
         ▼
      [MySQL 8]
```

**Services:**
- **Ledger** - Core double-entry accounting, balance tracking, overdraft prevention
- **Account** - User identity, maps users to ledger account IDs
- **Asset** - In-memory registry of supported assets (symbol, decimals, active)
- **Wallet** - Deposit/withdrawal orchestration, reservation model
- **Gateway** - REST facade, translates HTTP to gRPC

## Tech Stack

- Go 1.24, gRPC, Protocol Buffers (Buf)
- MySQL 8, Redis 7
- zap (logging), Prometheus (metrics), Grafana (dashboards)
- golangci-lint, Docker Compose

## Essential Commands

```bash
make up         # Start full local stack (services + infra)
make down       # Stop and clean up
make build      # Compile all services
make test       # Run unit + integration tests
make lint       # Run golangci-lint + go vet
make proto      # Generate protobuf code
make fmt        # Format code
```

## Code Organization

```
cmd/                    # Service entry points (main.go per service)
internal/platform/      # Shared packages
  ├── observability/    # Logger, metrics, request ID context
  └── grpc/             # gRPC server/client helpers (WIP)
proto/                  # Protobuf definitions
proto/gen/              # Generated Go code (do not edit)
deploy/                 # K8s manifests, Prometheus/Grafana configs
docs/                   # conventions.md is the key reference
```

## Key Conventions

**IMPORTANT: Always read `docs/conventions.md` before writing or reviewing code.** It is the authoritative style guide. Summary below is a quick reference only — defer to conventions.md when in doubt.

### Idempotency
All write operations MUST accept an idempotency key. Check for existing results before processing.

### Error Handling
```go
// Always wrap with context
return fmt.Errorf("creating account %s: %w", id, err)

// Use errors.Is/As for inspection
if errors.Is(err, ErrNotFound) { ... }
```

### Context
- First parameter, always: `func DoThing(ctx context.Context, ...)`
- Never store in structs
- Check `ctx.Err()` in loops

### Logging
Use structured fields consistently:
```go
logger.Info("transaction posted",
    zap.String("request_id", reqID),
    zap.String("tx_id", txID),
    zap.String("idempotency_key", key),
    zap.Duration("duration", elapsed),
)
```

### Testing
- Table-driven tests with subtests
- Use `t.Parallel()` for independent tests
- Integration tests: `// +build integration`
- **TDD workflow:** write tests first, but only run `go test` once the implementation exists. Use `go build` to verify compilation in between.

### Database
```go
tx, err := db.BeginTx(ctx, nil)
if err != nil { return err }
defer tx.Rollback() // Always defer rollback

// ... do work ...

return tx.Commit()
```

## Service Bootstrap Pattern

Each service follows this structure in `cmd/<service>/main.go`:
1. Create cancellable context
2. Init observability (logger + metrics registry)
3. Start metrics HTTP server (goroutine)
4. Set up signal handling (SIGINT/SIGTERM)
5. Create and start gRPC server (with reflection)
6. Block until shutdown signal or error

## Git Conventions

- Branch format: `{issue#}-{ticket-id}` (e.g., `3-t03-grpc-platform-baseline`)
- Tickets follow phases: T0 (infra), T1 (ledger), T2 (account), T3 (wallet), T7 (ops)
- **Do NOT add `Co-Authored-By: Claude` or any AI attribution to commit messages**
- **NEVER commit generated code** — `proto/gen/` is in `.gitignore`. Run `make proto` locally to regenerate.

## Current State

- **Complete:** T0.1 (bootstrap), T0.2 (observability package), T0.3 (gRPC platform baseline), T1.1 (Ledger domain model + invariants), T1.2 (Ledger MySQL schema + migrations)
- **Next:** T1.3 (Ledger repository)

## Gotchas

- Metrics ports are 909X (9091-9094), NOT the gRPC ports
- Never log sensitive data (keys, balances, PII)
- Docker health checks required - services must expose health endpoints
- Proto generation uses Buf, not protoc directly
- `make test` includes integration tests (`-tags=integration`)

## Local Development

```bash
# First time setup
make tools          # Install dev tools
make proto          # Generate proto code
make up             # Start everything

# Access points
# Grafana:    http://localhost:3000
# Prometheus: http://localhost:9090
# Gateway:    http://localhost:8080
```

## Quick Reference

| Service | gRPC Port | Metrics Port |
|---------|-----------|--------------|
| Ledger  | 9001      | 9091         |
| Account | 9002      | 9092         |
| Wallet  | 9003      | 9093         |
| Asset   | 9004      | 9095         |
| Gateway | 8080 (HTTP) | 9094       |
