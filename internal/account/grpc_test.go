package account_test

import (
	"context"
	"net"
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

	"github.com/CLAM101/exchange-ledger-platform/internal/account"
	"github.com/CLAM101/exchange-ledger-platform/internal/platform/observability"
	accountv1 "github.com/CLAM101/exchange-ledger-platform/proto/gen/account/v1"

	platformgrpc "github.com/CLAM101/exchange-ledger-platform/internal/platform/grpc"
)

const bufSize = 1024 * 1024

// setupGRPC creates an in-memory gRPC server+client pair with the given mock repo.
func setupGRPC(t *testing.T, repo account.Repository) accountv1.AccountServiceClient {
	t.Helper()

	logger := zap.NewNop()
	metrics := observability.NewTestMetrics()
	hs := health.NewServer()
	hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	grpcServer := platformgrpc.NewServer(logger, metrics, hs)
	handler := account.NewServer(repo, logger)
	accountv1.RegisterAccountServiceServer(grpcServer, handler)

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

	return accountv1.NewAccountServiceClient(conn)
}

func TestGRPC_CreateUser_Success(t *testing.T) {
	t.Parallel()
	now := time.Now()

	repo := &mockRepo{
		createUserFn: func(_ context.Context, u account.User) (*account.User, error) {
			return &account.User{
				ID:             "user-grpc-1",
				Email:          u.Email,
				IdempotencyKey: u.IdempotencyKey,
				CreatedAt:      now,
			}, nil
		},
	}

	client := setupGRPC(t, repo)

	resp, err := client.CreateUser(context.Background(), &accountv1.CreateUserRequest{
		Email:          "grpc@example.com",
		IdempotencyKey: "grpc-key-1",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if resp.User.Id != "user-grpc-1" {
		t.Errorf("id = %q, want %q", resp.User.Id, "user-grpc-1")
	}
	if resp.User.Email != "grpc@example.com" {
		t.Errorf("email = %q, want %q", resp.User.Email, "grpc@example.com")
	}
}

func TestGRPC_GetUser_Success(t *testing.T) {
	t.Parallel()
	now := time.Now()

	repo := &mockRepo{
		getUserFn: func(_ context.Context, id account.UserID) (*account.User, error) {
			return &account.User{
				ID:        id,
				Email:     "grpc@example.com",
				CreatedAt: now,
			}, nil
		},
	}

	client := setupGRPC(t, repo)

	resp, err := client.GetUser(context.Background(), &accountv1.GetUserRequest{
		UserId: "user-grpc-2",
	})
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if resp.User.Id != "user-grpc-2" {
		t.Errorf("id = %q, want %q", resp.User.Id, "user-grpc-2")
	}
}

func TestGRPC_GetLedgerAccount_Success(t *testing.T) {
	t.Parallel()

	repo := &mockRepo{
		getLedgerAccountFn: func(_ context.Context, _ account.UserID, _ string) (string, error) {
			return "user:user-grpc-3", nil
		},
	}

	client := setupGRPC(t, repo)

	resp, err := client.GetLedgerAccount(context.Background(), &accountv1.GetLedgerAccountRequest{
		UserId: "user-grpc-3",
		Asset:  "BTC",
	})
	if err != nil {
		t.Fatalf("GetLedgerAccount: %v", err)
	}
	if resp.LedgerAccountId != "user:user-grpc-3" {
		t.Errorf("ledger_account_id = %q, want %q", resp.LedgerAccountId, "user:user-grpc-3")
	}
}

func TestGRPC_CreateUser_InvalidEmail(t *testing.T) {
	t.Parallel()

	client := setupGRPC(t, &mockRepo{})

	_, err := client.CreateUser(context.Background(), &accountv1.CreateUserRequest{
		Email:          "bad-email",
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

func TestGRPC_GetUser_NotFound(t *testing.T) {
	t.Parallel()

	repo := &mockRepo{
		getUserFn: func(_ context.Context, _ account.UserID) (*account.User, error) {
			return nil, account.ErrNotFound
		},
	}

	client := setupGRPC(t, repo)

	_, err := client.GetUser(context.Background(), &accountv1.GetUserRequest{
		UserId: "nonexistent",
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.NotFound {
		t.Errorf("code = %v, want NotFound", st.Code())
	}
}

func TestGRPC_LinkAssetAccount_Success(t *testing.T) {
	t.Parallel()

	repo := &mockRepo{
		linkAssetFn: func(_ context.Context, ua account.UserAssetAccount) (*account.UserAssetAccount, error) {
			return &ua, nil
		},
	}

	client := setupGRPC(t, repo)

	resp, err := client.LinkAssetAccount(context.Background(), &accountv1.LinkAssetAccountRequest{
		UserId: "user-grpc-4",
		Asset:  "ETH",
	})
	if err != nil {
		t.Fatalf("LinkAssetAccount: %v", err)
	}
	if resp.LedgerAccountId != "user:user-grpc-4" {
		t.Errorf("ledger_account_id = %q, want %q", resp.LedgerAccountId, "user:user-grpc-4")
	}
	if resp.Asset != "ETH" {
		t.Errorf("asset = %q, want %q", resp.Asset, "ETH")
	}
}

func TestGRPC_LinkAssetAccount_MissingAsset(t *testing.T) {
	t.Parallel()

	client := setupGRPC(t, &mockRepo{})

	_, err := client.LinkAssetAccount(context.Background(), &accountv1.LinkAssetAccountRequest{
		UserId: "user-grpc-4",
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", st.Code())
	}
}
