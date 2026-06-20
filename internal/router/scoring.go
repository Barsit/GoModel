package router

import (
	"errors"
	"sort"
	"time"
)

// ErrNoAcceptableCandidate indicates a strategy filtered out every candidate.
// Callers fall back to the first candidate rather than failing the request.
var ErrNoAcceptableCandidate = errors.New("no acceptable routing candidate")

// candidateScore pairs a candidate with its computed score for sorting.
type candidateScore struct {
	candidate *ProviderCandidate
	score     float64
}

// maxErrorRate is the default upper bound above which a candidate is filtered.
const maxErrorRate = 0.5

// isCircuitOpen reports whether a candidate's breaker blocks requests.
// An empty CircuitState is treated as healthy ("closed").
func isCircuitOpen(c *ProviderCandidate) bool {
	return c.CircuitState == "open" || c.CircuitState == "half-open"
}

// acceptable returns false when a candidate should be filtered out before
// scoring: circuit open or error rate at/above the threshold. Candidates with
// unknown error rate (0) are kept.
func acceptable(c *ProviderCandidate, maxErrRate float64) bool {
	if isCircuitOpen(c) {
		return false
	}
	if c.ErrorRate >= maxErrRate {
		return false
	}
	return true
}

// normalizeLatency maps latency values into [0, 1] relative to min and max.
// Lower latency → lower (better) score. Unknown latencies (0) are scored 1.0
// (worst) so complete-data providers win. When all latencies are unknown or
// equal, every value scores 0.
func normalizeLatency(values []time.Duration, min, max time.Duration) []float64 {
	scores := make([]float64, len(values))
	if max <= min || max == 0 {
		for i := range values {
			if values[i] == 0 {
				scores[i] = 1.0
			}
		}
		return scores
	}
	span := float64(max - min)
	for i, v := range values {
		if v == 0 {
			scores[i] = 1.0
			continue
		}
		scores[i] = float64(v-min) / span
	}
	return scores
}

// normalizeCost maps per-million-token costs into [0, 1] relative to min and
// max. Lower cost → lower (better) score. Missing pricing (perMtokCost returns
// -1) is scored 1.0 so complete-data providers win. Known zero-cost (free
// models) is scored 0.0 (best), properly distinguished from unknown.
// When all costs are unknown or equal, every value scores 0.
func normalizeCost(values []float64, min, max float64) []float64 {
	scores := make([]float64, len(values))
	if max <= min {
		for i, v := range values {
			if v < 0 {
				scores[i] = 1.0
			}
		}
		return scores
	}
	span := max - min
	for i, v := range values {
		if v < 0 {
			scores[i] = 1.0
			continue
		}
		scores[i] = (v - min) / span
	}
	return scores
}

// pickBest returns the candidate with the lowest score, breaking ties by
// ProviderName lexicographic order for determinism. Returns
// ErrNoAcceptableCandidate when scored is empty.
func pickBest(scored []candidateScore) (*ProviderCandidate, error) {
	if len(scored) == 0 {
		return nil, ErrNoAcceptableCandidate
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score < scored[j].score
		}
		return scored[i].candidate.ProviderName < scored[j].candidate.ProviderName
	})
	return scored[0].candidate, nil
}

// firstAcceptable returns the first acceptable candidate in order, or
// ErrNoAcceptableCandidate when none pass the filter.
func firstAcceptable(candidates []ProviderCandidate, maxErrRate float64) (*ProviderCandidate, error) {
	for i := range candidates {
		if acceptable(&candidates[i], maxErrRate) {
			return &candidates[i], nil
		}
	}
	return nil, ErrNoAcceptableCandidate
}

// minMaxLatency returns the min and max non-zero latency across candidates.
func minMaxLatency(candidates []ProviderCandidate) (min, max time.Duration) {
	for i := range candidates {
		v := candidates[i].Latency
		if v <= 0 {
			continue
		}
		if min == 0 || v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return min, max
}

// minMaxCost returns the min and max per-million-token cost across candidates.
// Unknown costs (perMtokCost returns -1) are skipped. Known-zero costs
// (free models) are included.
func minMaxCost(candidates []ProviderCandidate) (min, max float64) {
	var seen bool
	for i := range candidates {
		cost := perMtokCost(&candidates[i])
		if cost < 0 {
			continue
		}
		if !seen {
			min = cost
			max = cost
			seen = true
			continue
		}
		if cost < min {
			min = cost
		}
		if cost > max {
			max = cost
		}
	}
	return min, max
}

// perMtokCost returns the sum of input and output per-million-token prices for
// a candidate. Returns -1 when pricing is absent (so callers can distinguish
// free models at cost 0 from unknown pricing).
func perMtokCost(c *ProviderCandidate) float64 {
	if c == nil || c.Pricing == nil {
		return -1
	}
	// When both per-token pricing fields are nil, treat the candidate as
	// having unknown pricing (not free) so the cost score defaults to 1.0
	// (worst). A known-free model has InputPerMtok=0 and OutputPerMtok=0.
	if c.Pricing.InputPerMtok == nil && c.Pricing.OutputPerMtok == nil {
		return -1
	}
	var cost float64
	if c.Pricing.InputPerMtok != nil {
		cost += *c.Pricing.InputPerMtok
	}
	if c.Pricing.OutputPerMtok != nil {
		cost += *c.Pricing.OutputPerMtok
	}
	return cost
}

// latencyValues collects the latency field from each candidate preserving order.
func latencyValues(candidates []ProviderCandidate) []time.Duration {
	out := make([]time.Duration, len(candidates))
	for i := range candidates {
		out[i] = candidates[i].Latency
	}
	return out
}

// costValues collects the per-million-token cost from each candidate preserving order.
func costValues(candidates []ProviderCandidate) []float64 {
	out := make([]float64, len(candidates))
	for i := range candidates {
		out[i] = perMtokCost(&candidates[i])
	}
	return out
}

// rankBy filters candidates and ranks them by a precomputed per-candidate
// score, returning the best (lowest-scoring) one. It is the shared body of the
// single-factor strategies; callers compute the score array (e.g. normalized
// cost or latency) and pass it here.
func rankBy(candidates []ProviderCandidate, scores []float64, maxErrRate float64) (*ProviderCandidate, error) {
	scored := make([]candidateScore, 0, len(candidates))
	for i := range candidates {
		if !acceptable(&candidates[i], maxErrRate) {
			continue
		}
		scored = append(scored, candidateScore{
			candidate: &candidates[i],
			score:     scores[i],
		})
	}
	return pickBest(scored)
}
