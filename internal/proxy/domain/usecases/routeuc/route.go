// Package routeuc declares the inbound port used by transports to route requests.
package routeuc

import (
	"context"

	"github.com/JLugagne/claudeproxy/internal/proxy/domain"
)

// RouteQueries resolves where a request must be sent. Implementations must be
// safe for concurrent use and cheap: they run on every proxied request.
type RouteQueries interface {
	// ResolveRoute returns the routing decision for model; the empty model
	// selects the fallback provider. It returns domain.ErrNoRoute.
	ResolveRoute(ctx context.Context, model domain.Model) (domain.Decision, error)
}
