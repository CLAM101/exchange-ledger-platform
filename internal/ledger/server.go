package ledger

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	ledgerv1 "github.com/CLAM101/exchange-ledger-platform/proto/gen/ledger/v1"
)

const (
	defaultPageSize = 50
	maxPageSize     = 200
)

// Server implements the LedgerService gRPC interface.
type Server struct {
	ledgerv1.UnimplementedLedgerServiceServer
	repo   Repository
	logger *zap.Logger
}

// NewServer creates a new LedgerService gRPC server.
func NewServer(repo Repository, logger *zap.Logger) *Server {
	return &Server{repo: repo, logger: logger}
}

// PostTransaction atomically records a balanced double-entry transaction.
func (s *Server) PostTransaction(ctx context.Context, req *ledgerv1.PostTransactionRequest) (*ledgerv1.PostTransactionResponse, error) {
	if req.IdempotencyKey == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency_key is required")
	}
	if len(req.Postings) < 2 {
		return nil, status.Error(codes.InvalidArgument, "at least two postings are required")
	}

	postings := make([]Posting, len(req.Postings))
	for i, p := range req.Postings {
		postings[i] = Posting{
			AccountID: AccountID(p.AccountId),
			Asset:     Asset(p.Asset),
			Amount:    Amount(p.Amount),
		}
	}

	tx := Transaction{
		IdempotencyKey: req.IdempotencyKey,
		Postings:       postings,
	}

	result, err := s.repo.PostTransaction(ctx, tx)
	if err != nil {
		s.logger.Warn("PostTransaction failed",
			zap.String("idempotency_key", req.IdempotencyKey),
			zap.Error(err),
		)
		return nil, domainToStatus(err)
	}

	return &ledgerv1.PostTransactionResponse{
		Transaction: transactionToProto(result),
	}, nil
}

// GetBalance returns the current balance for an account and asset pair.
func (s *Server) GetBalance(ctx context.Context, req *ledgerv1.GetBalanceRequest) (*ledgerv1.GetBalanceResponse, error) {
	if req.AccountId == "" {
		return nil, status.Error(codes.InvalidArgument, "account_id is required")
	}
	if req.Asset == "" {
		return nil, status.Error(codes.InvalidArgument, "asset is required")
	}

	balance, err := s.repo.GetBalance(ctx, AccountID(req.AccountId), Asset(req.Asset))
	if err != nil {
		return nil, domainToStatus(err)
	}

	return &ledgerv1.GetBalanceResponse{
		AccountId: req.AccountId,
		Asset:     req.Asset,
		Balance:   int64(balance),
	}, nil
}

// ListEntries returns ledger entries for an account and asset with cursor-based pagination.
func (s *Server) ListEntries(ctx context.Context, req *ledgerv1.ListEntriesRequest) (*ledgerv1.ListEntriesResponse, error) {
	if req.AccountId == "" {
		return nil, status.Error(codes.InvalidArgument, "account_id is required")
	}
	if req.Asset == "" {
		return nil, status.Error(codes.InvalidArgument, "asset is required")
	}

	pageSize := clampPageSize(int(req.PageSize))

	cursor, err := decodePageToken(req.PageToken)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}

	// Request one extra to detect whether there's a next page.
	entries, err := s.repo.ListEntries(ctx, AccountID(req.AccountId), Asset(req.Asset), cursor, pageSize+1)
	if err != nil {
		return nil, domainToStatus(err)
	}

	var nextToken string
	if len(entries) > pageSize {
		entries = entries[:pageSize]
		nextToken = encodePageToken(entries[pageSize-1].EntryID)
	}

	pbEntries := make([]*ledgerv1.Entry, len(entries))
	for i, e := range entries {
		pbEntries[i] = &ledgerv1.Entry{
			EntryId:   e.EntryID,
			TxId:      e.TxID,
			AccountId: string(e.AccountID),
			Asset:     string(e.Asset),
			Amount:    int64(e.Amount),
			CreatedAt: timestamppb.New(e.CreatedAt),
		}
	}

	return &ledgerv1.ListEntriesResponse{
		Entries:       pbEntries,
		NextPageToken: nextToken,
	}, nil
}

// domainToStatus maps domain errors to gRPC status codes.
func domainToStatus(err error) error {
	switch {
	case errors.Is(err, ErrNoPostings),
		errors.Is(err, ErrZeroAmount),
		errors.Is(err, ErrAssetMismatch),
		errors.Is(err, ErrUnbalanced):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, ErrOverdraft):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

func transactionToProto(tx *Transaction) *ledgerv1.Transaction {
	postings := make([]*ledgerv1.Posting, len(tx.Postings))
	for i, p := range tx.Postings {
		postings[i] = &ledgerv1.Posting{
			AccountId: string(p.AccountID),
			Asset:     string(p.Asset),
			Amount:    int64(p.Amount),
		}
	}
	return &ledgerv1.Transaction{
		Id:             tx.ID,
		IdempotencyKey: tx.IdempotencyKey,
		Postings:       postings,
		CreatedAt:      timestamppb.New(tx.CreatedAt),
	}
}

func clampPageSize(size int) int {
	if size <= 0 {
		return defaultPageSize
	}
	if size > maxPageSize {
		return maxPageSize
	}
	return size
}

func encodePageToken(entryID int64) string {
	return base64.StdEncoding.EncodeToString([]byte(strconv.FormatInt(entryID, 10)))
}

func decodePageToken(token string) (int64, error) {
	if token == "" {
		return 0, nil
	}
	raw, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return 0, fmt.Errorf("base64 decode: %w", err)
	}
	cursor, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse cursor: %w", err)
	}
	return cursor, nil
}
