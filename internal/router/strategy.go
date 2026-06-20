// Package router provides pluggable routing strategies that select the best
// provider from a set of candidates serving the same model ID. Strategies are
// cost-aware and/or latency-aware, fed by provider runtime statistics.
package router

import (
	"context"
	"time"

	"gomodel/internal/core"
)

// ProviderCandidate describes one provider able to serve a given model, together
// with the signals a strategy needs to score it. Fields with zero values
// (nil Pricing, 0 Latency, "" CircuitState) mean "unknown"; strategies treat
// unknown signals as worst-case so that providers with complete data are preferred.
type ProviderCandidate struct {
	// Provider is the underlying provider implementation (may be nil for
	// strategy-only callers that just need the name/metadata).
	Provider core.Provider

	// ProviderName is the concrete configured instance name, e.g.
	// "openai-primary". Used as a deterministic tiebreaker.
	ProviderName string

	// ProviderType is the provider type, e.g. "openai" or "azure".
	ProviderType string

	// ModelID is the model identifier this candidate serves.
	ModelID string

	// Pricing is the per-model pricing used by cost-aware strategies.
	// nil means pricing is unknown.
	Pricing *core.ModelPricing

	// Latency is the provider's smoothed P50 latency. 0 means unknown.
	Latency time.Duration

	// CircuitState is "closed", "open", or "half-open". "" defaults to
	// "closed" (healthy) when a provider has no breaker.
	CircuitState string

	// ErrorRate is the smoothed error ratio in [0, 1]. 0 means unknown
	// (which strategies may treat conservatively or optimistically per policy).
	ErrorRate float64
}

// RoutingStrategy selects the best provider candidate from a non-empty list.
// Implementations must be deterministic: given identical candidates they must
// return the same choice, breaking ties by ProviderName lexicographic order.
//
// Select returns an error only when no candidate is acceptable (e.g. every
// candidate is filtered out); callers fall back to the first candidate in
// that case rather than failing the request.
type RoutingStrategy interface {
	// Name returns the strategy identifier, e.g. "balanced", "first_fit".
	Name() string

	// Select picks one candidate. The candidates slice is non-empty; strategies
	// that filter out every candidate return an error so the caller can fall
	// back to the first candidate and log a warning.
	Select(ctx context.Context, candidates []ProviderCandidate) (*ProviderCandidate, error)
}
