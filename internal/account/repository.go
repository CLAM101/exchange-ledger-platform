package account

import "context"

// Repository defines the persistence operations for the account service.
type Repository interface {
	// CreateUser registers a new user. If a user with the same idempotency key
	// already exists, the existing user is returned. Returns ErrEmailExists if
	// the email is already taken by a different user.
	CreateUser(ctx context.Context, user User) (*User, error)

	// GetUser retrieves a user by ID. Returns ErrNotFound if the user does not exist.
	GetUser(ctx context.Context, id UserID) (*User, error)

	// GetUserByEmail retrieves a user by email. Returns ErrNotFound if no user
	// has the given email.
	GetUserByEmail(ctx context.Context, email string) (*User, error)

	// LinkAssetAccount creates a mapping from a user+asset to a ledger account ID.
	// If the mapping already exists, the existing mapping is returned.
	LinkAssetAccount(ctx context.Context, ua UserAssetAccount) (*UserAssetAccount, error)

	// GetLedgerAccountID returns the ledger account ID for a user and asset pair.
	// Returns ErrNotFound if no mapping exists.
	GetLedgerAccountID(ctx context.Context, userID UserID, asset string) (string, error)
}
