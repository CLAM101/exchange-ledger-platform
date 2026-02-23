package ledger_test

import (
	"testing"

	"github.com/CLAM101/exchange-ledger-platform/internal/ledger"
)

func TestTransactionValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		tx      ledger.Transaction
		wantErr bool
	}{
		{
			name: "valid balanced transaction",
			tx: ledger.Transaction{
				ID:             "tx_1",
				IdempotencyKey: "idem_1",
				Postings: []ledger.Posting{
					{AccountID: "acc_user", Asset: "BTC", Amount: -1000},
					{AccountID: "acc_exchange", Asset: "BTC", Amount: 1000},
				},
			},
			wantErr: false,
		},
		{
			name: "unbalanced postings",
			tx: ledger.Transaction{
				ID:             "tx_2",
				IdempotencyKey: "idem_2",
				Postings: []ledger.Posting{
					{AccountID: "acc_user", Asset: "BTC", Amount: -1000},
					{AccountID: "acc_exchange", Asset: "BTC", Amount: 999},
				},
			},
			wantErr: true,
		},
		{
			name: "zero amount posting",
			tx: ledger.Transaction{
				ID:             "tx_3",
				IdempotencyKey: "idem_3",
				Postings: []ledger.Posting{
					{AccountID: "acc_user", Asset: "BTC", Amount: 0},
					{AccountID: "acc_exchange", Asset: "BTC", Amount: 0},
				},
			},
			wantErr: true,
		},
		{
			name: "mixed assets",
			tx: ledger.Transaction{
				ID:             "tx_4",
				IdempotencyKey: "idem_4",
				Postings: []ledger.Posting{
					{AccountID: "acc_user", Asset: "BTC", Amount: -1000},
					{AccountID: "acc_exchange", Asset: "ETH", Amount: 1000},
				},
			},
			wantErr: true,
		},
		{
			name: "no postings",
			tx: ledger.Transaction{
				ID:             "tx_5",
				IdempotencyKey: "idem_5",
				Postings:       []ledger.Posting{},
			},
			wantErr: true,
		},
		{
			name: "single posting cannot balance",
			tx: ledger.Transaction{
				ID:             "tx_6",
				IdempotencyKey: "idem_6",
				Postings: []ledger.Posting{
					{AccountID: "acc_user", Asset: "BTC", Amount: 1000},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.tx.Validate()

			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}

func TestTransactionCheckOverdraft(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		tx       ledger.Transaction
		balances map[ledger.AccountID]ledger.Amount
		wantErr  bool
	}{
		{
			name: "sufficient balance",
			tx: ledger.Transaction{
				ID:             "tx_1",
				IdempotencyKey: "idem_1",
				Postings: []ledger.Posting{
					{AccountID: "acc_user", Asset: "BTC", Amount: -1000},
					{AccountID: "acc_exchange", Asset: "BTC", Amount: 1000},
				},
			},
			balances: map[ledger.AccountID]ledger.Amount{
				"acc_user": 2000,
			},
			wantErr: false,
		},
		{
			name: "exact balance is not an overdraft",
			tx: ledger.Transaction{
				ID:             "tx_2",
				IdempotencyKey: "idem_2",
				Postings: []ledger.Posting{
					{AccountID: "acc_user", Asset: "BTC", Amount: -1000},
					{AccountID: "acc_exchange", Asset: "BTC", Amount: 1000},
				},
			},
			balances: map[ledger.AccountID]ledger.Amount{
				"acc_user": 1000,
			},
			wantErr: false,
		},
		{
			name: "insufficient balance causes overdraft",
			tx: ledger.Transaction{
				ID:             "tx_3",
				IdempotencyKey: "idem_3",
				Postings: []ledger.Posting{
					{AccountID: "acc_user", Asset: "BTC", Amount: -1001},
					{AccountID: "acc_exchange", Asset: "BTC", Amount: 1001},
				},
			},
			balances: map[ledger.AccountID]ledger.Amount{
				"acc_user": 1000,
			},
			wantErr: true,
		},
		{
			name: "system account not in balances map is not checked",
			tx: ledger.Transaction{
				ID:             "tx_4",
				IdempotencyKey: "idem_4",
				Postings: []ledger.Posting{
					{AccountID: "acc_user", Asset: "BTC", Amount: -500},
					{AccountID: "acc_system_pool", Asset: "BTC", Amount: 500},
				},
			},
			// Only user account is passed in; system pool is intentionally omitted.
			balances: map[ledger.AccountID]ledger.Amount{
				"acc_user": 1000,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.tx.CheckOverdraft(tt.balances)

			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}
