package e2eapi_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JLugagne/claudeproxy/internal/proxy"
	"github.com/JLugagne/claudeproxy/internal/proxy/config"
)

type seen struct {
	auth   string
	apiKey string
	body   string
}

func fakeProvider(t *testing.T, name string, requests chan<- seen) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests <- seen{auth: r.Header.Get("Authorization"), apiKey: r.Header.Get("X-Api-Key"), body: string(body)}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message_start\ndata: {\"provider\":\""+name+"\"}\n\n")
	}))
	t.Cleanup(server.Close)
	return server
}

func startProxy(t *testing.T, anthropicURL, openrouterURL string) *httptest.Server {
	t.Helper()
	p, err := proxy.New(config.Config{
		MaxBodyBytes:    1 << 20,
		DefaultProvider: "anthropic",
		Providers: map[string]config.Provider{
			"anthropic":  {BaseURL: anthropicURL, Auth: "passthrough"},
			"openrouter": {BaseURL: openrouterURL + "/api", Auth: "bearer", APIKey: "sk-or-test"},
		},
		Routes: []config.Route{
			{Match: "claude-haiku-*", Provider: "openrouter", Model: "deepseek/deepseek-v4.1-flash"},
			{Match: "*/*", Provider: "openrouter"},
			{Match: "claude-*", Provider: "anthropic"},
		},
	}, config.Metrics{Disabled: true}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	server := httptest.NewServer(p.Handler)
	t.Cleanup(server.Close)
	return server
}

func send(t *testing.T, proxyURL, model string) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, proxyURL+"/v1/messages", strings.NewReader(`{"model":"`+model+`","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer sk-ant-oat-user-token")
	req.Header.Set("Anthropic-Version", "2023-06-01")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	return string(body)
}

func TestModelsAreRoutedToTheirProvider(t *testing.T) {
	anthropicSeen := make(chan seen, 1)
	openrouterSeen := make(chan seen, 1)
	anthropic := fakeProvider(t, "anthropic", anthropicSeen)
	openrouter := fakeProvider(t, "openrouter", openrouterSeen)
	proxyServer := startProxy(t, anthropic.URL, openrouter.URL)

	t.Run("anthropic model is passed through with the user's token", func(t *testing.T) {
		stream := send(t, proxyServer.URL, "claude-opus-5-5")

		got := <-anthropicSeen
		if got.auth != "Bearer sk-ant-oat-user-token" || !strings.Contains(got.body, `"model":"claude-opus-5-5"`) {
			t.Fatalf("anthropic saw %+v", got)
		}
		if !strings.Contains(stream, `"provider":"anthropic"`) {
			t.Fatalf("stream %q", stream)
		}
	})

	t.Run("third-party model goes to openrouter without the user's token", func(t *testing.T) {
		stream := send(t, proxyServer.URL, "deepseek/deepseek-v4.1-flash")

		got := <-openrouterSeen
		if got.auth != "Bearer sk-or-test" || got.apiKey != "" {
			t.Fatalf("openrouter saw credentials %+v", got)
		}
		if !strings.Contains(stream, `"provider":"openrouter"`) {
			t.Fatalf("stream %q", stream)
		}
	})

	t.Run("claude tier can be diverted with a model rewrite", func(t *testing.T) {
		send(t, proxyServer.URL, "claude-haiku-5-5")

		got := <-openrouterSeen
		if !strings.Contains(got.body, `"model":"deepseek/deepseek-v4.1-flash"`) {
			t.Fatalf("openrouter saw body %s", got.body)
		}
	})

	t.Run("requests without a model go to the default provider", func(t *testing.T) {
		resp, err := http.Get(proxyServer.URL + "/v1/models")
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()

		if got := <-anthropicSeen; got.body != "" {
			t.Fatalf("anthropic saw %+v", got)
		}
	})
}
