// Package proxy is the composition root of the proxy bounded context: New
// builds the domain router, one upstream adapter per provider and the inbound
// HTTP handler, all by direct injection.
package proxy

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/JLugagne/claudeproxy/internal/proxy/app"
	"github.com/JLugagne/claudeproxy/internal/proxy/config"
	"github.com/JLugagne/claudeproxy/internal/proxy/domain"
	"github.com/JLugagne/claudeproxy/internal/proxy/domain/usecases/routeuc"
	inboundhttp "github.com/JLugagne/claudeproxy/internal/proxy/inbound/http"
	"github.com/JLugagne/claudeproxy/internal/proxy/outbound/upstream"
)

// ErrInvalidConfig is returned by New when the proxy section is inconsistent.
var ErrInvalidConfig = errors.New("invalid proxy configuration")

// Proxy is a wired proxy. Close releases idle upstream connections.
type Proxy struct {
	Handler   http.Handler
	Routes    routeuc.RouteQueries
	transport *http.Transport
}

// New wires the proxy from a resolved configuration (see config.Config.Resolve).
// It returns ErrInvalidConfig, wrapping the offending route or provider error.
func New(cfg config.Config, logger *slog.Logger) (*Proxy, error) {
	routes, err := buildRoutes(cfg.Routes)
	if err != nil {
		return nil, errors.Join(ErrInvalidConfig, err)
	}
	router := domain.NewRouter(routes, domain.ProviderName(cfg.DefaultProvider))
	for _, name := range router.Providers() {
		if _, ok := cfg.Providers[string(name)]; !ok {
			return nil, errors.Join(ErrInvalidConfig, fmt.Errorf("provider %q is referenced but not declared", name))
		}
	}

	transport := upstream.NewTransport()
	upstreams := make(map[domain.ProviderName]http.Handler, len(cfg.Providers))
	for name, p := range cfg.Providers {
		handler, err := buildUpstream(name, p, transport, logger)
		if err != nil {
			return nil, errors.Join(ErrInvalidConfig, err)
		}
		upstreams[domain.ProviderName(name)] = handler
	}

	application := app.New(router)
	return &Proxy{
		Handler:   inboundhttp.NewHandler(application, upstreams, cfg.MaxBodyBytes, logger),
		Routes:    application,
		transport: transport,
	}, nil
}

// Close releases idle upstream connections; in-flight requests are unaffected.
func (p *Proxy) Close() {
	p.transport.CloseIdleConnections()
}

func buildRoutes(defs []config.Route) ([]domain.Route, error) {
	routes := make([]domain.Route, 0, len(defs))
	for i, def := range defs {
		var route domain.Route
		var err error
		switch {
		case def.Match != "" && def.Regex != "":
			err = errors.New("set either match or regex, not both")
		case def.Match != "":
			route, err = domain.NewGlobRoute(def.Match, domain.ProviderName(def.Provider), def.Model)
		case def.Regex != "":
			route, err = domain.NewRegexRoute(def.Regex, domain.ProviderName(def.Provider), def.Model)
		default:
			err = errors.New("match or regex is required")
		}
		if err != nil {
			return nil, errors.Join(fmt.Errorf("routes[%d]", i), err)
		}
		routes = append(routes, route)
	}
	return routes, nil
}

func buildUpstream(name string, p config.Provider, transport http.RoundTripper, logger *slog.Logger) (http.Handler, error) {
	baseURL, err := url.Parse(p.BaseURL)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("provider %q: base_url", name), err)
	}
	return upstream.New(upstream.Config{
		Name:                          name,
		BaseURL:                       baseURL,
		Auth:                          upstream.AuthMode(p.Auth),
		APIKey:                        p.APIKey,
		SetHeaders:                    p.Headers,
		DropHeaders:                   p.DropHeaders,
		FlattenMidConversationUpdates: p.FlattenMidConversationUpdates,
	}, transport, logger)
}
