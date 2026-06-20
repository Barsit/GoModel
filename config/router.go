package config

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// RouterConfig configures intelligent provider selection. When a request does
// not pin a provider and multiple providers serve the same model, the gateway
// scores candidates by cost and/or latency using the configured strategy.
type RouterConfig struct {
	// Strategy is the default strategy id: "balanced" or "first_fit".
	// Empty defaults to "balanced".
	Strategy string `yaml:"strategy" json:"strategy" env:"MODEL_ROUTING_STRATEGY"`

	// Weights tunes the balanced strategy. CostWeight and LatencyWeight are
	// ignored by other strategies.
	Weights RouterWeights `yaml:"weights" json:"weights"`

	// WeightsCSV is the env-only form of Weights as "cost,latency"
	// (e.g. "0.6,0.4"). Parsed into Weights during validation.
	WeightsCSV string `yaml:"-" json:"-" env:"MODEL_ROUTING_STRATEGY_WEIGHTS"`

	// MaxErrorRate filters candidates at/above this smoothed error ratio before
	// scoring. Zero falls back to the strategy default (0.5).
	MaxErrorRate float64 `yaml:"max_error_rate" json:"max_error_rate" env:"MODEL_ROUTING_MAX_ERROR_RATE"`
}

// RouterWeights tunes the balanced strategy's cost/latency trade-off.
type RouterWeights struct {
	Cost    float64 `yaml:"cost" json:"cost" env:"MODEL_ROUTING_COST_WEIGHT"`
	Latency float64 `yaml:"latency" json:"latency" env:"MODEL_ROUTING_LATENCY_WEIGHT"`
}

// RouterStrategyBalanced and the other built-in strategy ids.
const (
	RouterStrategyBalanced = "balanced"
	RouterStrategyFirstFit = "first_fit"
)

// DefaultRouterConfig returns the default router configuration: balanced strategy
// with 0.6 cost / 0.4 latency weights and a 0.5 max error rate.
func DefaultRouterConfig() RouterConfig {
	return RouterConfig{
		Strategy: RouterStrategyBalanced,
		Weights: RouterWeights{
			Cost:    0.6,
			Latency: 0.4,
		},
		MaxErrorRate: 0.5,
	}
}

// ValidateRouterConfig normalizes and validates the router config, applying
// defaults for empty/invalid fields.
func ValidateRouterConfig(cfg *RouterConfig) error {
	if cfg.Strategy == "" {
		cfg.Strategy = RouterStrategyBalanced
	}
	strategy := strings.ToLower(strings.TrimSpace(cfg.Strategy))
	switch strategy {
	case RouterStrategyBalanced, RouterStrategyFirstFit:
	default:
		return fmt.Errorf("invalid router.strategy %q: must be one of balanced, first_fit", cfg.Strategy)
	}
	cfg.Strategy = strategy

	// MODEL_ROUTING_STRATEGY_WEIGHTS overrides YAML weights when set.
	if strings.TrimSpace(cfg.WeightsCSV) != "" {
		cost, lat, err := parseWeightsCSV(cfg.WeightsCSV)
		if err != nil {
			return fmt.Errorf("invalid MODEL_ROUTING_STRATEGY_WEIGHTS: %w", err)
		}
		cfg.Weights = RouterWeights{Cost: cost, Latency: lat}
	}

	if math.IsNaN(cfg.Weights.Cost) || math.IsNaN(cfg.Weights.Latency) ||
		math.IsInf(cfg.Weights.Cost, 0) || math.IsInf(cfg.Weights.Latency, 0) {
		return fmt.Errorf("router.weights must be finite numbers, got cost=%v latency=%v", cfg.Weights.Cost, cfg.Weights.Latency)
	}
	if cfg.Weights.Cost < 0 || cfg.Weights.Latency < 0 {
		return fmt.Errorf("router.weights must be non-negative, got cost=%v latency=%v", cfg.Weights.Cost, cfg.Weights.Latency)
	}
	if cfg.Weights.Cost == 0 && cfg.Weights.Latency == 0 && strategy == RouterStrategyBalanced {
		cfg.Weights = RouterWeights{Cost: 0.6, Latency: 0.4}
	}
	if math.IsNaN(cfg.MaxErrorRate) || math.IsInf(cfg.MaxErrorRate, 0) {
		return fmt.Errorf("router.max_error_rate must be a finite number, got %v", cfg.MaxErrorRate)
	}
	if cfg.MaxErrorRate < 0 || cfg.MaxErrorRate > 1 {
		return fmt.Errorf("router.max_error_rate must be in [0, 1], got %v", cfg.MaxErrorRate)
	}
	return nil
}

// parseWeightsCSV parses a "cost,latency" string into two non-negative floats.
func parseWeightsCSV(s string) (cost, latency float64, err error) {
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected two comma-separated weights, got %q", s)
	}
	if cost, err = parseFloatField(parts[0], "cost"); err != nil {
		return 0, 0, err
	}
	if latency, err = parseFloatField(parts[1], "latency"); err != nil {
		return 0, 0, err
	}
	if cost < 0 || latency < 0 {
		return 0, 0, fmt.Errorf("weights must be non-negative, got %v,%v", cost, latency)
	}
	return cost, latency, nil
}

func parseFloatField(s, name string) (float64, error) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s weight %q: %w", name, s, err)
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("invalid %s weight %q: must be a finite number", name, s)
	}
	return v, nil
}
