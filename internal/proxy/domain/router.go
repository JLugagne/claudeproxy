package domain

import "sync"

// Router picks the provider for a model: routes are tried in order and the
// first match wins; unmatched or model-less requests go to the fallback.
// A Router is immutable and safe for concurrent use.
type Router struct {
	routes   []Route
	fallback ProviderName
	// cache memoizes Resolve by model name. Routes are immutable, so an outcome never changes and needs no invalidation.
	cache sync.Map
}

// NewRouter builds a router. An empty fallback makes unmatched requests fail
// with ErrNoRoute instead of being forwarded.
func NewRouter(routes []Route, fallback ProviderName) *Router {
	return &Router{routes: routes, fallback: fallback}
}

// Resolve returns where to send a request for model. It returns ErrNoRoute.
// Results are memoized: routes are immutable, so a given model always resolves
// the same way and the ordered matching runs once per distinct model.
func (r *Router) Resolve(model Model) (Decision, error) {
	if cached, ok := r.cache.Load(model); ok {
		resolved := cached.(resolution)
		return resolved.decision, resolved.err
	}
	decision, err := r.resolve(model)
	r.cache.Store(model, resolution{decision: decision, err: err})
	return decision, err
}

// Providers lists every provider the router can return, fallback included,
// so the composition root can check each one is configured.
func (r *Router) Providers() []ProviderName {
	providers := make([]ProviderName, 0, len(r.routes)+1)
	for _, route := range r.routes {
		providers = append(providers, route.provider)
	}
	if r.fallback != "" {
		providers = append(providers, r.fallback)
	}
	return providers
}

// resolution is a memoized routing outcome, error included.
type resolution struct {
	decision Decision
	err      error
}

func (r *Router) resolve(model Model) (Decision, error) {
	if model != "" {
		for _, route := range r.routes {
			if upstream, ok := route.resolve(model); ok {
				return Decision{Provider: route.provider, Model: upstream, Requested: model}, nil
			}
		}
	}
	if r.fallback == "" {
		return Decision{}, ErrNoRoute
	}
	return Decision{Provider: r.fallback, Model: model, Requested: model}, nil
}
