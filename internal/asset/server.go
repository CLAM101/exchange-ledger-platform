package asset

import (
	"context"
	"errors"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	assetv1 "github.com/CLAM101/exchange-ledger-platform/proto/gen/asset/v1"
)

// Server implements the AssetService gRPC interface.
type Server struct {
	assetv1.UnimplementedAssetServiceServer
	registry Registry
	logger   *zap.Logger
}

// NewServer creates a new AssetService gRPC server.
func NewServer(registry Registry, logger *zap.Logger) *Server {
	return &Server{registry: registry, logger: logger}
}

// GetAsset returns a single asset by symbol.
func (s *Server) GetAsset(_ context.Context, req *assetv1.GetAssetRequest) (*assetv1.GetAssetResponse, error) {
	if req.Symbol == "" {
		return nil, status.Error(codes.InvalidArgument, "symbol is required")
	}

	a, err := s.registry.Get(req.Symbol)
	if err != nil {
		return nil, domainToStatus(err)
	}

	return &assetv1.GetAssetResponse{
		Asset: assetToProto(a),
	}, nil
}

// ListAssets returns all registered assets.
func (s *Server) ListAssets(_ context.Context, _ *assetv1.ListAssetsRequest) (*assetv1.ListAssetsResponse, error) {
	assets := s.registry.List()
	out := make([]*assetv1.Asset, len(assets))
	for i, a := range assets {
		out[i] = assetToProto(a)
	}
	return &assetv1.ListAssetsResponse{Assets: out}, nil
}

// domainToStatus maps domain errors to gRPC status codes.
func domainToStatus(err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}

func assetToProto(a Asset) *assetv1.Asset {
	return &assetv1.Asset{
		Symbol:   a.Symbol,
		Decimals: a.Decimals,
		Active:   a.Active,
	}
}
