// Package app implements the proxy use cases on top of the domain router.
package app

import (
	"context"

	"github.com/JLugagne/claudeproxy/internal/proxy/domain"
	"github.com/JLugagne/claudeproxy/internal/proxy/domain/usecases/routeuc"
)

// App implements routeuc.RouteQueries. It is safe for concurrent use.
type App struct {
	router *domain.Router
}

var _ routeuc.RouteQueries = (*App)(nil)

func New(router *domain.Router) *App {
	return &App{router: router}
}

// ResolveRoute implements routeuc.RouteQueries.
func (a *App) ResolveRoute(_ context.Context, model domain.Model) (domain.Decision, error) {
	return a.router.Resolve(model)
}
