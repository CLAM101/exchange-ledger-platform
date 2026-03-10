package ledger_test

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/CLAM101/exchange-ledger-platform/internal/ledger"
	"github.com/CLAM101/exchange-ledger-platform/internal/platform/observability"
	ledgerv1 "github.com/CLAM101/exchange-ledger-platform/proto/gen/ledger/v1"

	platformgrpc "github.com/CLAM101/exchange-ledger-platform/internal/platform/grpc"
)

const bufSize = 1024 * 1024

// setupGRPC creates an in-memory gRPC server+client pair with the given mock repo.
func setupGRPC(t *testing.T, repo ledger.Repository) ledgerv1.LedgerServiceClient {
	t.Helper()

	logger := zap.NewNop()
	metrics := observability.NewTestMetrics()
	hs := health.NewServer()
	hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	grpcServer := platformgrpc.NewServer(logger, metrics, hs)
	handler := ledger.NewServer(repo, logger)
	ledgerv1.RegisterLedgerServiceServer(grpcServer, handler)

	lis := bufconn.Listen(bufSize)
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			t.Logf("gRPC server exited: %v", err)
		}
	}()
	t.Cleanup(func() { grpcServer.Stop() })

	conn, err := grpc.NewClient(
		"passthrough:///bufconn",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	return ledgerv1.NewLedgerServiceClient(conn)
}

func TestGRPC_PostTransaction_Success(t *testing.T) {
	t.Parallel()
	now := time.Now()

	repo := &mockRepo{
		postTxFn: func(_ context.Context, tx ledger.Transaction) (*ledger.Transaction, error) {
			return &ledger.Transaction{
				ID:             "tx-grpc-1",
				IdempotencyKey: tx.IdempotencyKey,
				Postings:       tx.Postings,
				CreatedAt:      now,
			}, nil
		},
	}

	client := setupGRPC(t, repo)

	resp, err := client.PostTransaction(context.Background(), &ledgerv1.PostTransactionRequest{
		IdempotencyKey: "grpc-key-1",
		Postings: []*ledgerv1.Posting{
			{AccountId: "a", Asset: "BTC", Amount: -100},
			{AccountId: "b", Asset: "BTC", Amount: 100},
		},
	})
	if err != nil {
		t.Fatalf("PostTransaction: %v", err)
	}
	if resp.Transaction.Id != "tx-grpc-1" {
		t.Errorf("id = %q, want %q", resp.Transaction.Id, "tx-grpc-1")
	}
	if resp.Transaction.IdempotencyKey != "grpc-key-1" {
		t.Errorf("key = %q, want %q", resp.Transaction.IdempotencyKey, "grpc-key-1")
	}
	if len(resp.Transaction.Postings) != 2 {
		t.Errorf("postings = %d, want 2", len(resp.Transaction.Postings))
	}
}

func TestGRPC_PostTransaction_MissingKey(t *testing.T) {
	t.Parallel()

	client := setupGRPC(t, &mockRepo{})

	_, err := client.PostTransaction(context.Background(), &ledgerv1.PostTransactionRequest{
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

func TestGRPC_PostTransaction_Overdraft(t *testing.T) {
	t.Parallel()

	repo := &mockRepo{
		postTxFn: func(_ context.Context, _ ledger.Transaction) (*ledger.Transaction, error) {
			return nil, ledger.ErrOverdraft
		},
	}

	client := setupGRPC(t, repo)

	_, err := client.PostTransaction(context.Background(), &ledgerv1.PostTransactionRequest{
		IdempotencyKey: "grpc-key-od",
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

func TestGRPC_GetBalance_Success(t *testing.T) {
	t.Parallel()

	repo := &mockRepo{
		getBalFn: func(_ context.Context, id ledger.AccountID, asset ledger.Asset) (ledger.Amount, error) {
			return 42000, nil
		},
	}

	client := setupGRPC(t, repo)

	resp, err := client.GetBalance(context.Background(), &ledgerv1.GetBalanceRequest{
		AccountId: "acc-grpc-1",
		Asset:     "BTC",
	})
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if resp.Balance != 42000 {
		t.Errorf("balance = %d, want 42000", resp.Balance)
	}
	if resp.AccountId != "acc-grpc-1" {
		t.Errorf("account_id = %q, want %q", resp.AccountId, "acc-grpc-1")
	}
	if resp.Asset != "BTC" {
		t.Errorf("asset = %q, want %q", resp.Asset, "BTC")
	}
}

func TestGRPC_ListEntries_Success(t *testing.T) {
	t.Parallel()
	now := time.Now()

	entries := make([]ledger.Entry, 5)
	for i := range entries {
		entries[i] = ledger.Entry{
			EntryID:   int64(i + 1),
			TxID:      "tx-" + strconv.Itoa(i),
			AccountID: "acc-grpc-2",
			Asset:     "BTC",
			Amount:    -100,
			CreatedAt: now,
		}
	}

	repo := &mockRepo{
		listEntFn: func(_ context.Context, _ ledger.AccountID, _ ledger.Asset, cursor int64, limit int) ([]ledger.Entry, error) {
			var result []ledger.Entry
			for _, e := range entries {
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

	client := setupGRPC(t, repo)

	// First page: page_size=2
	resp1, err := client.ListEntries(context.Background(), &ledgerv1.ListEntriesRequest{
		AccountId: "acc-grpc-2",
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
	resp2, err := client.ListEntries(context.Background(), &ledgerv1.ListEntriesRequest{
		AccountId: "acc-grpc-2",
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

	// Third page — 1 remaining, no next token
	resp3, err := client.ListEntries(context.Background(), &ledgerv1.ListEntriesRequest{
		AccountId: "acc-grpc-2",
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
