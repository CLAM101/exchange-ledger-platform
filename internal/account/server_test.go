package account_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/CLAM101/exchange-ledger-platform/internal/account"
	accountv1 "github.com/CLAM101/exchange-ledger-platform/proto/gen/account/v1"
)

// mockRepo is a test double for account.Repository.
type mockRepo struct {
	createUserFn       func(ctx context.Context, user account.User) (*account.User, error)
	getUserFn          func(ctx context.Context, id account.UserID) (*account.User, error)
	getUserByEmailFn   func(ctx context.Context, email string) (*account.User, error)
	linkAssetFn        func(ctx context.Context, ua account.UserAssetAccount) (*account.UserAssetAccount, error)
	getLedgerAccountFn func(ctx context.Context, userID account.UserID, asset string) (string, error)
}

func (m *mockRepo) CreateUser(ctx context.Context, user account.User) (*account.User, error) {
	return m.createUserFn(ctx, user)
}

func (m *mockRepo) GetUser(ctx context.Context, id account.UserID) (*account.User, error) {
	return m.getUserFn(ctx, id)
}

func (m *mockRepo) GetUserByEmail(ctx context.Context, email string) (*account.User, error) {
	return m.getUserByEmailFn(ctx, email)
}

func (m *mockRepo) LinkAssetAccount(ctx context.Context, ua account.UserAssetAccount) (*account.UserAssetAccount, error) {
	return m.linkAssetFn(ctx, ua)
}

func (m *mockRepo) GetLedgerAccountID(ctx context.Context, userID account.UserID, asset string) (string, error) {
	return m.getLedgerAccountFn(ctx, userID, asset)
}

func newTestServer(repo account.Repository) *account.Server {
	return account.NewServer(repo, zap.NewNop())
}

// --- CreateUser tests ---

func TestCreateUser_MissingEmail(t *testing.T) {
	t.Parallel()
	srv := newTestServer(&mockRepo{})

	_, err := srv.CreateUser(context.Background(), &accountv1.CreateUserRequest{
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

func TestCreateUser_InvalidEmail(t *testing.T) {
	t.Parallel()
	srv := newTestServer(&mockRepo{})

	_, err := srv.CreateUser(context.Background(), &accountv1.CreateUserRequest{
		Email:          "alice.example.com",
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

func TestCreateUser_MissingIdempotencyKey(t *testing.T) {
	t.Parallel()
	srv := newTestServer(&mockRepo{})

	_, err := srv.CreateUser(context.Background(), &accountv1.CreateUserRequest{
		Email: "alice@example.com",
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", st.Code())
	}
}

func TestCreateUser_Success(t *testing.T) {
	t.Parallel()
	now := time.Now()

	repo := &mockRepo{
		createUserFn: func(_ context.Context, u account.User) (*account.User, error) {
			return &account.User{
				ID:             "user-abc",
				Email:          u.Email,
				IdempotencyKey: u.IdempotencyKey,
				CreatedAt:      now,
			}, nil
		},
	}
	srv := newTestServer(repo)

	resp, err := srv.CreateUser(context.Background(), &accountv1.CreateUserRequest{
		Email:          "alice@example.com",
		IdempotencyKey: "key-1",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if resp.User.Id != "user-abc" {
		t.Errorf("id = %q, want %q", resp.User.Id, "user-abc")
	}
	if resp.User.Email != "alice@example.com" {
		t.Errorf("email = %q, want %q", resp.User.Email, "alice@example.com")
	}
}

func TestCreateUser_DuplicateEmail(t *testing.T) {
	t.Parallel()
	repo := &mockRepo{
		createUserFn: func(_ context.Context, _ account.User) (*account.User, error) {
			return nil, account.ErrEmailExists
		},
	}
	srv := newTestServer(repo)

	_, err := srv.CreateUser(context.Background(), &accountv1.CreateUserRequest{
		Email:          "alice@example.com",
		IdempotencyKey: "key-dup",
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.AlreadyExists {
		t.Errorf("code = %v, want AlreadyExists", st.Code())
	}
}

func TestCreateUser_RepoError(t *testing.T) {
	t.Parallel()
	repo := &mockRepo{
		createUserFn: func(_ context.Context, _ account.User) (*account.User, error) {
			return nil, errors.New("db connection lost")
		},
	}
	srv := newTestServer(repo)

	_, err := srv.CreateUser(context.Background(), &accountv1.CreateUserRequest{
		Email:          "alice@example.com",
		IdempotencyKey: "key-err",
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.Internal {
		t.Errorf("code = %v, want Internal", st.Code())
	}
}

// --- GetUser tests ---

func TestGetUser_MissingUserID(t *testing.T) {
	t.Parallel()
	srv := newTestServer(&mockRepo{})

	_, err := srv.GetUser(context.Background(), &accountv1.GetUserRequest{})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", st.Code())
	}
}

func TestGetUser_Success(t *testing.T) {
	t.Parallel()
	now := time.Now()

	repo := &mockRepo{
		getUserFn: func(_ context.Context, id account.UserID) (*account.User, error) {
			return &account.User{
				ID:        id,
				Email:     "alice@example.com",
				CreatedAt: now,
			}, nil
		},
	}
	srv := newTestServer(repo)

	resp, err := srv.GetUser(context.Background(), &accountv1.GetUserRequest{
		UserId: "user-abc",
	})
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if resp.User.Id != "user-abc" {
		t.Errorf("id = %q, want %q", resp.User.Id, "user-abc")
	}
	if resp.User.Email != "alice@example.com" {
		t.Errorf("email = %q, want %q", resp.User.Email, "alice@example.com")
	}
}

func TestGetUser_NotFound(t *testing.T) {
	t.Parallel()
	repo := &mockRepo{
		getUserFn: func(_ context.Context, _ account.UserID) (*account.User, error) {
			return nil, account.ErrNotFound
		},
	}
	srv := newTestServer(repo)

	_, err := srv.GetUser(context.Background(), &accountv1.GetUserRequest{
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

// --- GetLedgerAccount tests ---

func TestGetLedgerAccount_MissingUserID(t *testing.T) {
	t.Parallel()
	srv := newTestServer(&mockRepo{})

	_, err := srv.GetLedgerAccount(context.Background(), &accountv1.GetLedgerAccountRequest{
		Asset: "BTC",
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", st.Code())
	}
}

func TestGetLedgerAccount_MissingAsset(t *testing.T) {
	t.Parallel()
	srv := newTestServer(&mockRepo{})

	_, err := srv.GetLedgerAccount(context.Background(), &accountv1.GetLedgerAccountRequest{
		UserId: "user-abc",
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", st.Code())
	}
}

func TestGetLedgerAccount_Success(t *testing.T) {
	t.Parallel()
	repo := &mockRepo{
		getLedgerAccountFn: func(_ context.Context, _ account.UserID, _ string) (string, error) {
			return "user:user-abc", nil
		},
	}
	srv := newTestServer(repo)

	resp, err := srv.GetLedgerAccount(context.Background(), &accountv1.GetLedgerAccountRequest{
		UserId: "user-abc",
		Asset:  "BTC",
	})
	if err != nil {
		t.Fatalf("GetLedgerAccount: %v", err)
	}
	if resp.UserId != "user-abc" {
		t.Errorf("user_id = %q, want %q", resp.UserId, "user-abc")
	}
	if resp.Asset != "BTC" {
		t.Errorf("asset = %q, want %q", resp.Asset, "BTC")
	}
	if resp.LedgerAccountId != "user:user-abc" {
		t.Errorf("ledger_account_id = %q, want %q", resp.LedgerAccountId, "user:user-abc")
	}
}

func TestGetLedgerAccount_NotFound(t *testing.T) {
	t.Parallel()
	repo := &mockRepo{
		getLedgerAccountFn: func(_ context.Context, _ account.UserID, _ string) (string, error) {
			return "", account.ErrNotFound
		},
	}
	srv := newTestServer(repo)

	_, err := srv.GetLedgerAccount(context.Background(), &accountv1.GetLedgerAccountRequest{
		UserId: "nonexistent",
		Asset:  "BTC",
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.NotFound {
		t.Errorf("code = %v, want NotFound", st.Code())
	}
}

// --- LinkAssetAccount tests ---

func TestLinkAssetAccount_MissingUserID(t *testing.T) {
	t.Parallel()
	srv := newTestServer(&mockRepo{})

	_, err := srv.LinkAssetAccount(context.Background(), &accountv1.LinkAssetAccountRequest{
		Asset: "BTC",
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", st.Code())
	}
}

func TestLinkAssetAccount_MissingAsset(t *testing.T) {
	t.Parallel()
	srv := newTestServer(&mockRepo{})

	_, err := srv.LinkAssetAccount(context.Background(), &accountv1.LinkAssetAccountRequest{
		UserId: "user-abc",
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", st.Code())
	}
}

func TestLinkAssetAccount_Success(t *testing.T) {
	t.Parallel()

	var capturedUA account.UserAssetAccount
	repo := &mockRepo{
		linkAssetFn: func(_ context.Context, ua account.UserAssetAccount) (*account.UserAssetAccount, error) {
			capturedUA = ua
			return &ua, nil
		},
	}
	srv := newTestServer(repo)

	resp, err := srv.LinkAssetAccount(context.Background(), &accountv1.LinkAssetAccountRequest{
		UserId: "user-abc",
		Asset:  "ETH",
	})
	if err != nil {
		t.Fatalf("LinkAssetAccount: %v", err)
	}
	if resp.UserId != "user-abc" {
		t.Errorf("user_id = %q, want %q", resp.UserId, "user-abc")
	}
	if resp.Asset != "ETH" {
		t.Errorf("asset = %q, want %q", resp.Asset, "ETH")
	}
	if resp.LedgerAccountId != "user:user-abc" {
		t.Errorf("ledger_account_id = %q, want %q", resp.LedgerAccountId, "user:user-abc")
	}
	if capturedUA.UserID != "user-abc" {
		t.Errorf("repo user_id = %q, want %q", capturedUA.UserID, "user-abc")
	}
}
