// Package wallet implements deposit and withdrawal orchestration
// using a reservation model on top of the ledger and account services.
package wallet

import (
	"context"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	accountv1 "github.com/CLAM101/exchange-ledger-platform/proto/gen/account/v1"
	ledgerv1 "github.com/CLAM101/exchange-ledger-platform/proto/gen/ledger/v1"
	walletv1 "github.com/CLAM101/exchange-ledger-platform/proto/gen/wallet/v1"
)

const (
	// ExternalDepositsAccount is the system account that funds user deposits.
	ExternalDepositsAccount = "external:deposits"

	// DefaultAsset is the default asset for wallet operations.
	DefaultAsset = "BTC"
)

// Server implements the WalletService gRPC interface.
type Server struct {
	walletv1.UnimplementedWalletServiceServer
	accounts accountv1.AccountServiceClient
	ledger   ledgerv1.LedgerServiceClient
	logger   *zap.Logger
}

// NewServer creates a new WalletService gRPC server.
func NewServer(accounts accountv1.AccountServiceClient, ledger ledgerv1.LedgerServiceClient, logger *zap.Logger) *Server {
	return &Server{accounts: accounts, ledger: ledger, logger: logger}
}

// Deposit credits a user's account from the external deposits source.
func (s *Server) Deposit(ctx context.Context, req *walletv1.DepositRequest) (*walletv1.DepositResponse, error) {
	// Validate request.
	if req.IdempotencyKey == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency_key is required")
	}
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	if req.Amount <= 0 {
		return nil, status.Error(codes.InvalidArgument, "amount must be positive")
	}

	// Resolve user → ledger account ID.
	acctResp, err := s.accounts.GetLedgerAccount(ctx, &accountv1.GetLedgerAccountRequest{
		UserId: req.UserId,
		Asset:  DefaultAsset,
	})
	if err != nil {
		return nil, mapDownstreamError(err, "resolving account")
	}

	// Post double-entry transaction via ledger.
	txResp, err := s.ledger.PostTransaction(ctx, &ledgerv1.PostTransactionRequest{
		IdempotencyKey: req.IdempotencyKey,
		Postings: []*ledgerv1.Posting{
			{AccountId: ExternalDepositsAccount, Asset: DefaultAsset, Amount: -req.Amount},
			{AccountId: acctResp.LedgerAccountId, Asset: DefaultAsset, Amount: req.Amount},
		},
	})
	if err != nil {
		return nil, mapDownstreamError(err, "posting transaction")
	}

	s.logger.Info("deposit completed",
		zap.String("user_id", req.UserId),
		zap.String("tx_id", txResp.Transaction.Id),
		zap.String("idempotency_key", req.IdempotencyKey),
		zap.Int64("amount", req.Amount),
	)

	return &walletv1.DepositResponse{
		TransactionId: txResp.Transaction.Id,
	}, nil
}

// mapDownstreamError converts downstream gRPC errors to appropriate wallet-level codes.
func mapDownstreamError(err error, context string) error {
	st, ok := status.FromError(err)
	if !ok {
		return status.Errorf(codes.Internal, "%s: %v", context, err)
	}

	switch st.Code() {
	case codes.NotFound:
		return status.Error(codes.NotFound, st.Message())
	case codes.InvalidArgument:
		return status.Error(codes.InvalidArgument, st.Message())
	case codes.FailedPrecondition:
		return status.Error(codes.FailedPrecondition, st.Message())
	default:
		return status.Errorf(codes.Internal, "%s: %s", context, st.Message())
	}
}
