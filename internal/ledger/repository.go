package ledger

import "context"

// Repository defines the persistence operations for the ledger.
type Repository interface {
	// PostTransaction atomically records a double-entry transaction.
	// If a transaction with the same idempotency key already exists,
	// the existing transaction is returned without modification.
	// Returns ErrOverdraft if applying the postings would cause any
	// account balance to go negative.
	PostTransaction(ctx context.Context, tx Transaction) (*Transaction, error)

	// GetTransaction retrieves a transaction by its idempotency key.
	// Returns ErrNotFound if no transaction exists with the given key.
	GetTransaction(ctx context.Context, idempotencyKey string) (*Transaction, error)

	// GetBalance returns the current balance for an account and asset pair.
	// Returns 0 if no balance row exists (account has never transacted).
	GetBalance(ctx context.Context, accountID AccountID, asset Asset) (Amount, error)

	// ListEntries returns ledger entries for an account and asset pair,
	// ordered by entry_id ascending. cursor is the last entry_id seen
	// (0 for the first page). Returns at most limit entries.
	ListEntries(ctx context.Context, accountID AccountID, asset Asset, cursor int64, limit int) ([]Entry, error)
}
