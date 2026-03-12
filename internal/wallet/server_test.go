package wallet_test

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/CLAM101/exchange-ledger-platform/internal/wallet"
	accountv1 "github.com/CLAM101/exchange-ledger-platform/proto/gen/account/v1"
	ledgerv1 "github.com/CLAM101/exchange-ledger-platform/proto/gen/ledger/v1"
	walletv1 "github.com/CLAM101/exchange-ledger-platform/proto/gen/wallet/v1"
)

// --- Mock clients ---

type mockAccountClient struct {
	accountv1.AccountServiceClient
	getLedgerAccountFn func(ctx context.Context, in *accountv1.GetLedgerAccountRequest, opts ...grpc.CallOption) (*accountv1.GetLedgerAccountResponse, error)
}

func (m *mockAccountClient) GetLedgerAccount(ctx context.Context, in *accountv1.GetLedgerAccountRequest, opts ...grpc.CallOption) (*accountv1.GetLedgerAccountResponse, error) {
	return m.getLedgerAccountFn(ctx, in, opts...)
}

type mockLedgerClient struct {
	ledgerv1.LedgerServiceClient
	postTransactionFn func(ctx context.Context, in *ledgerv1.PostTransactionRequest, opts ...grpc.CallOption) (*ledgerv1.PostTransactionResponse, error)
}

func (m *mockLedgerClient) PostTransaction(ctx context.Context, in *ledgerv1.PostTransactionRequest, opts ...grpc.CallOption) (*ledgerv1.PostTransactionResponse, error) {
	return m.postTransactionFn(ctx, in, opts...)
}

// --- Helper ---

func newTestServer(ac accountv1.AccountServiceClient, lc ledgerv1.LedgerServiceClient) *wallet.Server {
	return wallet.NewServer(ac, lc, zap.NewNop())
}

// --- Validation tests ---

func TestDeposit_MissingIdempotencyKey(t *testing.T) {
	t.Parallel()
	srv := newTestServer(&mockAccountClient{}, &mockLedgerClient{})

	_, err := srv.Deposit(context.Background(), &walletv1.DepositRequest{
		UserId: "user-1",
		Amount: 1000,
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", st.Code())
	}
}

func TestDeposit_MissingUserID(t *testing.T) {
	t.Parallel()
	srv := newTestServer(&mockAccountClient{}, &mockLedgerClient{})

	_, err := srv.Deposit(context.Background(), &walletv1.DepositRequest{
		Amount:         1000,
		IdempotencyKey: "key-1",
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", st.Code())
	}
}

func TestDeposit_ZeroAmount(t *testing.T) {
	t.Parallel()
	srv := newTestServer(&mockAccountClient{}, &mockLedgerClient{})

	_, err := srv.Deposit(context.Background(), &walletv1.DepositRequest{
		UserId:         "user-1",
		Amount:         0,
		IdempotencyKey: "key-1",
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", st.Code())
	}
}

func TestDeposit_NegativeAmount(t *testing.T) {
	t.Parallel()
	srv := newTestServer(&mockAccountClient{}, &mockLedgerClient{})

	_, err := srv.Deposit(context.Background(), &walletv1.DepositRequest{
		UserId:         "user-1",
		Amount:         -500,
		IdempotencyKey: "key-1",
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", st.Code())
	}
}

// --- Downstream error tests ---

func TestDeposit_UserNotFound(t *testing.T) {
	t.Parallel()

	ac := &mockAccountClient{
		getLedgerAccountFn: func(_ context.Context, _ *accountv1.GetLedgerAccountRequest, _ ...grpc.CallOption) (*accountv1.GetLedgerAccountResponse, error) {
			return nil, status.Error(codes.NotFound, "user not found")
		},
	}
	srv := newTestServer(ac, &mockLedgerClient{})

	_, err := srv.Deposit(context.Background(), &walletv1.DepositRequest{
		UserId:         "nonexistent",
		Amount:         1000,
		IdempotencyKey: "key-1",
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.NotFound {
		t.Errorf("code = %v, want NotFound", st.Code())
	}
}

func TestDeposit_AccountServiceInternalError(t *testing.T) {
	t.Parallel()

	ac := &mockAccountClient{
		getLedgerAccountFn: func(_ context.Context, _ *accountv1.GetLedgerAccountRequest, _ ...grpc.CallOption) (*accountv1.GetLedgerAccountResponse, error) {
			return nil, status.Error(codes.Unavailable, "connection refused")
		},
	}
	srv := newTestServer(ac, &mockLedgerClient{})

	_, err := srv.Deposit(context.Background(), &walletv1.DepositRequest{
		UserId:         "user-1",
		Amount:         1000,
		IdempotencyKey: "key-1",
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.Internal {
		t.Errorf("code = %v, want Internal", st.Code())
	}
}

func TestDeposit_LedgerError(t *testing.T) {
	t.Parallel()

	ac := &mockAccountClient{
		getLedgerAccountFn: func(_ context.Context, _ *accountv1.GetLedgerAccountRequest, _ ...grpc.CallOption) (*accountv1.GetLedgerAccountResponse, error) {
			return &accountv1.GetLedgerAccountResponse{LedgerAccountId: "user:user-1"}, nil
		},
	}
	lc := &mockLedgerClient{
		postTransactionFn: func(_ context.Context, _ *ledgerv1.PostTransactionRequest, _ ...grpc.CallOption) (*ledgerv1.PostTransactionResponse, error) {
			return nil, status.Error(codes.FailedPrecondition, "overdraft")
		},
	}
	srv := newTestServer(ac, lc)

	_, err := srv.Deposit(context.Background(), &walletv1.DepositRequest{
		UserId:         "user-1",
		Amount:         1000,
		IdempotencyKey: "key-1",
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.FailedPrecondition {
		t.Errorf("code = %v, want FailedPrecondition", st.Code())
	}
}

// --- Success tests ---

func TestDeposit_Success(t *testing.T) {
	t.Parallel()

	var capturedPostings []*ledgerv1.Posting
	var capturedKey string

	ac := &mockAccountClient{
		getLedgerAccountFn: func(_ context.Context, req *accountv1.GetLedgerAccountRequest, _ ...grpc.CallOption) (*accountv1.GetLedgerAccountResponse, error) {
			if req.UserId != "user-1" {
				t.Errorf("GetLedgerAccount user_id = %q, want %q", req.UserId, "user-1")
			}
			if req.Asset != wallet.DefaultAsset {
				t.Errorf("GetLedgerAccount asset = %q, want %q", req.Asset, wallet.DefaultAsset)
			}
			return &accountv1.GetLedgerAccountResponse{LedgerAccountId: "user:user-1"}, nil
		},
	}
	lc := &mockLedgerClient{
		postTransactionFn: func(_ context.Context, req *ledgerv1.PostTransactionRequest, _ ...grpc.CallOption) (*ledgerv1.PostTransactionResponse, error) {
			capturedPostings = req.Postings
			capturedKey = req.IdempotencyKey
			return &ledgerv1.PostTransactionResponse{
				Transaction: &ledgerv1.Transaction{Id: "tx-abc"},
			}, nil
		},
	}
	srv := newTestServer(ac, lc)

	resp, err := srv.Deposit(context.Background(), &walletv1.DepositRequest{
		UserId:         "user-1",
		Amount:         5000,
		IdempotencyKey: "deposit-key-1",
	})
	if err != nil {
		t.Fatalf("Deposit: %v", err)
	}
	if resp.TransactionId != "tx-abc" {
		t.Errorf("transaction_id = %q, want %q", resp.TransactionId, "tx-abc")
	}
	if capturedKey != "deposit-key-1" {
		t.Errorf("idempotency_key = %q, want %q", capturedKey, "deposit-key-1")
	}

	// Verify postings structure: debit external, credit user.
	if len(capturedPostings) != 2 {
		t.Fatalf("postings count = %d, want 2", len(capturedPostings))
	}

	debit := capturedPostings[0]
	if debit.AccountId != wallet.ExternalDepositsAccount {
		t.Errorf("debit account = %q, want %q", debit.AccountId, wallet.ExternalDepositsAccount)
	}
	if debit.Asset != wallet.DefaultAsset {
		t.Errorf("debit asset = %q, want %q", debit.Asset, wallet.DefaultAsset)
	}
	if debit.Amount != -5000 {
		t.Errorf("debit amount = %d, want %d", debit.Amount, -5000)
	}

	credit := capturedPostings[1]
	if credit.AccountId != "user:user-1" {
		t.Errorf("credit account = %q, want %q", credit.AccountId, "user:user-1")
	}
	if credit.Asset != wallet.DefaultAsset {
		t.Errorf("credit asset = %q, want %q", credit.Asset, wallet.DefaultAsset)
	}
	if credit.Amount != 5000 {
		t.Errorf("credit amount = %d, want %d", credit.Amount, 5000)
	}
}

func TestDeposit_IdempotentReplay(t *testing.T) {
	t.Parallel()

	ac := &mockAccountClient{
		getLedgerAccountFn: func(_ context.Context, _ *accountv1.GetLedgerAccountRequest, _ ...grpc.CallOption) (*accountv1.GetLedgerAccountResponse, error) {
			return &accountv1.GetLedgerAccountResponse{LedgerAccountId: "user:user-1"}, nil
		},
	}
	lc := &mockLedgerClient{
		postTransactionFn: func(_ context.Context, _ *ledgerv1.PostTransactionRequest, _ ...grpc.CallOption) (*ledgerv1.PostTransactionResponse, error) {
			// Ledger handles idempotency transparently — returns the same tx.
			return &ledgerv1.PostTransactionResponse{
				Transaction: &ledgerv1.Transaction{Id: "tx-replay"},
			}, nil
		},
	}
	srv := newTestServer(ac, lc)

	req := &walletv1.DepositRequest{
		UserId:         "user-1",
		Amount:         1000,
		IdempotencyKey: "replay-key",
	}

	resp1, err := srv.Deposit(context.Background(), req)
	if err != nil {
		t.Fatalf("Deposit (first): %v", err)
	}
	resp2, err := srv.Deposit(context.Background(), req)
	if err != nil {
		t.Fatalf("Deposit (replay): %v", err)
	}
	if resp1.TransactionId != resp2.TransactionId {
		t.Errorf("replay tx_id = %q, want %q", resp2.TransactionId, resp1.TransactionId)
	}
}
