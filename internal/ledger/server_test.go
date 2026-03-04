package ledger_test

import (
	"context"
	"encoding/base64"
	"strconv"
	"testing"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/CLAM101/exchange-ledger-platform/internal/ledger"
	ledgerv1 "github.com/CLAM101/exchange-ledger-platform/proto/gen/ledger/v1"
)

// mockRepo is a test double for ledger.Repository.
type mockRepo struct {
	postTxFn  func(ctx context.Context, tx ledger.Transaction) (*ledger.Transaction, error)
	getTxFn   func(ctx context.Context, key string) (*ledger.Transaction, error)
	getBalFn  func(ctx context.Context, id ledger.AccountID, asset ledger.Asset) (ledger.Amount, error)
	listEntFn func(ctx context.Context, id ledger.AccountID, asset ledger.Asset, cursor int64, limit int) ([]ledger.Entry, error)
}

func (m *mockRepo) PostTransaction(ctx context.Context, tx ledger.Transaction) (*ledger.Transaction, error) {
	return m.postTxFn(ctx, tx)
}

func (m *mockRepo) GetTransaction(ctx context.Context, key string) (*ledger.Transaction, error) {
	return m.getTxFn(ctx, key)
}

func (m *mockRepo) GetBalance(ctx context.Context, id ledger.AccountID, asset ledger.Asset) (ledger.Amount, error) {
	return m.getBalFn(ctx, id, asset)
}

func (m *mockRepo) ListEntries(ctx context.Context, id ledger.AccountID, asset ledger.Asset, cursor int64, limit int) ([]ledger.Entry, error) {
	return m.listEntFn(ctx, id, asset, cursor, limit)
}

func newTestServer(repo ledger.Repository) *ledger.Server {
	return ledger.NewServer(repo, zap.NewNop())
}

// --- PostTransaction tests ---

func TestPostTransaction_MissingIdempotencyKey(t *testing.T) {
	t.Parallel()
	srv := newTestServer(&mockRepo{})

	_, err := srv.PostTransaction(context.Background(), &ledgerv1.PostTransactionRequest{
		Postings: []*ledgerv1.Posting{
			{AccountId: "a", Asset: "BTC", Amount: -100},
			{AccountId: "b", Asset: "BTC", Amount: 100},
		},
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", st.Code())
	}
}

func TestPostTransaction_SuccessUnit(t *testing.T) {
	t.Parallel()
	now := time.Now()

	repo := &mockRepo{
		postTxFn: func(_ context.Context, tx ledger.Transaction) (*ledger.Transaction, error) {
			return &ledger.Transaction{
				ID:             "tx-123",
				IdempotencyKey: tx.IdempotencyKey,
				Postings:       tx.Postings,
				CreatedAt:      now,
			}, nil
		},
	}
	srv := newTestServer(repo)

	resp, err := srv.PostTransaction(context.Background(), &ledgerv1.PostTransactionRequest{
		IdempotencyKey: "key-1",
		Postings: []*ledgerv1.Posting{
			{AccountId: "a", Asset: "BTC", Amount: -100},
			{AccountId: "b", Asset: "BTC", Amount: 100},
		},
	})
	if err != nil {
		t.Fatalf("PostTransaction: %v", err)
	}
	if resp.Transaction.Id != "tx-123" {
		t.Errorf("id = %q, want %q", resp.Transaction.Id, "tx-123")
	}
	if resp.Transaction.IdempotencyKey != "key-1" {
		t.Errorf("key = %q, want %q", resp.Transaction.IdempotencyKey, "key-1")
	}
	if len(resp.Transaction.Postings) != 2 {
		t.Errorf("postings = %d, want 2", len(resp.Transaction.Postings))
	}
}

func TestPostTransaction_OverdraftUnit(t *testing.T) {
	t.Parallel()
	repo := &mockRepo{
		postTxFn: func(_ context.Context, _ ledger.Transaction) (*ledger.Transaction, error) {
			return nil, ledger.ErrOverdraft
		},
	}
	srv := newTestServer(repo)

	_, err := srv.PostTransaction(context.Background(), &ledgerv1.PostTransactionRequest{
		IdempotencyKey: "key-od",
		Postings: []*ledgerv1.Posting{
			{AccountId: "a", Asset: "BTC", Amount: -100},
			{AccountId: "b", Asset: "BTC", Amount: 100},
		},
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.FailedPrecondition {
		t.Errorf("code = %v, want FailedPrecondition", st.Code())
	}
}

func TestPostTransaction_ValidationError(t *testing.T) {
	t.Parallel()
	repo := &mockRepo{
		postTxFn: func(_ context.Context, _ ledger.Transaction) (*ledger.Transaction, error) {
			return nil, ledger.ErrUnbalanced
		},
	}
	srv := newTestServer(repo)

	_, err := srv.PostTransaction(context.Background(), &ledgerv1.PostTransactionRequest{
		IdempotencyKey: "key-unbal",
		Postings: []*ledgerv1.Posting{
			{AccountId: "a", Asset: "BTC", Amount: -100},
			{AccountId: "b", Asset: "BTC", Amount: 50},
		},
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", st.Code())
	}
}

// --- GetBalance tests ---

func TestGetBalance_MissingFields(t *testing.T) {
	t.Parallel()
	srv := newTestServer(&mockRepo{})

	tests := []struct {
		name string
		req  *ledgerv1.GetBalanceRequest
	}{
		{"missing account_id", &ledgerv1.GetBalanceRequest{Asset: "BTC"}},
		{"missing asset", &ledgerv1.GetBalanceRequest{AccountId: "acc-1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := srv.GetBalance(context.Background(), tt.req)
			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("expected gRPC status error, got %v", err)
			}
			if st.Code() != codes.InvalidArgument {
				t.Errorf("code = %v, want InvalidArgument", st.Code())
			}
		})
	}
}

func TestGetBalance_Success(t *testing.T) {
	t.Parallel()
	repo := &mockRepo{
		getBalFn: func(_ context.Context, id ledger.AccountID, asset ledger.Asset) (ledger.Amount, error) {
			return 42000, nil
		},
	}
	srv := newTestServer(repo)

	resp, err := srv.GetBalance(context.Background(), &ledgerv1.GetBalanceRequest{
		AccountId: "acc-1",
		Asset:     "BTC",
	})
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if resp.Balance != 42000 {
		t.Errorf("balance = %d, want 42000", resp.Balance)
	}
	if resp.AccountId != "acc-1" {
		t.Errorf("account_id = %q, want %q", resp.AccountId, "acc-1")
	}
	if resp.Asset != "BTC" {
		t.Errorf("asset = %q, want %q", resp.Asset, "BTC")
	}
}

// --- ListEntries tests ---

func TestListEntries_MissingFields(t *testing.T) {
	t.Parallel()
	srv := newTestServer(&mockRepo{})

	tests := []struct {
		name string
		req  *ledgerv1.ListEntriesRequest
	}{
		{"missing account_id", &ledgerv1.ListEntriesRequest{Asset: "BTC"}},
		{"missing asset", &ledgerv1.ListEntriesRequest{AccountId: "acc-1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := srv.ListEntries(context.Background(), tt.req)
			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("expected gRPC status error, got %v", err)
			}
			if st.Code() != codes.InvalidArgument {
				t.Errorf("code = %v, want InvalidArgument", st.Code())
			}
		})
	}
}

func TestListEntries_DefaultPageSize(t *testing.T) {
	t.Parallel()

	var capturedLimit int
	repo := &mockRepo{
		listEntFn: func(_ context.Context, _ ledger.AccountID, _ ledger.Asset, _ int64, limit int) ([]ledger.Entry, error) {
			capturedLimit = limit
			return nil, nil
		},
	}
	srv := newTestServer(repo)

	_, err := srv.ListEntries(context.Background(), &ledgerv1.ListEntriesRequest{
		AccountId: "acc-1",
		Asset:     "BTC",
		// page_size = 0 → default
	})
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	// Default is 50; repo gets limit+1=51 for next-page detection.
	if capturedLimit != 51 {
		t.Errorf("captured limit = %d, want 51", capturedLimit)
	}
}

func TestListEntries_CapsPageSize(t *testing.T) {
	t.Parallel()

	var capturedLimit int
	repo := &mockRepo{
		listEntFn: func(_ context.Context, _ ledger.AccountID, _ ledger.Asset, _ int64, limit int) ([]ledger.Entry, error) {
			capturedLimit = limit
			return nil, nil
		},
	}
	srv := newTestServer(repo)

	_, err := srv.ListEntries(context.Background(), &ledgerv1.ListEntriesRequest{
		AccountId: "acc-1",
		Asset:     "BTC",
		PageSize:  9999,
	})
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	// Max is 200; repo gets limit+1=201.
	if capturedLimit != 201 {
		t.Errorf("captured limit = %d, want 201", capturedLimit)
	}
}

func TestListEntries_PaginationUnit(t *testing.T) {
	t.Parallel()

	now := time.Now()
	allEntries := make([]ledger.Entry, 5)
	for i := range allEntries {
		allEntries[i] = ledger.Entry{
			EntryID:   int64(i + 1),
			TxID:      "tx-" + strconv.Itoa(i),
			AccountID: "acc-1",
			Asset:     "BTC",
			Amount:    -100,
			CreatedAt: now,
		}
	}

	repo := &mockRepo{
		listEntFn: func(_ context.Context, _ ledger.AccountID, _ ledger.Asset, cursor int64, limit int) ([]ledger.Entry, error) {
			var result []ledger.Entry
			for _, e := range allEntries {
				if e.EntryID > cursor {
					result = append(result, e)
				}
			}
			if len(result) > limit {
				result = result[:limit]
			}
			return result, nil
		},
	}
	srv := newTestServer(repo)

	// First page: page_size=2
	resp1, err := srv.ListEntries(context.Background(), &ledgerv1.ListEntriesRequest{
		AccountId: "acc-1",
		Asset:     "BTC",
		PageSize:  2,
	})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if len(resp1.Entries) != 2 {
		t.Fatalf("page 1: expected 2 entries, got %d", len(resp1.Entries))
	}
	if resp1.NextPageToken == "" {
		t.Fatal("page 1: expected next_page_token")
	}

	// Second page using token
	resp2, err := srv.ListEntries(context.Background(), &ledgerv1.ListEntriesRequest{
		AccountId: "acc-1",
		Asset:     "BTC",
		PageSize:  2,
		PageToken: resp1.NextPageToken,
	})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(resp2.Entries) != 2 {
		t.Fatalf("page 2: expected 2 entries, got %d", len(resp2.Entries))
	}
	if resp2.NextPageToken == "" {
		t.Fatal("page 2: expected next_page_token")
	}

	// Third page — 1 remaining, no next token
	resp3, err := srv.ListEntries(context.Background(), &ledgerv1.ListEntriesRequest{
		AccountId: "acc-1",
		Asset:     "BTC",
		PageSize:  2,
		PageToken: resp2.NextPageToken,
	})
	if err != nil {
		t.Fatalf("page 3: %v", err)
	}
	if len(resp3.Entries) != 1 {
		t.Fatalf("page 3: expected 1 entry, got %d", len(resp3.Entries))
	}
	if resp3.NextPageToken != "" {
		t.Errorf("page 3: expected empty next_page_token, got %q", resp3.NextPageToken)
	}
}

func TestListEntries_InvalidToken(t *testing.T) {
	t.Parallel()
	srv := newTestServer(&mockRepo{})

	_, err := srv.ListEntries(context.Background(), &ledgerv1.ListEntriesRequest{
		AccountId: "acc-1",
		Asset:     "BTC",
		PageToken: "not-valid-base64-number",
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", st.Code())
	}
}

func TestListEntries_ValidTokenDecodesToCursor(t *testing.T) {
	t.Parallel()

	var capturedCursor int64
	repo := &mockRepo{
		listEntFn: func(_ context.Context, _ ledger.AccountID, _ ledger.Asset, cursor int64, _ int) ([]ledger.Entry, error) {
			capturedCursor = cursor
			return nil, nil
		},
	}
	srv := newTestServer(repo)

	// Encode cursor 42 as base64.
	token := base64.StdEncoding.EncodeToString([]byte("42"))

	_, err := srv.ListEntries(context.Background(), &ledgerv1.ListEntriesRequest{
		AccountId: "acc-1",
		Asset:     "BTC",
		PageToken: token,
	})
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if capturedCursor != 42 {
		t.Errorf("cursor = %d, want 42", capturedCursor)
	}
}
