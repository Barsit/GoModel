package router

import (
	"context"
	"errors"
	"testing"
	"time"

	"gomodel/internal/core"
)

func floatPtr(v float64) *float64 { return &v }

func pricing(input, output float64) *core.ModelPricing {
	return &core.ModelPricing{InputPerMtok: floatPtr(input), OutputPerMtok: floatPtr(output)}
}

func candidates(cs ...ProviderCandidate) []ProviderCandidate { return cs }

func mustSelect(t *testing.T, s RoutingStrategy, cands []ProviderCandidate) *ProviderCandidate {
	t.Helper()
	got, err := s.Select(context.Background(), cands)
	if err != nil {
		t.Fatalf("%s.Select returned error: %v", s.Name(), err)
	}
	if got == nil {
		t.Fatalf("%s.Select returned nil candidate", s.Name())
	}
	return got
}

func TestBalanced_PicksCheapestWithEqualLatency(t *testing.T) {
	s := NewBalancedStrategy()
	cands := candidates(
		ProviderCandidate{ProviderName: "alpha", Pricing: pricing(5, 15), Latency: 100 * time.Millisecond},
		ProviderCandidate{ProviderName: "zeta", Pricing: pricing(1, 4), Latency: 100 * time.Millisecond},
	)
	got := mustSelect(t, s, cands)
	if got.ProviderName != "zeta" {
		t.Fatalf("expected zeta (cheaper), got %s", got.ProviderName)
	}
}

func TestBalanced_PicksFastestWithEqualCost(t *testing.T) {
	s := NewBalancedStrategy()
	cands := candidates(
		ProviderCandidate{ProviderName: "alpha", Pricing: pricing(5, 5), Latency: 200 * time.Millisecond},
		ProviderCandidate{ProviderName: "zeta", Pricing: pricing(5, 5), Latency: 50 * time.Millisecond},
	)
	got := mustSelect(t, s, cands)
	if got.ProviderName != "zeta" {
		t.Fatalf("expected zeta (faster), got %s", got.ProviderName)
	}
}

func TestBalanced_TiebreaksByName(t *testing.T) {
	s := NewBalancedStrategy()
	cands := candidates(
		ProviderCandidate{ProviderName: "zeta", Pricing: pricing(5, 5), Latency: 100 * time.Millisecond},
		ProviderCandidate{ProviderName: "alpha", Pricing: pricing(5, 5), Latency: 100 * time.Millisecond},
	)
	got := mustSelect(t, s, cands)
	if got.ProviderName != "alpha" {
		t.Fatalf("expected alpha (lexicographic tiebreak), got %s", got.ProviderName)
	}
}

func TestBalanced_FiltersOpenCircuit(t *testing.T) {
	s := NewBalancedStrategy()
	cands := candidates(
		ProviderCandidate{ProviderName: "open", Pricing: pricing(1, 1), Latency: 10 * time.Millisecond, CircuitState: "open"},
		ProviderCandidate{ProviderName: "healthy", Pricing: pricing(10, 10), Latency: 500 * time.Millisecond},
	)
	got := mustSelect(t, s, cands)
	if got.ProviderName != "healthy" {
		t.Fatalf("expected healthy (open circuit filtered), got %s", got.ProviderName)
	}
}

func TestBalanced_FiltersHighErrorRate(t *testing.T) {
	s := NewBalancedStrategy()
	cands := candidates(
		ProviderCandidate{ProviderName: "flaky", Pricing: pricing(1, 1), Latency: 10 * time.Millisecond, ErrorRate: 0.9},
		ProviderCandidate{ProviderName: "stable", Pricing: pricing(10, 10), Latency: 500 * time.Millisecond},
	)
	got := mustSelect(t, s, cands)
	if got.ProviderName != "stable" {
		t.Fatalf("expected stable (high error rate filtered), got %s", got.ProviderName)
	}
}

func TestBalanced_AllFilteredFallsBackWithError(t *testing.T) {
	s := NewBalancedStrategy()
	cands := candidates(
		ProviderCandidate{ProviderName: "open", Pricing: pricing(1, 1), CircuitState: "open"},
	)
	_, err := s.Select(context.Background(), cands)
	if !errors.Is(err, ErrNoAcceptableCandidate) {
		t.Fatalf("expected ErrNoAcceptableCandidate, got %v", err)
	}
}

func TestFirstFit_ReturnsFirstAcceptable(t *testing.T) {
	s := NewFirstFitStrategy()
	cands := candidates(
		ProviderCandidate{ProviderName: "open", Pricing: pricing(50, 50), Latency: 500 * time.Millisecond, CircuitState: "open"},
		ProviderCandidate{ProviderName: "second", Pricing: pricing(50, 50), Latency: 500 * time.Millisecond},
		ProviderCandidate{ProviderName: "third", Pricing: pricing(1, 1), Latency: 10 * time.Millisecond},
	)
	got := mustSelect(t, s, cands)
	if got.ProviderName != "second" {
		t.Fatalf("expected second (first acceptable), got %s", got.ProviderName)
	}
}

func TestBalanced_FreeModelNotTreatedAsUnknown(t *testing.T) {
	s := NewBalancedStrategy()
	// freeModel has pricing = $0 (free). unknown has no pricing.
	freeModel := ProviderCandidate{ProviderName: "free", Pricing: pricing(0, 0), Latency: 100 * time.Millisecond}
	unknown := ProviderCandidate{ProviderName: "unknown", Latency: 100 * time.Millisecond}
	got := mustSelect(t, s, candidates(freeModel, unknown))
	if got.ProviderName != "free" {
		t.Fatalf("expected free (known zero cost, score 0), got %s", got.ProviderName)
	}
}

func TestBalanced_ZeroValuesDefaultToSixtyForty(t *testing.T) {
	s := &BalancedStrategy{} // zero-value struct, should default to 0.6 cost / 0.4 latency
	cands := candidates(
		ProviderCandidate{ProviderName: "alpha", Pricing: pricing(5, 15), Latency: 100 * time.Millisecond},
		ProviderCandidate{ProviderName: "zeta", Pricing: pricing(1, 4), Latency: 100 * time.Millisecond},
	)
	got := mustSelect(t, s, cands)
	if got.ProviderName != "zeta" {
		t.Fatalf("expected zeta (cheaper, cost weight dominates at 0.6/0.4), got %s", got.ProviderName)
	}
}

func TestBalanced_EqualCostKnownVsUnknown(t *testing.T) {
	s := NewBalancedStrategy()
	// same latency, both $5 — but "unknown" has no pricing at all, so it gets
	// cost score 1.0 while the known-cost provider gets cost score 0.
	known := ProviderCandidate{ProviderName: "known", Pricing: pricing(5, 5), Latency: 100 * time.Millisecond}
	unknown := ProviderCandidate{ProviderName: "unknown", Latency: 100 * time.Millisecond}
	got := mustSelect(t, s, candidates(known, unknown))
	if got.ProviderName != "known" {
		t.Fatalf("expected known (complete data preferred over unknown), got %s", got.ProviderName)
	}
}

func TestStrategies_HandleEmpty(t *testing.T) {
	strategies := []RoutingStrategy{
		NewBalancedStrategy(),
		NewFirstFitStrategy(),
	}
	for _, s := range strategies {
		_, err := s.Select(context.Background(), nil)
		if !errors.Is(err, ErrNoAcceptableCandidate) {
			t.Errorf("%s on nil candidates: expected ErrNoAcceptableCandidate, got %v", s.Name(), err)
		}
	}
}

// --- Boundary tests ---

func TestBalanced_ErrorRateAtBoundary(t *testing.T) {
	s := NewBalancedStrategy()
	// ErrorRate == 0.5 is at the default filter boundary — candidate is filtered.
	atBoundary := ProviderCandidate{ProviderName: "at-boundary", Pricing: pricing(1, 1), Latency: 10 * time.Millisecond, ErrorRate: 0.5}
	healthy := ProviderCandidate{ProviderName: "healthy", Pricing: pricing(10, 10), Latency: 500 * time.Millisecond}
	got := mustSelect(t, s, candidates(atBoundary, healthy))
	if got.ProviderName != "healthy" {
		t.Fatalf("expected healthy (at-boundary filtered), got %s", got.ProviderName)
	}
}

func TestBalanced_ErrorRateBelowBoundary(t *testing.T) {
	s := NewBalancedStrategy()
	below := ProviderCandidate{ProviderName: "below", Pricing: pricing(1, 1), Latency: 10 * time.Millisecond, ErrorRate: 0.499}
	pricey := ProviderCandidate{ProviderName: "pricey", Pricing: pricing(10, 10), Latency: 500 * time.Millisecond}
	got := mustSelect(t, s, candidates(below, pricey))
	if got.ProviderName != "below" {
		t.Fatalf("expected below (0.499 < 0.5 is acceptable), got %s", got.ProviderName)
	}
}

func TestBalanced_EqualScoresTiebreak(t *testing.T) {
	s := NewBalancedStrategy()
	cands := candidates(
		ProviderCandidate{ProviderName: "zzz", Latency: 100 * time.Millisecond},
		ProviderCandidate{ProviderName: "aaa", Latency: 100 * time.Millisecond},
	)
	got := mustSelect(t, s, cands)
	if got.ProviderName != "aaa" {
		t.Fatalf("expected aaa (tiebreak), got %s", got.ProviderName)
	}
}

func TestFirstFit_SingleCandidate(t *testing.T) {
	s := NewFirstFitStrategy()
	cands := candidates(
		ProviderCandidate{ProviderName: "solo", Pricing: pricing(1, 1)},
	)
	got := mustSelect(t, s, cands)
	if got.ProviderName != "solo" {
		t.Fatalf("expected solo, got %s", got.ProviderName)
	}
}

func TestFirstFit_AllFilteredReturnsError(t *testing.T) {
	s := NewFirstFitStrategy()
	cands := candidates(
		ProviderCandidate{ProviderName: "open", CircuitState: "open"},
		ProviderCandidate{ProviderName: "half", CircuitState: "half-open"},
	)
	_, err := s.Select(context.Background(), cands)
	if !errors.Is(err, ErrNoAcceptableCandidate) {
		t.Fatalf("expected ErrNoAcceptableCandidate, got %v", err)
	}
}
