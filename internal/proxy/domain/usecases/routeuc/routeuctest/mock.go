// Package routeuctest provides test doubles for the routeuc ports.
package routeuctest

import (
	"context"

	"github.com/JLugagne/claudeproxy/internal/proxy/domain"
	"github.com/JLugagne/claudeproxy/internal/proxy/domain/usecases/routeuc"
)

// MockRouteQueries is a function-based mock of routeuc.RouteQueries. An unset
// func panics so unexpected calls fail loudly.
type MockRouteQueries struct {
	ResolveRouteFn func(ctx context.Context, model domain.Model) (domain.Decision, error)
}

var _ routeuc.RouteQueries = (*MockRouteQueries)(nil)

func (m *MockRouteQueries) ResolveRoute(ctx context.Context, model domain.Model) (domain.Decision, error) {
	if m.ResolveRouteFn == nil {
		panic("MockRouteQueries.ResolveRouteFn not set")
	}
	return m.ResolveRouteFn(ctx, model)
}
