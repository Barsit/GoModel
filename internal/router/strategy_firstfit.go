package router

import (
	"context"
)

// FirstFitStrategy returns the first acceptable candidate without scoring,
// preserving the gateway's historical first-wins behaviour. It exists for
// operators who want deterministic, config-order routing.
type FirstFitStrategy struct {
	// MaxErrorRate filters candidates at/above this ratio. Defaults to 0.5.
	MaxErrorRate float64
}

// NewFirstFitStrategy returns a first-fit strategy with the default error filter.
func NewFirstFitStrategy() *FirstFitStrategy {
	return &FirstFitStrategy{MaxErrorRate: maxErrorRate}
}

// Name returns "first_fit".
func (s *FirstFitStrategy) Name() string { return "first_fit" }

// Select returns the first acceptable candidate in registration order.
func (s *FirstFitStrategy) Select(_ context.Context, candidates []ProviderCandidate) (*ProviderCandidate, error) {
	maxErr := s.MaxErrorRate
	if maxErr == 0 {
		maxErr = maxErrorRate
	}
	return firstAcceptable(candidates, maxErr)
}
