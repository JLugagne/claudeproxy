package config_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/JLugagne/claudeproxy/internal/proxy/config"
)

func TestResolveExpandsEnvAndAppliesDefaults(t *testing.T) {
	t.Setenv("CP_TEST_KEY", "sk-or-123")
	t.Setenv("CP_TEST_HOST", "openrouter.ai")
	cfg := config.Config{Providers: map[string]config.Provider{
		"openrouter": {
			BaseURL: "https://${CP_TEST_HOST}/api",
			APIKey:  "${CP_TEST_KEY}",
			Headers: map[string]string{"X-Key-Hint": "key=$CP_TEST_KEY"},
		},
	}}

	got, err := cfg.Resolve()

	if err != nil {
		t.Fatal(err)
	}
	p := got.Providers["openrouter"]
	if p.BaseURL != "https://openrouter.ai/api" || p.APIKey != "sk-or-123" || p.Headers["X-Key-Hint"] != "key=sk-or-123" {
		t.Fatalf("provider = %+v", p)
	}
	if got.MaxBodyBytes != config.DefaultMaxBodyBytes {
		t.Fatalf("MaxBodyBytes = %d", got.MaxBodyBytes)
	}
	if cfg.Providers["openrouter"].APIKey != "${CP_TEST_KEY}" {
		t.Fatal("Resolve mutated its receiver")
	}
}

func TestResolveReportsEveryUnsetVariable(t *testing.T) {
	cfg := config.Config{Providers: map[string]config.Provider{
		"a": {APIKey: "${CP_TEST_UNSET_A}"},
		"b": {APIKey: "${CP_TEST_UNSET_B}", BaseURL: "${CP_TEST_UNSET_A}"},
	}}

	_, err := cfg.Resolve()

	if !errors.Is(err, config.ErrUnsetEnv) || !strings.Contains(err.Error(), "CP_TEST_UNSET_A") || !strings.Contains(err.Error(), "CP_TEST_UNSET_B") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveDoesNotTouchRoutes(t *testing.T) {
	cfg := config.Config{Routes: []config.Route{{Regex: "^or/(.+)$", Provider: "p", Model: "$1"}}}

	got, err := cfg.Resolve()

	if err != nil || got.Routes[0].Model != "$1" || got.Routes[0].Regex != "^or/(.+)$" {
		t.Fatalf("routes = %+v, err = %v", got.Routes, err)
	}
}
