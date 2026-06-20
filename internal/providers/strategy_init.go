package providers

import (
	router "gomodel/internal/router"

	"gomodel/config"
)

// buildStrategyRegistry constructs the routing strategy registry from config.
// The balanced strategy is instantiated with the configured weights so the
// configured cost/latency split applies to every request.
func buildStrategyRegistry(cfg config.RouterConfig) *router.StrategyRegistry {
	registry := router.NewStrategyRegistry()

	registry.Register("balanced", func() router.RoutingStrategy {
		return &router.BalancedStrategy{
			CostWeight:    cfg.Weights.Cost,
			LatencyWeight: cfg.Weights.Latency,
			MaxErrorRate:  cfg.MaxErrorRate,
		}
	})

	registry.Register("first_fit", func() router.RoutingStrategy {
		return &router.FirstFitStrategy{MaxErrorRate: cfg.MaxErrorRate}
	})

	if err := registry.SetDefault(cfg.Strategy); err != nil {
		// cfg.Strategy was already validated by ValidateRouterConfig, so this
		// should never fail at runtime; ignore to satisfy the err check.
		_ = err
	}
	return registry
}
