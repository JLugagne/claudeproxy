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
	"github.com/JLugagne/claudeproxy/internal/proxy/domain/usecases/usageuc"
	"github.com/JLugagne/claudeproxy/internal/proxy/inbound/dashboard"
	inboundhttp "github.com/JLugagne/claudeproxy/internal/proxy/inbound/http"
	"github.com/JLugagne/claudeproxy/internal/proxy/outbound/upstream"
	"github.com/JLugagne/claudeproxy/internal/proxy/outbound/usage"
)

// ErrInvalidConfig is returned by New when the proxy section is inconsistent.
var ErrInvalidConfig = errors.New("invalid proxy configuration")

// Proxy is a wired proxy. Close releases idle upstream connections.
type Proxy struct {
	Handler   http.Handler
	Routes    routeuc.RouteQueries
	transport *http.Transport
	// usage is the metrics store, nil when metrics are disabled.
	usage *usage.Store
	// live is the usage feed hub, nil when metrics are disabled.
	live *app.UsageHub
}

// New wires the proxy from a resolved configuration (see config.Config.Resolve).
// It returns ErrInvalidConfig, wrapping the offending route or provider error.
func New(cfg config.Config, metrics config.Metrics, logger *slog.Logger) (*Proxy, error) {
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

	recorder := usageuc.Recorder(usageuc.Nop{})
	var store *usage.Store
	var hub *app.UsageHub
	metricsErr := "usage metrics are disabled"
	if metrics.Enabled() {
		hub = app.NewUsageHub()
		path, err := metrics.ResolvePath()
		switch {
		case err != nil:
			metricsErr = "usage database path could not be resolved: " + err.Error()
			logger.Warn("usage metrics disabled: cannot resolve database path", "error", err)
		default:
			if store, err = usage.Open(path, logger, usage.WithPublisher(hub)); err != nil {
				metricsErr = "usage database could not be opened: " + err.Error()
				logger.Warn("usage metrics disabled: cannot open database", "path", path, "error", err)
				store = nil
			} else {
				recorder = store
				metricsErr = ""
				logger.Info("usage metrics enabled", "path", path)
			}
		}
	}

	consoleCfg := dashboard.Config{
		MetricsError:    metricsErr,
		DefaultProvider: cfg.DefaultProvider,
		Providers:       providerInfos(cfg.Providers),
		Routes:          routeInfos(cfg.Routes),
	}
	if store != nil {
		consoleCfg.Queries = store
		consoleCfg.Feed = hub
	}

	application := app.New(router)
	handler := inboundhttp.NewHandler(application, upstreams, cfg.MaxBodyBytes, logger, inboundhttp.WithUsage(recorder))

	// The console is only served when metrics are enabled: with metrics off
	// there is nothing to read, so /dashboard does not exist at all rather than
	// showing empty charts or falling through to the proxy.
	var console http.Handler
	if metrics.Enabled() {
		console = dashboard.NewHandler(consoleCfg)
	}
	return &Proxy{
		Handler:   consoleMux{proxy: handler, console: console},
		Routes:    application,
		transport: transport,
		usage:     store,
		live:      hub,
	}, nil
}

// CloseLiveFeeds ends every open dashboard events stream. Call it when the
// HTTP server starts shutting down: server.Shutdown waits for active
// connections, and an event stream never ends on its own.
func (p *Proxy) CloseLiveFeeds() {
	if p.live != nil {
		p.live.Close()
	}
}

// Close releases idle upstream connections; in-flight requests are unaffected.
func (p *Proxy) Close() {
	p.CloseLiveFeeds()
	p.transport.CloseIdleConnections()
	if p.usage != nil {
		_ = p.usage.Close()
	}
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
		Format:                        upstream.Format(p.Format),
		APIKey:                        p.APIKey,
		SetHeaders:                    p.Headers,
		DropHeaders:                   p.DropHeaders,
		FlattenMidConversationUpdates: p.FlattenMidConversationUpdates,
		KeepClaudeMetadata:            p.KeepClaudeMetadata,
	}, transport, logger)
}
