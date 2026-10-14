package domain_test

import (
	"errors"
	"testing"

	"github.com/JLugagne/claudeproxy/internal/proxy/domain"
)

func mustGlob(t *testing.T, glob string, provider domain.ProviderName, rewrite string) domain.Route {
	t.Helper()
	route, err := domain.NewGlobRoute(glob, provider, rewrite)
	if err != nil {
		t.Fatal(err)
	}
	return route
}

func mustRegex(t *testing.T, expr string, provider domain.ProviderName, rewrite string) domain.Route {
	t.Helper()
	route, err := domain.NewRegexRoute(expr, provider, rewrite)
	if err != nil {
		t.Fatal(err)
	}
	return route
}

func TestRouterResolve(t *testing.T) {
	router := domain.NewRouter([]domain.Route{
		mustGlob(t, "claude-sonnet-*", "openrouter", "deepseek/deepseek-v4.1-flash"),
		mustGlob(t, "claude-*", "anthropic", ""),
		mustGlob(t, "deepseek/*", "openrouter", ""),
		mustRegex(t, `^or:(.+)$`, "openrouter", "$1"),
		mustGlob(t, "local/*-?", "ollama", "$1:$2"),
	}, "anthropic")

	tests := []struct {
		model    domain.Model
		provider domain.ProviderName
		upstream domain.Model
	}{
		{model: "claude-sonnet-5-5", provider: "openrouter", upstream: "deepseek/deepseek-v4.1-flash"},
		{model: "claude-opus-5-5", provider: "anthropic", upstream: "claude-opus-5-5"},
		{model: "deepseek/deepseek-v4.1-flash", provider: "openrouter", upstream: "deepseek/deepseek-v4.1-flash"},
		{model: "or:moonshotai/kimi-k3", provider: "openrouter", upstream: "moonshotai/kimi-k3"},
		{model: "local/qwen-7", provider: "ollama", upstream: "qwen:7"},
		{model: "gpt-6", provider: "anthropic", upstream: "gpt-6"},
		{model: "", provider: "anthropic", upstream: ""},
	}
	for _, tt := range tests {
		t.Run(string(tt.model), func(t *testing.T) {
			got, err := router.Resolve(tt.model)
			if err != nil {
				t.Fatal(err)
			}
			if got.Provider != tt.provider || got.Model != tt.upstream || got.Requested != tt.model {
				t.Fatalf("got %+v, want provider %q model %q", got, tt.provider, tt.upstream)
			}
			if got.Rewritten() != (tt.upstream != tt.model) {
				t.Fatalf("Rewritten() = %v", got.Rewritten())
			}
		})
	}
}

func TestGlobIsAnchoredAndLiteral(t *testing.T) {
	router := domain.NewRouter([]domain.Route{mustGlob(t, "a.b", "p", "")}, "")

	if _, err := router.Resolve("a.b"); err != nil {
		t.Fatalf("exact match failed: %v", err)
	}
	for _, model := range []domain.Model{"axb", "xa.b", "a.bx"} {
		if _, err := router.Resolve(model); !errors.Is(err, domain.ErrNoRoute) {
			t.Fatalf("%q: error = %v, want ErrNoRoute", model, err)
		}
	}
}

func TestRouterWithoutFallback(t *testing.T) {
	router := domain.NewRouter(nil, "")

	if _, err := router.Resolve(""); !errors.Is(err, domain.ErrNoRoute) {
		t.Fatalf("error = %v, want ErrNoRoute", err)
	}
}

func TestInvalidRoutes(t *testing.T) {
	if _, err := domain.NewRegexRoute("(", "p", ""); !errors.Is(err, domain.ErrInvalidRoute) {
		t.Fatalf("bad regex: error = %v", err)
	}
	if _, err := domain.NewGlobRoute("*", "", ""); !errors.Is(err, domain.ErrInvalidRoute) {
		t.Fatalf("missing provider: error = %v", err)
	}
}

func TestRouterProviders(t *testing.T) {
	router := domain.NewRouter([]domain.Route{mustGlob(t, "x", "a", ""), mustGlob(t, "y", "b", "")}, "c")

	got := router.Providers()

	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("got %v", got)
	}
}
