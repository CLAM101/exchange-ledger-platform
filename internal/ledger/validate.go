package ledger

import (
	"errors"
	"fmt"
)

// Sentinel errors for transaction validation. Callers can use errors.Is() to
// inspect which invariant was violated.
var (
	ErrNoPostings    = errors.New("transaction must have at least two postings")
	ErrZeroAmount    = errors.New("posting amount must be non-zero")
	ErrAssetMismatch = errors.New("all postings must use the same asset")
	ErrUnbalanced    = errors.New("postings must sum to zero")
	ErrOverdraft     = errors.New("transaction would overdraft account")
	ErrNotFound      = errors.New("not found")
)

// Validate checks the structural invariants of a transaction:
//   - at least two postings
//   - no posting has a zero amount
//   - all postings share the same asset
//   - posting amounts sum to exactly zero
func (t Transaction) Validate() error {
	if len(t.Postings) < 2 {
		return ErrNoPostings
	}

	asset := t.Postings[0].Asset
	var sum Amount

	for _, p := range t.Postings {
		if p.Amount == 0 {
			return ErrZeroAmount
		}
		if p.Asset != asset {
			return fmt.Errorf("%w: got %q, expected %q", ErrAssetMismatch, p.Asset, asset)
		}
		sum += p.Amount
	}

	if sum != 0 {
		return fmt.Errorf("%w: sum is %d", ErrUnbalanced, sum)
	}

	return nil
}

// CheckOverdraft verifies that applying this transaction would not cause any
// user account balance to drop below zero. Only accounts present in the
// balances map are checked — system/pool accounts are intentionally omitted
// by the caller.
func (t Transaction) CheckOverdraft(balances map[AccountID]Amount) error {
	for _, p := range t.Postings {
		current, isUserAccount := balances[p.AccountID]
		if !isUserAccount {
			continue
		}
		if current+p.Amount < 0 {
			return fmt.Errorf("%w: account %q balance %d, posting %d", ErrOverdraft, p.AccountID, current, p.Amount)
		}
	}
	return nil
}
