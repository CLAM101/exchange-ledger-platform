// Package wallet implements deposit and withdrawal orchestration
// using a reservation model on top of the ledger and account services.
package wallet

import (
	"context"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	accountv1 "github.com/CLAM101/exchange-ledger-platform/proto/gen/account/v1"
	assetv1 "github.com/CLAM101/exchange-ledger-platform/proto/gen/asset/v1"
	ledgerv1 "github.com/CLAM101/exchange-ledger-platform/proto/gen/ledger/v1"
	walletv1 "github.com/CLAM101/exchange-ledger-platform/proto/gen/wallet/v1"
)

// ExternalDepositsAccount is the system account that funds user deposits.
const ExternalDepositsAccount = "external:deposits"

// Server implements the WalletService gRPC interface.
type Server struct {
	walletv1.UnimplementedWalletServiceServer
	accounts accountv1.AccountServiceClient
	assets   assetv1.AssetServiceClient
	ledger   ledgerv1.LedgerServiceClient
	logger   *zap.Logger
}

// NewServer creates a new WalletService gRPC server.
func NewServer(accounts accountv1.AccountServiceClient, assets assetv1.AssetServiceClient, ledger ledgerv1.LedgerServiceClient, logger *zap.Logger) *Server {
	return &Server{accounts: accounts, assets: assets, ledger: ledger, logger: logger}
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
	if req.Asset == "" {
		return nil, status.Error(codes.InvalidArgument, "asset is required")
	}

	// Validate asset exists in the registry.
	if _, err := s.assets.GetAsset(ctx, &assetv1.GetAssetRequest{Symbol: req.Asset}); err != nil {
		return nil, mapDownstreamError(err, "validating asset")
	}

	// Resolve user → ledger account ID, lazy-linking if this is the first
	// deposit for this asset.
	acctResp, err := s.accounts.GetLedgerAccount(ctx, &accountv1.GetLedgerAccountRequest{
		UserId: req.UserId,
		Asset:  req.Asset,
	})
	if err != nil {
		st, ok := status.FromError(err)
		if !ok || st.Code() != codes.NotFound {
			return nil, mapDownstreamError(err, "resolving account")
		}
		// First deposit for this asset — create the link.
		linkResp, linkErr := s.accounts.LinkAssetAccount(ctx, &accountv1.LinkAssetAccountRequest{
			UserId: req.UserId,
			Asset:  req.Asset,
		})
		if linkErr != nil {
			return nil, mapDownstreamError(linkErr, "linking asset account")
		}
		acctResp = &accountv1.GetLedgerAccountResponse{
			UserId:          linkResp.UserId,
			Asset:           linkResp.Asset,
			LedgerAccountId: linkResp.LedgerAccountId,
		}
	}

	// Post double-entry transaction via ledger.
	txResp, err := s.ledger.PostTransaction(ctx, &ledgerv1.PostTransactionRequest{
		IdempotencyKey: req.IdempotencyKey,
		Postings: []*ledgerv1.Posting{
			{AccountId: ExternalDepositsAccount, Asset: req.Asset, Amount: -req.Amount},
			{AccountId: acctResp.LedgerAccountId, Asset: req.Asset, Amount: req.Amount},
		},
	})
	if err != nil {
		return nil, mapDownstreamError(err, "posting transaction")
	}

	s.logger.Info("deposit completed",
		zap.String("user_id", req.UserId),
		zap.String("tx_id", txResp.Transaction.Id),
		zap.String("idempotency_key", req.IdempotencyKey),
		zap.String("asset", req.Asset),
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
