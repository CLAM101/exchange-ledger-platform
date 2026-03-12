package asset_test

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

	"github.com/CLAM101/exchange-ledger-platform/internal/asset"
	"github.com/CLAM101/exchange-ledger-platform/internal/platform/observability"
	assetv1 "github.com/CLAM101/exchange-ledger-platform/proto/gen/asset/v1"

	platformgrpc "github.com/CLAM101/exchange-ledger-platform/internal/platform/grpc"
)

const bufSize = 1024 * 1024

// setupGRPC creates an in-memory gRPC server+client pair with the given registry.
func setupGRPC(t *testing.T, reg asset.Registry) assetv1.AssetServiceClient {
	t.Helper()

	logger := zap.NewNop()
	metrics := observability.NewTestMetrics()
	hs := health.NewServer()
	hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	grpcServer := platformgrpc.NewServer(logger, metrics, hs)
	handler := asset.NewServer(reg, logger)
	assetv1.RegisterAssetServiceServer(grpcServer, handler)

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

	return assetv1.NewAssetServiceClient(conn)
}

func TestGRPC_GetAsset_Success(t *testing.T) {
	t.Parallel()
	reg := asset.NewInMemoryRegistry(asset.DefaultAssets())
	client := setupGRPC(t, reg)

	resp, err := client.GetAsset(context.Background(), &assetv1.GetAssetRequest{Symbol: "BTC"})
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

func TestGRPC_GetAsset_NotFound(t *testing.T) {
	t.Parallel()
	reg := asset.NewInMemoryRegistry(asset.DefaultAssets())
	client := setupGRPC(t, reg)

	_, err := client.GetAsset(context.Background(), &assetv1.GetAssetRequest{Symbol: "DOGE"})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.NotFound {
		t.Errorf("code = %v, want NotFound", st.Code())
	}
}

func TestGRPC_ListAssets_ReturnsAll(t *testing.T) {
	t.Parallel()
	reg := asset.NewInMemoryRegistry(asset.DefaultAssets())
	client := setupGRPC(t, reg)

	resp, err := client.ListAssets(context.Background(), &assetv1.ListAssetsRequest{})
	if err != nil {
		t.Fatalf("ListAssets: %v", err)
	}
	if len(resp.Assets) != 3 {
		t.Fatalf("len = %d, want 3", len(resp.Assets))
	}

	symbols := make(map[string]bool)
	for _, a := range resp.Assets {
		symbols[a.Symbol] = true
	}
	for _, want := range []string{"BTC", "ETH", "USDC"} {
		if !symbols[want] {
			t.Errorf("missing asset %q", want)
		}
	}
}
