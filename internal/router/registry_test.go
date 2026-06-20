package router

import (
	"context"
	"testing"
)

// WeightsOverride and ParseWeights were removed in the strategy simplification;
// their test coverage is in config/router.go via parseWeightsCSV.

func TestStrategyRegistry_DefaultIsBalanced(t *testing.T) {
	r := NewStrategyRegistry()
	s, ok := r.Resolve(context.Background())
	if !ok {
		t.Fatal("expected default strategy to resolve")
	}
	if s.Name() != "balanced" {
		t.Fatalf("expected default balanced, got %s", s.Name())
	}
}

func TestStrategyRegistry_OverrideHonored(t *testing.T) {
	r := NewStrategyRegistry()
	ctx := WithStrategyOverride(context.Background(), "first_fit")
	s, ok := r.Resolve(ctx)
	if !ok {
		t.Fatal("expected override to resolve")
	}
	if s.Name() != "first_fit" {
		t.Fatalf("expected first_fit, got %s", s.Name())
	}
}

func TestStrategyRegistry_InvalidOverrideRejected(t *testing.T) {
	r := NewStrategyRegistry()
	ctx := WithStrategyOverride(context.Background(), "nonsense")
	s, ok := r.Resolve(ctx)
	if ok {
		t.Fatal("expected invalid override to be rejected")
	}
	if s != nil {
		t.Fatal("expected nil strategy for invalid override")
	}
}

func TestStrategyRegistry_Names(t *testing.T) {
	r := NewStrategyRegistry()
	names := r.Names()
	if len(names) != 2 {
		t.Fatalf("expected 2 strategies, got %d (%v)", len(names), names)
	}
}

func TestStrategyRegistry_SetDefaultInvalid(t *testing.T) {
	r := NewStrategyRegistry()
	if err := r.SetDefault("bogus"); err == nil {
		t.Fatal("expected error setting unknown default")
	}
}
