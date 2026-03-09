package account

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/CLAM101/exchange-ledger-platform/internal/platform/observability"
)

const mysqlDuplicateEntry = 1062

// MySQLRepository implements Repository using a MySQL database.
type MySQLRepository struct {
	db      *sql.DB
	logger  *zap.Logger
	metrics *observability.Metrics
}

// NewMySQLRepository creates a new MySQLRepository.
func NewMySQLRepository(db *sql.DB, logger *zap.Logger, metrics *observability.Metrics) *MySQLRepository {
	return &MySQLRepository{db: db, logger: logger, metrics: metrics}
}

// dbErr increments the DB error counter and wraps the error with context.
func (r *MySQLRepository) dbErr(err error, msg string, args ...any) error {
	r.metrics.DBErrorTotal.Inc()
	return fmt.Errorf("%s: %w", fmt.Sprintf(msg, args...), err)
}

// CreateUser registers a new user. If a user with the same idempotency key
// already exists, the existing user is returned. Returns ErrEmailExists if
// the email is already taken by a different user.
func (r *MySQLRepository) CreateUser(ctx context.Context, user User) (*User, error) {
	if err := user.Validate(); err != nil {
		return nil, fmt.Errorf("validating user: %w", err)
	}

	dbTx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, r.dbErr(err, "beginning transaction")
	}
	defer dbTx.Rollback() //nolint:errcheck // Rollback after commit returns ErrTxDone which is expected

	// Idempotency check: return existing user if idempotency key matches.
	existing, err := r.findByIdempotencyKey(ctx, dbTx, user.IdempotencyKey)
	if err != nil {
		return nil, r.dbErr(err, "checking idempotency")
	}
	if existing != nil {
		r.metrics.IdempotencyReplay.Inc()
		r.logger.Info("idempotency replay",
			zap.String("user_id", string(existing.ID)),
			zap.String("idempotency_key", existing.IdempotencyKey),
		)
		return existing, nil
	}

	// Generate ID and timestamp.
	user.ID = UserID(uuid.NewString())
	user.CreatedAt = time.Now()

	// Insert user row.
	if _, err := dbTx.ExecContext(ctx,
		`INSERT INTO users (user_id, email, idempotency_key, created_at)
		 VALUES (?, ?, ?, ?)`,
		string(user.ID), user.Email, user.IdempotencyKey, user.CreatedAt,
	); err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == mysqlDuplicateEntry {
			return nil, fmt.Errorf("%w: %s", ErrEmailExists, user.Email)
		}
		return nil, r.dbErr(err, "inserting user")
	}

	if err := dbTx.Commit(); err != nil {
		return nil, r.dbErr(err, "committing transaction")
	}

	r.logger.Info("user created",
		zap.String("user_id", string(user.ID)),
		zap.String("email", user.Email),
		zap.String("idempotency_key", user.IdempotencyKey),
	)

	return &user, nil
}

// GetUser retrieves a user by ID.
func (r *MySQLRepository) GetUser(ctx context.Context, id UserID) (*User, error) {
	var u User
	err := r.db.QueryRowContext(ctx,
		`SELECT user_id, email, idempotency_key, created_at FROM users WHERE user_id = ?`,
		string(id),
	).Scan(&u.ID, &u.Email, &u.IdempotencyKey, &u.CreatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, r.dbErr(err, "querying user %s", id)
	}
	return &u, nil
}

// GetUserByEmail retrieves a user by email.
func (r *MySQLRepository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	err := r.db.QueryRowContext(ctx,
		`SELECT user_id, email, idempotency_key, created_at FROM users WHERE email = ?`,
		email,
	).Scan(&u.ID, &u.Email, &u.IdempotencyKey, &u.CreatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, r.dbErr(err, "querying user by email %s", email)
	}
	return &u, nil
}

// LinkAssetAccount creates a mapping from a user+asset to a ledger account ID.
// If the mapping already exists, the existing mapping is returned.
func (r *MySQLRepository) LinkAssetAccount(ctx context.Context, ua UserAssetAccount) (*UserAssetAccount, error) {
	// Check for existing mapping.
	var existing UserAssetAccount
	err := r.db.QueryRowContext(ctx,
		`SELECT user_id, asset, ledger_account_id, created_at
		 FROM user_asset_accounts WHERE user_id = ? AND asset = ?`,
		string(ua.UserID), ua.Asset,
	).Scan(&existing.UserID, &existing.Asset, &existing.LedgerAccountID, &existing.CreatedAt)

	if err == nil {
		return &existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, r.dbErr(err, "checking existing asset account %s/%s", ua.UserID, ua.Asset)
	}

	// No existing mapping — insert.
	ua.CreatedAt = time.Now()

	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO user_asset_accounts (user_id, asset, ledger_account_id, created_at)
		 VALUES (?, ?, ?, ?)`,
		string(ua.UserID), ua.Asset, ua.LedgerAccountID, ua.CreatedAt,
	); err != nil {
		return nil, r.dbErr(err, "linking asset account %s/%s", ua.UserID, ua.Asset)
	}

	return &ua, nil
}

// GetLedgerAccountID returns the ledger account ID for a user and asset pair.
func (r *MySQLRepository) GetLedgerAccountID(ctx context.Context, userID UserID, asset string) (string, error) {
	var ledgerAccountID string
	err := r.db.QueryRowContext(ctx,
		`SELECT ledger_account_id FROM user_asset_accounts WHERE user_id = ? AND asset = ?`,
		string(userID), asset,
	).Scan(&ledgerAccountID)

	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", r.dbErr(err, "querying ledger account for %s/%s", userID, asset)
	}
	return ledgerAccountID, nil
}

// findByIdempotencyKey checks if a user with the given key already exists
// inside an active database transaction. Returns nil, nil if not found.
func (r *MySQLRepository) findByIdempotencyKey(ctx context.Context, dbTx *sql.Tx, key string) (*User, error) {
	var u User
	err := dbTx.QueryRowContext(ctx,
		`SELECT user_id, email, idempotency_key, created_at
		 FROM users WHERE idempotency_key = ?`, key,
	).Scan(&u.ID, &u.Email, &u.IdempotencyKey, &u.CreatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}
