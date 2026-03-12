// Package ledger implements core double-entry accounting, balance tracking,
// and overdraft prevention for the exchange ledger platform.
package ledger

import (
	"strings"
	"time"
)

// AccountID uniquely identifies a ledger account.
// System accounts use known prefixes ("external:", "fees:", "pool:").
// User accounts use the "user:" prefix.
type AccountID string

// OverdraftExempt returns true for accounts that are allowed to go negative.
// Only "external:" accounts qualify — these are bookkeeping counterparties
// (e.g. "external:deposits") that offset real user balances, not actual
// asset holdings. All other accounts (user, fees, pool, unknown) are
// protected by overdraft checks.
func (id AccountID) OverdraftExempt() bool {
	return strings.HasPrefix(string(id), "external:")
}

// Asset identifies the currency being transacted (e.g. "BTC").
type Asset string

// Amount is an integer quantity of the smallest indivisible unit of an asset
// (e.g. satoshis for BTC). Never use float64 for monetary values.
type Amount int64

// Posting is one leg of a double-entry transaction — a signed movement of an
// asset into or out of a single account. Positive = credit, negative = debit.
type Posting struct {
	AccountID AccountID
	Asset     Asset
	Amount    Amount
}

// Transaction groups a balanced set of postings that must be applied atomically.
type Transaction struct {
	ID             string
	IdempotencyKey string
	Postings       []Posting
	CreatedAt      time.Time
}

// Entry is a persisted ledger entry — one posting within a committed transaction,
// with its database-assigned entry_id and timestamp.
type Entry struct {
	EntryID   int64
	TxID      string
	AccountID AccountID
	Asset     Asset
	Amount    Amount
	CreatedAt time.Time
}
