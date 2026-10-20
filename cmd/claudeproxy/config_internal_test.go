package claudeproxy

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/JLugagne/claudeproxy/internal/proxy"
	proxyconfig "github.com/JLugagne/claudeproxy/internal/proxy/config"
	"github.com/JLugagne/claudeproxy/internal/proxy/domain"
)

func TestShippedConfigIsValid(t *testing.T) {
	cfg, err := loadConfig(filepath.Join("..", "..", "configs", "config.yaml.example"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := proxy.New(cfg.Proxy, proxyconfig.Metrics{Disabled: true}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	p.Close()

	for model, want := range map[string]string{
		"claude-opus-5-5": "anthropic",
		"minimax-m3":      "anthropic",
		"qwen3.8-flash":   "anthropic",
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
