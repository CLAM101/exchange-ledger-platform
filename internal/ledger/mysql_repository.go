package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// StatusPosted is the status value for a successfully committed transaction.
const StatusPosted = "posted"

// MySQLRepository implements Repository using a MySQL database.
type MySQLRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewMySQLRepository creates a new MySQLRepository.
func NewMySQLRepository(db *sql.DB, logger *zap.Logger) *MySQLRepository {
	return &MySQLRepository{db: db, logger: logger}
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
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer dbTx.Rollback()

	// Idempotency check: return existing result if already posted.
	existing, err := r.findByIdempotencyKey(ctx, dbTx, tx.IdempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("checking idempotency: %w", err)
	}
	if existing != nil {
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

		if err == sql.ErrNoRows {
			// Row doesn't exist yet — insert a zero-balance row.
			if _, err := dbTx.ExecContext(ctx,
				`INSERT INTO ledger_balances (account_id, asset, balance) VALUES (?, ?, 0)`,
				pair.accountID, pair.asset,
			); err != nil {
				return nil, fmt.Errorf("inserting balance row %s/%s: %w", pair.accountID, pair.asset, err)
			}
			balance = 0
		} else if err != nil {
			return nil, fmt.Errorf("locking balance %s/%s: %w", pair.accountID, pair.asset, err)
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
		return nil, fmt.Errorf("inserting transaction: %w", err)
	}

	// Insert entry rows (one per posting).
	for _, p := range tx.Postings {
		if _, err := dbTx.ExecContext(ctx,
			`INSERT INTO ledger_entries (tx_id, account_id, amount, asset, created_at)
			 VALUES (?, ?, ?, ?, ?)`,
			tx.ID, string(p.AccountID), int64(p.Amount), string(p.Asset), tx.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("inserting entry for %s: %w", p.AccountID, err)
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
			return nil, fmt.Errorf("updating balance for %s/%s: %w", pair.accountID, pair.asset, err)
		}
	}

	if err := dbTx.Commit(); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

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

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying transaction: %w", err)
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

	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("querying balance for %s/%s: %w", accountID, asset, err)
	}
	return Amount(balance), nil
}

// findByIdempotencyKey checks if a transaction with the given key already
// exists inside an active database transaction. Returns nil, nil if not found.
func (r *MySQLRepository) findByIdempotencyKey(ctx context.Context, dbTx *sql.Tx, key string) (*Transaction, error) {
	var tx Transaction
	err := dbTx.QueryRowContext(ctx,
		`SELECT tx_id, idempotency_key, created_at
		 FROM ledger_transactions WHERE idempotency_key = ?`, key,
	).Scan(&tx.ID, &tx.IdempotencyKey, &tx.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying transaction by idempotency key: %w", err)
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
		return nil, fmt.Errorf("querying entries for tx %s: %w", txID, err)
	}
	defer rows.Close()

	var postings []Posting
	for rows.Next() {
		var (
			accountID string
			amount    int64
			asset     string
		)
		if err := rows.Scan(&accountID, &amount, &asset); err != nil {
			return nil, fmt.Errorf("scanning entry: %w", err)
		}
		postings = append(postings, Posting{
			AccountID: AccountID(accountID),
			Amount:    Amount(amount),
			Asset:     Asset(asset),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating entries: %w", err)
	}

	return postings, nil
}
