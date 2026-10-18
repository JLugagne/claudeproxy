package proxy

import (
	"sort"

	"github.com/JLugagne/claudeproxy/internal/proxy/config"
	"github.com/JLugagne/claudeproxy/internal/proxy/inbound/dashboard"
)

// providerInfos turns the provider configuration into the redacted view the
// console shows: names and endpoints, never the API keys.
func providerInfos(providers map[string]config.Provider) []dashboard.ProviderInfo {
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)

	infos := make([]dashboard.ProviderInfo, 0, len(names))
	for _, name := range names {
		provider := providers[name]
		infos = append(infos, dashboard.ProviderInfo{
			Name:       name,
			BaseURL:    provider.BaseURL,
			Auth:       provider.Auth,
			KeyPresent: provider.APIKey != "",
		})
	}
	return infos
}

// routeInfos keeps the configured order: the first match wins.
func routeInfos(routes []config.Route) []dashboard.RouteInfo {
	infos := make([]dashboard.RouteInfo, 0, len(routes))
	for _, route := range routes {
		infos = append(infos, dashboard.RouteInfo{
			Match:    route.Match,
			Regex:    route.Regex,
			Provider: route.Provider,
			Model:    route.Model,
		})
	}
	return infos
}
