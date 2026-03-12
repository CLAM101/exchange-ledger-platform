package asset_test

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/CLAM101/exchange-ledger-platform/internal/asset"
	assetv1 "github.com/CLAM101/exchange-ledger-platform/proto/gen/asset/v1"
)

// mockRegistry is a test double for asset.Registry.
type mockRegistry struct {
	getFn  func(symbol string) (asset.Asset, error)
	listFn func() []asset.Asset
}

func (m *mockRegistry) Get(symbol string) (asset.Asset, error) {
	return m.getFn(symbol)
}

func (m *mockRegistry) List() []asset.Asset {
	return m.listFn()
}

func newTestServer(reg asset.Registry) *asset.Server {
	return asset.NewServer(reg, zap.NewNop())
}

// --- GetAsset tests ---

func TestGetAsset_MissingSymbol(t *testing.T) {
	t.Parallel()
	srv := newTestServer(&mockRegistry{})

	_, err := srv.GetAsset(context.Background(), &assetv1.GetAssetRequest{})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", st.Code())
	}
}

func TestGetAsset_NotFound(t *testing.T) {
	t.Parallel()
	reg := &mockRegistry{
		getFn: func(_ string) (asset.Asset, error) {
			return asset.Asset{}, asset.ErrNotFound
		},
	}
	srv := newTestServer(reg)

	_, err := srv.GetAsset(context.Background(), &assetv1.GetAssetRequest{Symbol: "DOGE"})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.NotFound {
		t.Errorf("code = %v, want NotFound", st.Code())
	}
}

func TestGetAsset_Success(t *testing.T) {
	t.Parallel()
	reg := &mockRegistry{
		getFn: func(symbol string) (asset.Asset, error) {
			return asset.Asset{Symbol: symbol, Decimals: 8, Active: true}, nil
		},
	}
	srv := newTestServer(reg)

	resp, err := srv.GetAsset(context.Background(), &assetv1.GetAssetRequest{Symbol: "BTC"})
	if err != nil {
		t.Fatalf("GetAsset: %v", err)
	}
	if resp.Asset.Symbol != "BTC" {
		t.Errorf("symbol = %q, want %q", resp.Asset.Symbol, "BTC")
	}
	if resp.Asset.Decimals != 8 {
		t.Errorf("decimals = %d, want %d", resp.Asset.Decimals, 8)
	}
	if !resp.Asset.Active {
		t.Error("expected active = true")
	}
}

// --- ListAssets tests ---

func TestListAssets_Success(t *testing.T) {
	t.Parallel()
	reg := &mockRegistry{
		listFn: func() []asset.Asset {
			return []asset.Asset{
				{Symbol: "BTC", Decimals: 8, Active: true},
				{Symbol: "ETH", Decimals: 18, Active: true},
			}
		},
	}
	srv := newTestServer(reg)

	resp, err := srv.ListAssets(context.Background(), &assetv1.ListAssetsRequest{})
	if err != nil {
		t.Fatalf("ListAssets: %v", err)
	}
	if len(resp.Assets) != 2 {
		t.Fatalf("len = %d, want 2", len(resp.Assets))
	}
	if resp.Assets[0].Symbol != "BTC" {
		t.Errorf("assets[0].symbol = %q, want %q", resp.Assets[0].Symbol, "BTC")
	}
}
