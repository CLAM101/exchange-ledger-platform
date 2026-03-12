// Package asset implements an in-memory asset registry for the exchange platform.
package asset

import "errors"

// Asset represents a tradeable asset with its configuration.
type Asset struct {
	Symbol   string
	Decimals int32
	Active   bool
}

// Registry provides read access to the asset catalogue.
type Registry interface {
	Get(symbol string) (Asset, error)
	List() []Asset
}

// ErrNotFound is returned when an asset symbol is not in the registry.
var ErrNotFound = errors.New("asset not found")

// InMemoryRegistry is a Registry backed by a static map.
type InMemoryRegistry struct {
	assets map[string]Asset
}

// NewInMemoryRegistry creates a registry pre-seeded with the given assets.
func NewInMemoryRegistry(seed []Asset) *InMemoryRegistry {
	m := make(map[string]Asset, len(seed))
	for _, a := range seed {
		m[a.Symbol] = a
	}
	return &InMemoryRegistry{assets: m}
}

// DefaultAssets returns the platform's built-in asset list.
func DefaultAssets() []Asset {
	return []Asset{
		{Symbol: "BTC", Decimals: 8, Active: true},
		{Symbol: "ETH", Decimals: 18, Active: true},
		{Symbol: "USDC", Decimals: 6, Active: true},
	}
}

// Get returns the asset for the given symbol or ErrNotFound.
func (r *InMemoryRegistry) Get(symbol string) (Asset, error) {
	a, ok := r.assets[symbol]
	if !ok {
		return Asset{}, ErrNotFound
	}
	return a, nil
}

// List returns all registered assets.
func (r *InMemoryRegistry) List() []Asset {
	out := make([]Asset, 0, len(r.assets))
	for _, a := range r.assets {
		out = append(out, a)
	}
	return out
}
