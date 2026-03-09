package account_test

import (
	"testing"

	"github.com/CLAM101/exchange-ledger-platform/internal/account"
)

func TestUserValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		user    account.User
		wantErr bool
	}{
		{
			name:    "valid user",
			user:    account.User{Email: "alice@example.com", IdempotencyKey: "key-1"},
			wantErr: false,
		},
		{
			name:    "empty email",
			user:    account.User{Email: "", IdempotencyKey: "key-2"},
			wantErr: true,
		},
		{
			name:    "email missing @",
			user:    account.User{Email: "alice.example.com", IdempotencyKey: "key-3"},
			wantErr: true,
		},
		{
			name:    "empty idempotency key",
			user:    account.User{Email: "bob@example.com", IdempotencyKey: ""},
			wantErr: true,
		},
		{
			name:    "both empty",
			user:    account.User{Email: "", IdempotencyKey: ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.user.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
