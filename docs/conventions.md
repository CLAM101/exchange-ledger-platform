# Development Conventions

This document outlines the coding standards and best practices for the Exchange Ledger Platform.

## General Principles

1. **Clarity over cleverness** - Write code that is easy to understand
2. **Fail fast** - Validate inputs early and return errors promptly
3. **Idempotency** - All write operations must be idempotent using keys
4. **Observability** - Log meaningful events, emit metrics for key operations

## Go Style

### Code Organization

- Follow standard Go project layout
- Keep packages focused and cohesive
- Prefer internal packages for non-exported code
- Domain logic goes in `internal/<service>/`
- Shared platform code goes in `internal/platform/`

### Naming Conventions

- Use MixedCaps (camelCase/PascalCase), never underscores
- Interface names: single method = method name + "er" suffix (e.g., `Reader`)
- Receiver names: short (1-2 chars), consistent within a type
- Package names: short, lowercase, no underscores
- Variable names: short for small scopes, descriptive for larger scopes

### Error Handling

#### Error Creation

```go
// Use standard errors for simple cases
if err != nil {
    return fmt.Errorf("failed to process transaction: %w", err)
}

// Use custom error types for domain errors
type InsufficientFundsError struct {
    AccountID string
    Required  int64
    Available int64
}

func (e *InsufficientFundsError) Error() string {
    return fmt.Sprintf("insufficient funds: account=%s required=%d available=%d",
        e.AccountID, e.Required, e.Available)
}
```

#### Error Wrapping

- Always wrap errors with context using `%w` verb
- Add meaningful context at each layer
- Don't repeat information from lower layers

```go
// Good
if err := repo.Save(ctx, tx); err != nil {
    return fmt.Errorf("save transaction: %w", err)
}

// Bad - too generic
if err := repo.Save(ctx, tx); err != nil {
    return fmt.Errorf("error: %w", err)
}
```

#### Error Checking

- Check all errors - no exceptions
- Use `errors.Is()` and `errors.As()` for error inspection
- Return early on errors (no else blocks)

```go
// Good
tx, err := repo.GetTransaction(ctx, id)
if err != nil {
    return nil, err
}
return tx, nil

// Bad
tx, err := repo.GetTransaction(ctx, id)
if err != nil {
    return nil, err
} else {
    return tx, nil
}
```

## Context Usage

### Context Passing

- Always pass `context.Context` as the first parameter
- Don't store contexts in structs
- Use `context.Background()` only in main, tests, and top-level initialization

```go
// Good
func (s *Service) ProcessTransaction(ctx context.Context, tx *Transaction) error {
    // ...
}

// Bad - context not first
func (s *Service) ProcessTransaction(tx *Transaction, ctx context.Context) error {
    // ...
}
```

### Context Values

- Use context for request-scoped values only (request IDs, trace IDs, auth tokens)
- Define typed keys to avoid collisions

```go
type contextKey string

const (
    requestIDKey contextKey = "request_id"
    userIDKey    contextKey = "user_id"
)

func WithRequestID(ctx context.Context, id string) context.Context {
    return context.WithValue(ctx, requestIDKey, id)
}

func GetRequestID(ctx context.Context) string {
    if id, ok := ctx.Value(requestIDKey).(string); ok {
        return id
    }
    return ""
}
```

### Context Cancellation

- Respect context cancellation in long-running operations
- Check `ctx.Err()` periodically in loops

```go
func (s *Service) ProcessBatch(ctx context.Context, items []Item) error {
    for _, item := range items {
        // Check for cancellation
        if ctx.Err() != nil {
            return ctx.Err()
        }
        
        if err := s.processItem(ctx, item); err != nil {
            return err
        }
    }
    return nil
}
```

## Logging

### Structured Logging

Use structured logging with consistent fields:

```go
logger.Info("transaction posted",
    zap.String("tx_id", tx.ID),
    zap.String("idempotency_key", tx.IdempotencyKey),
    zap.Int64("amount", tx.Amount),
    zap.Duration("duration", elapsed),
)
```

### Log Levels

- **Debug**: Detailed diagnostic information (disabled in production)
- **Info**: Normal operational events (start/stop, state changes)
- **Warn**: Unexpected but handled situations (retries, degraded mode)
- **Error**: Error events that need attention

### Standard Fields

Always include when applicable:
- `request_id` - Request correlation ID
- `user_id` - User identifier
- `tx_id` - Transaction ID
- `idempotency_key` - Idempotency key for write operations
- `duration` - Operation duration
- `error` - Error message and stack trace

```go
logger.Error("failed to post transaction",
    zap.String("request_id", reqID),
    zap.String("tx_id", txID),
    zap.String("idempotency_key", key),
    zap.Error(err),
)
```

## Testing

### Test Organization

- Table-driven tests for multiple scenarios
- Subtests for related test cases
- Use `t.Parallel()` when tests are independent

```go
func TestValidateTransaction(t *testing.T) {
    tests := []struct {
        name    string
        tx      *Transaction
        wantErr bool
    }{
        {
            name: "valid transaction",
            tx:   &Transaction{/* ... */},
            wantErr: false,
        },
        {
            name: "unbalanced transaction",
            tx:   &Transaction{/* ... */},
            wantErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateTransaction(tt.tx)
            if (err != nil) != tt.wantErr {
                t.Errorf("ValidateTransaction() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

### Test Naming

- Test functions: `TestFunctionName`
- Benchmark functions: `BenchmarkFunctionName`
- Example functions: `ExampleFunctionName`

### Assertions

- Use testify/assert for better error messages (optional)
- Prefer explicit comparisons over reflection when possible

## Database

### Transactions

- Always use transactions for write operations
- Rollback on error, commit on success
- Use deferred rollback pattern

```go
func (r *Repository) SaveTransaction(ctx context.Context, tx *Transaction) error {
    dbTx, err := r.db.BeginTx(ctx, nil)
    if err != nil {
        return fmt.Errorf("begin transaction: %w", err)
    }
    defer dbTx.Rollback() // Safe to call even after commit
    
    // ... perform operations ...
    
    if err := dbTx.Commit(); err != nil {
        return fmt.Errorf("commit transaction: %w", err)
    }
    return nil
}
```

### Scanning Named Types

When scanning SQL rows into structs that use named types (e.g. `type AccountID string`),
scan directly into the named type field — do not use intermediate variables with manual conversion.
Go's `database/sql` driver resolves the underlying type via reflection, so named types based on
primitives (`string`, `int64`, etc.) work with `Scan` out of the box.

```go
// Good - scan directly into named type fields
var e Entry
rows.Scan(&e.EntryID, &e.TxID, &e.AccountID, &e.Asset, &e.Amount, &e.CreatedAt)

// Bad - unnecessary intermediate variables
var acctID, assetStr string
rows.Scan(&e.EntryID, &e.TxID, &acctID, &assetStr, &e.Amount, &e.CreatedAt)
e.AccountID = AccountID(acctID)
e.Asset = Asset(assetStr)
```

### Connection Management

- Use connection pooling
- Set appropriate timeouts
- Handle context cancellation

## API Design

### gRPC

- Use protobuf for service definitions
- Return appropriate status codes
- Include meaningful error messages
- Enable server reflection for development

### REST

- Use standard HTTP status codes
- Return JSON for all responses
- Include error details in response body
- Use idempotency keys for writes

## Performance

### General

- Profile before optimizing
- Avoid premature optimization
- Use benchmarks to measure improvements

### Concurrency

- Use channels for coordination
- Use sync primitives for protection
- Avoid goroutine leaks (always ensure exit path)
- Be cautious with shared state

## Documentation

### Code Comments

- Exported functions/types must have doc comments
- Doc comments start with the name being documented
- Keep comments up to date with code changes

```go
// PostTransaction records a double-entry transaction in the ledger.
// It validates that postings balance and enforces overdraft rules.
// Returns InsufficientFundsError if account balance would go negative.
func PostTransaction(ctx context.Context, tx *Transaction) error {
    // ...
}
```

### Package Comments

- Every package should have a doc.go file
- Describe package purpose and key concepts

## Security

- Never log sensitive data (passwords, tokens, full PII)
- Never expose sensitive data, API keys, passwords, private keys or anything similar. 
- Validate all inputs
- Use parameterized queries (no SQL injection)
- Set timeouts on all external calls
- Use TLS for all network communication

## Dependencies

- Minimize external dependencies
- Pin dependency versions
- Regularly update dependencies for security patches
- Prefer standard library when sufficient

