package ledger

import "time"

// AccountID uniquely identifies a ledger account.
type AccountID string

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
