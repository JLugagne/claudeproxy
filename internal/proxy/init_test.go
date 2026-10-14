package proxy_test

import (
	"errors"
	"log/slog"
	"testing"

	"github.com/JLugagne/claudeproxy/internal/proxy"
	"github.com/JLugagne/claudeproxy/internal/proxy/config"
)

func validProviders() map[string]config.Provider {
	return map[string]config.Provider{
		"anthropic":  {BaseURL: "https://api.anthropic.com", Auth: "passthrough"},
		"openrouter": {BaseURL: "https://openrouter.ai/api", Auth: "bearer", APIKey: "k"},
	}
}

func TestNewAcceptsValidConfig(t *testing.T) {
	p, err := proxy.New(config.Config{
		MaxBodyBytes:    1024,
		DefaultProvider: "anthropic",
		Providers:       validProviders(),
		Routes: []config.Route{
			{Match: "claude-*", Provider: "anthropic"},
			{Regex: "^or/(.+)$", Provider: "openrouter", Model: "$1"},
		},
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	p.Close()
}

func TestNewRejectsInvalidConfig(t *testing.T) {
	tests := map[string]config.Config{
		"undeclared route provider": {Providers: validProviders(), Routes: []config.Route{{Match: "x", Provider: "deepseek"}}},
		"undeclared default":        {Providers: validProviders(), DefaultProvider: "deepseek"},
		"match and regex":           {Providers: validProviders(), Routes: []config.Route{{Match: "x", Regex: "x", Provider: "anthropic"}}},
		"no matcher":                {Providers: validProviders(), Routes: []config.Route{{Provider: "anthropic"}}},
		"bad regex":                 {Providers: validProviders(), Routes: []config.Route{{Regex: "(", Provider: "anthropic"}}},
		"missing auth":              {Providers: map[string]config.Provider{"x": {BaseURL: "https://x.dev", APIKey: "k"}}},
		"missing api key":           {Providers: map[string]config.Provider{"x": {BaseURL: "https://x.dev", Auth: "bearer"}}},
		"bad base url":              {Providers: map[string]config.Provider{"x": {BaseURL: "://", Auth: "passthrough"}}},
	}
	for name, cfg := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := proxy.New(cfg, slog.New(slog.DiscardHandler)); !errors.Is(err, proxy.ErrInvalidConfig) {
				t.Fatalf("error = %v, want ErrInvalidConfig", err)
			}
		})
	}
}
