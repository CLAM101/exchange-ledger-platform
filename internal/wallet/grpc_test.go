package wallet_test

import (
	"context"
	"net"
	"testing"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/CLAM101/exchange-ledger-platform/internal/platform/observability"
	"github.com/CLAM101/exchange-ledger-platform/internal/wallet"
	accountv1 "github.com/CLAM101/exchange-ledger-platform/proto/gen/account/v1"
	ledgerv1 "github.com/CLAM101/exchange-ledger-platform/proto/gen/ledger/v1"
	walletv1 "github.com/CLAM101/exchange-ledger-platform/proto/gen/wallet/v1"

	platformgrpc "github.com/CLAM101/exchange-ledger-platform/internal/platform/grpc"
)

const bufSize = 1024 * 1024

// setupGRPC creates an in-memory gRPC server+client pair for the wallet service.
func setupGRPC(t *testing.T, ac accountv1.AccountServiceClient, lc ledgerv1.LedgerServiceClient) walletv1.WalletServiceClient {
	t.Helper()

	logger := zap.NewNop()
	metrics := observability.NewTestMetrics()
	hs := health.NewServer()
	hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	grpcServer := platformgrpc.NewServer(logger, metrics, hs)
	handler := wallet.NewServer(ac, lc, logger)
	walletv1.RegisterWalletServiceServer(grpcServer, handler)

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

	return walletv1.NewWalletServiceClient(conn)
}

func TestGRPC_Deposit_Success(t *testing.T) {
	t.Parallel()

	ac := &mockAccountClient{
		getLedgerAccountFn: func(_ context.Context, _ *accountv1.GetLedgerAccountRequest, _ ...grpc.CallOption) (*accountv1.GetLedgerAccountResponse, error) {
			return &accountv1.GetLedgerAccountResponse{LedgerAccountId: "user:user-1"}, nil
		},
	}
	lc := &mockLedgerClient{
		postTransactionFn: func(_ context.Context, _ *ledgerv1.PostTransactionRequest, _ ...grpc.CallOption) (*ledgerv1.PostTransactionResponse, error) {
			return &ledgerv1.PostTransactionResponse{
				Transaction: &ledgerv1.Transaction{Id: "tx-grpc-1"},
			}, nil
		},
	}

	client := setupGRPC(t, ac, lc)

	resp, err := client.Deposit(context.Background(), &walletv1.DepositRequest{
		UserId:         "user-1",
		Amount:         3000,
		IdempotencyKey: "grpc-key-1",
	})
	if err != nil {
		t.Fatalf("Deposit: %v", err)
	}
	if resp.TransactionId != "tx-grpc-1" {
		t.Errorf("transaction_id = %q, want %q", resp.TransactionId, "tx-grpc-1")
	}
}

func TestGRPC_Deposit_InvalidArgument(t *testing.T) {
	t.Parallel()

	client := setupGRPC(t, &mockAccountClient{}, &mockLedgerClient{})

	_, err := client.Deposit(context.Background(), &walletv1.DepositRequest{
		Amount:         1000,
		IdempotencyKey: "key-1",
		// Missing user_id
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", st.Code())
	}
}

func TestGRPC_Deposit_UserNotFound(t *testing.T) {
	t.Parallel()

	ac := &mockAccountClient{
		getLedgerAccountFn: func(_ context.Context, _ *accountv1.GetLedgerAccountRequest, _ ...grpc.CallOption) (*accountv1.GetLedgerAccountResponse, error) {
			return nil, status.Error(codes.NotFound, "user not found")
		},
	}

	client := setupGRPC(t, ac, &mockLedgerClient{})

	_, err := client.Deposit(context.Background(), &walletv1.DepositRequest{
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
