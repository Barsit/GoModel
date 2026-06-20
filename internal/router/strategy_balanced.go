package router

import (
	"context"
)

// BalancedStrategy scores candidates by a weighted combination of cost and
// latency. Lower total score wins; ties break by ProviderName. Candidates whose
// circuit is open or whose error rate is at/above MaxErrorRate are filtered out.
//
// Missing pricing scores 1.0 (worst) on cost; missing latency scores 1.0
// (worst) on latency — so providers with complete data are preferred. When
// every candidate misses the same dimension, that dimension contributes equally
// to all and ranking falls to the other dimension.
type BalancedStrategy struct {
	// CostWeight in [0, 1]. Defaults to 0.6 when zero.
	CostWeight float64
	// LatencyWeight in [0, 1]. Defaults to 0.4 when zero.
	LatencyWeight float64
	// MaxErrorRate filters candidates at/above this ratio. Defaults to 0.5.
	MaxErrorRate float64
}

// NewBalancedStrategy returns a balanced strategy with default weights.
func NewBalancedStrategy() *BalancedStrategy {
	return &BalancedStrategy{
		CostWeight:    0.6,
		LatencyWeight: 0.4,
		MaxErrorRate:  maxErrorRate,
	}
}

// Name returns "balanced".
func (s *BalancedStrategy) Name() string { return "balanced" }

// Select picks the lowest combined-score acceptable candidate.
func (s *BalancedStrategy) Select(_ context.Context, candidates []ProviderCandidate) (*ProviderCandidate, error) {
	costW := s.CostWeight
	if costW == 0 && s.LatencyWeight == 0 {
		costW = 0.6
	}
	latW := s.LatencyWeight
	if s.CostWeight == 0 && latW == 0 {
		latW = 0.4
	}
	maxErr := s.MaxErrorRate
	if maxErr == 0 {
		maxErr = maxErrorRate
	}

	costMin, costMax := minMaxCost(candidates)
	latMin, latMax := minMaxLatency(candidates)
	costScores := normalizeCost(costValues(candidates), costMin, costMax)
	latScores := normalizeLatency(latencyValues(candidates), latMin, latMax)

	scores := make([]float64, len(candidates))
	for i := range candidates {
		scores[i] = costW*costScores[i] + latW*latScores[i]
	}
	return rankBy(candidates, scores, maxErr)
}
