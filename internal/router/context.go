package router

import "context"

// strategyCtxKey carries a per-request routing-strategy override extracted
// from the X-GoModel-Routing-Strategy header. It is distinct from any other
// context key so callers cannot collide with it.
type strategyCtxKey struct{}


// RoutingOperation is a typed operation name that gates intelligent routing.
// Only chat and responses endpoints are eligible; audio, realtime,
// embeddings, and other capability-gated endpoints must never participate.
type RoutingOperation string

const (
	RoutingOpChat      RoutingOperation = "chat"
	RoutingOpResponses RoutingOperation = "responses"
)

// routingOpKey carries the endpoint operation that granted route eligibility.
// resolveProvider checks this to reject routing on non-chat/responses paths.
type routingOpKey struct{}

// WithRoutingEligible marks a context as eligible for intelligent routing
// despite having a non-empty provider hint. operation must be one of the
// allowed RoutingOperation values (chat, responses); passing any other value
// is silently ignored so that non-chat/responses code paths cannot
// accidentally enable routing.
func WithRoutingEligible(ctx context.Context, operation RoutingOperation) context.Context {
	switch operation {
	case RoutingOpChat, RoutingOpResponses:
		return context.WithValue(ctx, routingOpKey{}, operation)
	default:
		return ctx
	}
}

// IsRoutingEligible reports whether the context was marked as eligible by a
// recognised operation (chat or responses).
func IsRoutingEligible(ctx context.Context) bool {
	_, ok := ctx.Value(routingOpKey{}).(RoutingOperation)
	return ok
}

// StrategyOverrideFromContext returns a per-request strategy name override set
// on the context, and whether one was present.
func StrategyOverrideFromContext(ctx context.Context) (string, bool) {
	s, ok := ctx.Value(strategyCtxKey{}).(string)
	return s, ok
}

// WithStrategyOverride returns a context carrying a per-request strategy name
// override. An empty name clears any prior override.
func WithStrategyOverride(ctx context.Context, strategy string) context.Context {
	if strategy == "" {
		return context.WithValue(ctx, strategyCtxKey{}, nil)
	}
	return context.WithValue(ctx, strategyCtxKey{}, strategy)
}
