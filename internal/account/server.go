package account

import (
	"context"
	"errors"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	accountv1 "github.com/CLAM101/exchange-ledger-platform/proto/gen/account/v1"
)

// Server implements the AccountService gRPC interface.
type Server struct {
	accountv1.UnimplementedAccountServiceServer
	repo   Repository
	logger *zap.Logger
}

// NewServer creates a new AccountService gRPC server.
func NewServer(repo Repository, logger *zap.Logger) *Server {
	return &Server{repo: repo, logger: logger}
}

// CreateUser registers a new user and auto-links the default BTC ledger account.
func (s *Server) CreateUser(ctx context.Context, req *accountv1.CreateUserRequest) (*accountv1.CreateUserResponse, error) {
	u := User{
		Email:          req.Email,
		IdempotencyKey: req.IdempotencyKey,
	}
	if err := u.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	user, err := s.repo.CreateUser(ctx, u)
	if err != nil {
		s.logger.Warn("CreateUser failed",
			zap.String("idempotency_key", req.IdempotencyKey),
			zap.Error(err),
		)
		return nil, domainToStatus(err)
	}

	// Auto-link the default asset account.
	if _, err := s.repo.LinkAssetAccount(ctx, UserAssetAccount{
		UserID:          user.ID,
		Asset:           DefaultAsset,
		LedgerAccountID: LedgerAccountID(user.ID),
	}); err != nil {
		s.logger.Error("LinkAssetAccount failed after user creation",
			zap.String("user_id", string(user.ID)),
			zap.Error(err),
		)
		return nil, status.Error(codes.Internal, "failed to link asset account")
	}

	return &accountv1.CreateUserResponse{
		User: userToProto(user),
	}, nil
}

// GetUser retrieves a user by ID.
func (s *Server) GetUser(ctx context.Context, req *accountv1.GetUserRequest) (*accountv1.GetUserResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	user, err := s.repo.GetUser(ctx, UserID(req.UserId))
	if err != nil {
		return nil, domainToStatus(err)
	}

	return &accountv1.GetUserResponse{
		User: userToProto(user),
	}, nil
}

// GetLedgerAccount returns the ledger account ID for a user and asset pair.
func (s *Server) GetLedgerAccount(ctx context.Context, req *accountv1.GetLedgerAccountRequest) (*accountv1.GetLedgerAccountResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	if req.Asset == "" {
		return nil, status.Error(codes.InvalidArgument, "asset is required")
	}

	ledgerID, err := s.repo.GetLedgerAccountID(ctx, UserID(req.UserId), req.Asset)
	if err != nil {
		return nil, domainToStatus(err)
	}

	return &accountv1.GetLedgerAccountResponse{
		UserId:          req.UserId,
		Asset:           req.Asset,
		LedgerAccountId: ledgerID,
	}, nil
}

// domainToStatus maps domain errors to gRPC status codes.
func domainToStatus(err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, ErrEmailExists):
		return status.Error(codes.AlreadyExists, err.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}

func userToProto(u *User) *accountv1.User {
	return &accountv1.User{
		Id:        string(u.ID),
		Email:     u.Email,
		CreatedAt: timestamppb.New(u.CreatedAt),
	}
}
