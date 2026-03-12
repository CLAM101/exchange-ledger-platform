package asset_test

import (
	"errors"
	"testing"

	"github.com/CLAM101/exchange-ledger-platform/internal/asset"
)

func TestInMemoryRegistry_Get_Known(t *testing.T) {
	t.Parallel()
	reg := asset.NewInMemoryRegistry(asset.DefaultAssets())

	a, err := reg.Get("BTC")
	if err != nil {
		t.Fatalf("Get(BTC): %v", err)
	}
	if a.Symbol != "BTC" {
		t.Errorf("symbol = %q, want %q", a.Symbol, "BTC")
	}
	if a.Decimals != 8 {
		t.Errorf("decimals = %d, want %d", a.Decimals, 8)
	}
	if !a.Active {
		t.Error("expected active = true")
	}
}

func TestInMemoryRegistry_Get_Unknown(t *testing.T) {
	t.Parallel()
	reg := asset.NewInMemoryRegistry(asset.DefaultAssets())

	_, err := reg.Get("DOGE")
	if !errors.Is(err, asset.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestInMemoryRegistry_List(t *testing.T) {
	t.Parallel()
	reg := asset.NewInMemoryRegistry(asset.DefaultAssets())

	assets := reg.List()
	if len(assets) != 3 {
		t.Fatalf("len = %d, want 3", len(assets))
	}

	symbols := make(map[string]bool)
	for _, a := range assets {
		symbols[a.Symbol] = true
	}
	for _, want := range []string{"BTC", "ETH", "USDC"} {
		if !symbols[want] {
			t.Errorf("missing asset %q", want)
		}
	}
}
