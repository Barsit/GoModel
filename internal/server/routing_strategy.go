package server

import (
	"strings"

	router "gomodel/internal/router"

	"github.com/labstack/echo/v5"
)

// RoutingStrategyHeader carries the per-request routing-strategy override.
const RoutingStrategyHeader = "X-GoModel-Routing-Strategy"

// RoutingStrategyCapture reads the optional X-GoModel-Routing-Strategy header
// and attaches it to the request context as an override for the router.
// Invalid values are silently dropped by the registry (the router falls back to
// the configured global strategy). The middleware is a no-op when the header is
// absent, so non-routing requests pay only a header-lookup cost.
func RoutingStrategyCapture() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			ctx := c.Request().Context()

			if strategy := strings.TrimSpace(c.Request().Header.Get(RoutingStrategyHeader)); strategy != "" {
				ctx = router.WithStrategyOverride(ctx, strings.ToLower(strategy))
			}

			if ctx != c.Request().Context() {
				c.SetRequest(c.Request().WithContext(ctx))
			}
			return next(c)
		}
	}
}
