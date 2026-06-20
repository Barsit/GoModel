package router

import (
	"context"
	"fmt"
	"strings"
)

// StrategyRegistry maps strategy names to factories. A factory returns a fresh
// strategy instance (so per-request weights can be applied without mutation
// across requests). The default strategy is used when a request omits an
// override or names an unknown strategy.
type StrategyRegistry struct {
	factories map[string]func() RoutingStrategy
	defaultID string
}

// NewStrategyRegistry returns a registry pre-populated with the built-in
// strategies (balanced, first_fit) and "balanced" as the default.
func NewStrategyRegistry() *StrategyRegistry {
	r := &StrategyRegistry{
		factories: map[string]func() RoutingStrategy{},
		defaultID: "balanced",
	}
	r.Register("balanced", func() RoutingStrategy { return NewBalancedStrategy() })
	r.Register("first_fit", func() RoutingStrategy { return NewFirstFitStrategy() })
	return r
}

// Register adds or replaces a strategy factory under the given name.
func (r *StrategyRegistry) Register(name string, factory func() RoutingStrategy) {
	if factory == nil || name == "" {
		return
	}
	r.factories[strings.ToLower(strings.TrimSpace(name))] = factory
}

// SetDefault sets the default strategy id used when no/invalid override is given.
// Returns an error if the id is not registered.
func (r *StrategyRegistry) SetDefault(id string) error {
	if _, ok := r.factories[strings.ToLower(strings.TrimSpace(id))]; !ok {
		return fmt.Errorf("unknown routing strategy %q", id)
	}
	r.defaultID = strings.ToLower(strings.TrimSpace(id))
	return nil
}

// DefaultID returns the configured default strategy id.
func (r *StrategyRegistry) DefaultID() string { return r.defaultID }

// Names returns the registered strategy ids.
func (r *StrategyRegistry) Names() []string {
	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	return names
}

// Resolve returns the strategy for a request. If ctx carries a valid override,
// that strategy is used; otherwise the default is used. An invalid override id
// yields (nil, false) so the caller can fall back to the default and log a warning.
func (r *StrategyRegistry) Resolve(ctx context.Context) (RoutingStrategy, bool) {
	if override, ok := StrategyOverrideFromContext(ctx); ok {
		name := strings.ToLower(strings.TrimSpace(override))
		if factory, ok := r.factories[name]; ok {
			return factory(), true
		}
		return nil, false
	}
	if factory, ok := r.factories[r.defaultID]; ok {
		return factory(), true
	}
	return nil, false
}

// New returns a fresh instance of the named strategy, or nil if unknown.
func (r *StrategyRegistry) New(name string) RoutingStrategy {
	factory, ok := r.factories[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return nil
	}
	return factory()
}
