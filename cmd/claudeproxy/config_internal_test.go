package claudeproxy

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/JLugagne/claudeproxy/internal/proxy"
	"github.com/JLugagne/claudeproxy/internal/proxy/domain"
)

func TestShippedConfigIsValid(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test")

	cfg, err := loadConfig(filepath.Join("..", "..", "configs", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := proxy.New(cfg.Proxy, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	p.Close()

	for model, want := range map[string]string{
		"claude-opus-5-5":              "anthropic",
		"deepseek/deepseek-v4.1-flash": "openrouter",
		"or:moonshotai/kimi-k3":        "openrouter",
	} {
		decision, err := p.Routes.ResolveRoute(t.Context(), domain.Model(model))
		if err != nil || string(decision.Provider) != want {
			t.Errorf("%s: got %+v, %v; want provider %s", model, decision, err, want)
		}
	}
}

func TestUnknownConfigKeysAreRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("proxy:\n  rutes: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := loadConfig(path); err == nil {
		t.Fatal("typo in configuration was accepted")
	}
}
