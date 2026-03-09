package account

import (
	"errors"
	"strings"
)

// Validate checks the structural invariants of a User:
//   - email must be non-empty and contain "@"
//   - idempotency key must be non-empty
func (u User) Validate() error {
	if u.Email == "" || !strings.Contains(u.Email, "@") {
		return errors.New("email must be non-empty and contain @")
	}
	if u.IdempotencyKey == "" {
		return errors.New("idempotency key must be non-empty")
	}
	return nil
}
