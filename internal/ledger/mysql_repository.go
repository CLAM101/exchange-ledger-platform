package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/CLAM101/exchange-ledger-platform/internal/platform/observability"
)

// StatusPosted is the status value for a successfully committed transaction.
const StatusPosted = "posted"

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

// PostTransaction atomically records a double-entry transaction. If a
// transaction with the same idempotency key already exists, the existing
// transaction is returned without modification.
func (r *MySQLRepository) PostTransaction(ctx context.Context, tx Transaction) (*Transaction, error) {
	if err := tx.Validate(); err != nil {
		return nil, fmt.Errorf("validating transaction: %w", err)
	}

	dbTx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, r.dbErr(err, "beginning transaction")
	}
	defer dbTx.Rollback() //nolint:errcheck // Rollback after commit returns ErrTxDone which is expected

	// Idempotency check: return existing result if already posted.
	existing, err := r.findByIdempotencyKey(ctx, dbTx, tx.IdempotencyKey)
	if err != nil {
		return nil, r.dbErr(err, "checking idempotency")
	}
	if existing != nil {
		r.metrics.IdempotencyReplay.Inc()
		r.logger.Info("idempotency replay",
			zap.String("tx_id", existing.ID),
			zap.String("idempotency_key", existing.IdempotencyKey),
		)
		return existing, nil
	}

	// Collect and sort unique (account_id, asset) pairs for deterministic lock ordering.
	type accountAsset struct {
		accountID string
		asset     string
	}
	seen := make(map[accountAsset]bool)
	var pairs []accountAsset
	for _, p := range tx.Postings {
		key := accountAsset{string(p.AccountID), string(p.Asset)}
		if !seen[key] {
			seen[key] = true
			pairs = append(pairs, key)
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].accountID != pairs[j].accountID {
			return pairs[i].accountID < pairs[j].accountID
		}
		return pairs[i].asset < pairs[j].asset
	})

	// Lock and read balances. For each sorted pair, try to lock the existing
	// row first. If it doesn't exist, insert a zero-balance row (which also
	// acquires the lock). This avoids INSERT IGNORE gap locks that cause
	// deadlocks under concurrency.
	balances := make(map[AccountID]Amount)
	for _, pair := range pairs {
		var balance int64
		err := dbTx.QueryRowContext(ctx,
			`SELECT balance FROM ledger_balances WHERE account_id = ? AND asset = ? FOR UPDATE`,
			pair.accountID, pair.asset,
		).Scan(&balance)

		if errors.Is(err, sql.ErrNoRows) {
			// Row doesn't exist yet — insert a zero-balance row.
			if _, execErr := dbTx.ExecContext(ctx,
				`INSERT INTO ledger_balances (account_id, asset, balance) VALUES (?, ?, 0)`,
				pair.accountID, pair.asset,
			); execErr != nil {
				return nil, r.dbErr(execErr, "inserting balance row %s/%s", pair.accountID, pair.asset)
			}
			balance = 0
		} else if err != nil {
			return nil, r.dbErr(err, "locking balance %s/%s", pair.accountID, pair.asset)
		}

		balances[AccountID(pair.accountID)] = Amount(balance)
	}

	// Check overdraft using the domain logic. The balances map is keyed by
	// AccountID alone, which is safe because Validate() enforces that all
	// postings share the same asset (ErrAssetMismatch).
	if err := tx.CheckOverdraft(balances); err != nil {
		return nil, err
	}

	// Generate ID and timestamp.
	tx.ID = uuid.NewString()
	tx.CreatedAt = time.Now()

	// Insert transaction row.
	if _, err := dbTx.ExecContext(ctx,
		`INSERT INTO ledger_transactions (tx_id, idempotency_key, reference, status, created_at)
		 VALUES (?, ?, '', ?, ?)`,
		tx.ID, tx.IdempotencyKey, StatusPosted, tx.CreatedAt,
	); err != nil {
		return nil, r.dbErr(err, "inserting transaction")
	}

	// Insert entry rows (one per posting).
	for _, p := range tx.Postings {
		if _, err := dbTx.ExecContext(ctx,
			`INSERT INTO ledger_entries (tx_id, account_id, amount, asset, created_at)
			 VALUES (?, ?, ?, ?, ?)`,
			tx.ID, string(p.AccountID), int64(p.Amount), string(p.Asset), tx.CreatedAt,
		); err != nil {
			return nil, r.dbErr(err, "inserting entry for %s", p.AccountID)
		}
	}

	// Aggregate balance deltas per unique (account_id, asset) pair, then
	// issue one UPDATE per pair in the same sorted order used for locking.
	deltas := make(map[accountAsset]int64)
	for _, p := range tx.Postings {
		deltas[accountAsset{string(p.AccountID), string(p.Asset)}] += int64(p.Amount)
	}
	for _, pair := range pairs {
		if _, err := dbTx.ExecContext(ctx,
			`UPDATE ledger_balances SET balance = balance + ? WHERE account_id = ? AND asset = ?`,
			deltas[pair], pair.accountID, pair.asset,
		); err != nil {
			return nil, r.dbErr(err, "updating balance for %s/%s", pair.accountID, pair.asset)
		}
	}

	if err := dbTx.Commit(); err != nil {
		return nil, r.dbErr(err, "committing transaction")
	}

	r.metrics.TxPostedTotal.Inc()

	r.logger.Info("transaction posted",
		zap.String("tx_id", tx.ID),
		zap.String("idempotency_key", tx.IdempotencyKey),
		zap.Int("postings", len(tx.Postings)),
	)

	return &tx, nil
}

// GetTransaction retrieves a transaction by its idempotency key.
func (r *MySQLRepository) GetTransaction(ctx context.Context, idempotencyKey string) (*Transaction, error) {
	var tx Transaction
	err := r.db.QueryRowContext(ctx,
		`SELECT tx_id, idempotency_key, created_at
		 FROM ledger_transactions WHERE idempotency_key = ?`, idempotencyKey,
	).Scan(&tx.ID, &tx.IdempotencyKey, &tx.CreatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, r.dbErr(err, "querying transaction")
	}

	postings, err := r.loadPostings(ctx, r.db, tx.ID)
	if err != nil {
		return nil, err
	}
	tx.Postings = postings

	return &tx, nil
}

// GetBalance returns the current balance for an account and asset pair.
func (r *MySQLRepository) GetBalance(ctx context.Context, accountID AccountID, asset Asset) (Amount, error) {
	var balance int64
	err := r.db.QueryRowContext(ctx,
		`SELECT balance FROM ledger_balances WHERE account_id = ? AND asset = ?`,
		string(accountID), string(asset),
	).Scan(&balance)

	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, r.dbErr(err, "querying balance for %s/%s", accountID, asset)
	}
	return Amount(balance), nil
}

// ListEntries returns ledger entries for an account and asset pair, ordered
// by entry_id ascending. cursor is the last entry_id seen (0 for the first
// page). Returns at most limit entries.
func (r *MySQLRepository) ListEntries(ctx context.Context, accountID AccountID, asset Asset, cursor int64, limit int) ([]Entry, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT entry_id, tx_id, account_id, asset, amount, created_at
		 FROM ledger_entries
		 WHERE account_id = ? AND asset = ? AND entry_id > ?
		 ORDER BY entry_id ASC
		 LIMIT ?`,
		string(accountID), string(asset), cursor, limit,
	)
	if err != nil {
		return nil, r.dbErr(err, "querying entries for %s/%s", accountID, asset)
	}
	defer rows.Close() //nolint:errcheck // rows.Err() is checked after iteration

	var entries []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.EntryID, &e.TxID, &e.AccountID, &e.Asset, &e.Amount, &e.CreatedAt); err != nil {
			return nil, r.dbErr(err, "scanning entry")
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, r.dbErr(err, "iterating entries")
	}

	return entries, nil
}

// findByIdempotencyKey checks if a transaction with the given key already
// exists inside an active database transaction. Returns nil, nil if not found.
func (r *MySQLRepository) findByIdempotencyKey(ctx context.Context, dbTx *sql.Tx, key string) (*Transaction, error) {
	var tx Transaction
	err := dbTx.QueryRowContext(ctx,
		`SELECT tx_id, idempotency_key, created_at
		 FROM ledger_transactions WHERE idempotency_key = ?`, key,
	).Scan(&tx.ID, &tx.IdempotencyKey, &tx.CreatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, r.dbErr(err, "querying transaction by idempotency key")
	}

	postings, err := r.loadPostings(ctx, dbTx, tx.ID)
	if err != nil {
		return nil, err
	}
	tx.Postings = postings

	return &tx, nil
}

// queryable abstracts *sql.DB and *sql.Tx for shared query logic.
type queryable interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// loadPostings loads the postings for a transaction from ledger_entries.
func (r *MySQLRepository) loadPostings(ctx context.Context, q queryable, txID string) ([]Posting, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT account_id, amount, asset
		 FROM ledger_entries WHERE tx_id = ? ORDER BY entry_id`, txID,
	)
	if err != nil {
		return nil, r.dbErr(err, "querying entries for tx %s", txID)
	}
	defer rows.Close() //nolint:errcheck // rows.Err() is checked after iteration

	var postings []Posting
	for rows.Next() {
		var p Posting
		if err := rows.Scan(&p.AccountID, &p.Amount, &p.Asset); err != nil {
			return nil, r.dbErr(err, "scanning entry")
		}
		postings = append(postings, p)
	}
	if err := rows.Err(); err != nil {
		return nil, r.dbErr(err, "iterating entries")
	}

	return postings, nil
}
