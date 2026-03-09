// Package account implements user identity management and the mapping
// from users to ledger account IDs.
package account

import (
	"errors"
	"time"
)

// UserID uniquely identifies a user.
type UserID string

// User represents a registered user in the account service.
type User struct {
	ID             UserID
	Email          string
	IdempotencyKey string
	CreatedAt      time.Time
}

// UserAssetAccount maps a user and asset to a ledger account ID.
type UserAssetAccount struct {
	UserID          UserID
	Asset           string
	LedgerAccountID string
	CreatedAt       time.Time
}

// Sentinel errors for the account domain.
var (
	ErrNotFound    = errors.New("not found")
	ErrEmailExists = errors.New("email already exists")
)
