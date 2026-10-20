package upstream_test

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JLugagne/claudeproxy/internal/proxy/outbound/upstream"
	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

type captured struct {
	path   string
	query  string
	header http.Header
	body   string
}

func newCapturingServer(t *testing.T) (*httptest.Server, <-chan captured) {
	t.Helper()
	requests := make(chan captured, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests <- captured{path: r.URL.Path, query: r.URL.RawQuery, header: r.Header.Clone(), body: string(body)}
		w.Header().Set("Request-Id", "req_123")
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)
	return server, requests
}

func newProxy(t *testing.T, cfg upstream.Config) *httptest.Server {
	t.Helper()
	handler, err := upstream.New(cfg, upstream.NewTransport(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewServer(handler)
	t.Cleanup(proxy.Close)
	return proxy
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func newClientRequest(t *testing.T, target string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, target+"/v1/messages?beta=true", strings.NewReader(`{"model":"m"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer sk-ant-oat-secret")
	req.Header.Set("X-Api-Key", "sk-ant-api-secret")
	req.Header.Set("Anthropic-Version", "2023-06-01")
	req.Header.Set("Anthropic-Beta", "oauth-2025-04-20,interleaved-thinking-2025-05-14")
	req.Header.Set("User-Agent", "claude-cli/9.9.9")
	return req
}

func do(t *testing.T, req *http.Request) *http.Response {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestPassthroughForwardsRequestUntouched(t *testing.T) {
	server, requests := newCapturingServer(t)
	proxy := newProxy(t, upstream.Config{Name: "anthropic", BaseURL: mustURL(t, server.URL), Auth: upstream.AuthPassthrough})

	resp := do(t, newClientRequest(t, proxy.URL))

	got := <-requests
	if got.path != "/v1/messages" || got.query != "beta=true" || got.body != `{"model":"m"}` {
		t.Fatalf("unexpected request %+v", got)
	}
	for name, want := range map[string]string{
		"Authorization":     "Bearer sk-ant-oat-secret",
		"X-Api-Key":         "sk-ant-api-secret",
		"Anthropic-Version": "2023-06-01",
		"Anthropic-Beta":    "oauth-2025-04-20,interleaved-thinking-2025-05-14",
		"User-Agent":        "claude-cli/9.9.9",
	} {
		if got.header.Get(name) != want {
			t.Errorf("header %s = %q, want %q", name, got.header.Get(name), want)
		}
	}
	if got.header.Get("X-Forwarded-For") != "" {
		t.Errorf("X-Forwarded-For was added: %q", got.header.Get("X-Forwarded-For"))
	}
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusTeapot || resp.Header.Get("Request-Id") != "req_123" || string(body) != `{"ok":true}` {
		t.Fatalf("response not relayed: %d %v %s", resp.StatusCode, resp.Header, body)
	}
}

func TestBearerReplacesClientCredentials(t *testing.T) {
	server, requests := newCapturingServer(t)
	proxy := newProxy(t, upstream.Config{
		Name:        "openrouter",
		BaseURL:     mustURL(t, server.URL+"/api"),
		Auth:        upstream.AuthBearer,
		APIKey:      "sk-or-key",
		SetHeaders:  map[string]string{"X-Title": "claudeproxy"},
		DropHeaders: []string{"Anthropic-Beta"},
	})

	do(t, newClientRequest(t, proxy.URL))

	got := <-requests
	if got.path != "/api/v1/messages" {
		t.Fatalf("path = %q", got.path)
	}
	if got.header.Get("Authorization") != "Bearer sk-or-key" {
		t.Errorf("Authorization = %q", got.header.Get("Authorization"))
	}
	if got.header.Get("X-Api-Key") != "" || got.header.Get("Anthropic-Beta") != "" {
		t.Errorf("client headers leaked: %v", got.header)
	}
	if got.header.Get("X-Title") != "claudeproxy" || got.header.Get("Anthropic-Version") != "2023-06-01" {
		t.Errorf("headers = %v", got.header)
	}
}

func TestAPIKeyReplacesClientCredentials(t *testing.T) {
	server, requests := newCapturingServer(t)
	proxy := newProxy(t, upstream.Config{Name: "deepseek", BaseURL: mustURL(t, server.URL), Auth: upstream.AuthAPIKey, APIKey: "sk-ds"})

	do(t, newClientRequest(t, proxy.URL))

	got := <-requests
	if got.header.Get("X-Api-Key") != "sk-ds" || got.header.Get("Authorization") != "" {
		t.Fatalf("headers = %v", got.header)
	}
}

func TestStreamingIsFlushedImmediately(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message_start\ndata: {}\n\n")
		w.(http.Flusher).Flush()
		<-release
		_, _ = io.WriteString(w, "event: message_stop\ndata: {}\n\n")
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	proxy := newProxy(t, upstream.Config{Name: "anthropic", BaseURL: mustURL(t, server.URL), Auth: upstream.AuthPassthrough})

	resp := do(t, newClientRequest(t, proxy.URL))

	lines := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(resp.Body).ReadString('\n')
		lines <- line
	}()
	select {
	case line := <-lines:
		if line != "event: message_start\n" {
			t.Fatalf("first line = %q", line)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first SSE event was buffered by the proxy")
	}
	close(release)
}

func TestUnreachableUpstreamReturnsAnthropicError(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	target := server.URL
	server.Close()
	proxy := newProxy(t, upstream.Config{Name: "down", BaseURL: mustURL(t, target), Auth: upstream.AuthPassthrough})

	resp := do(t, newClientRequest(t, proxy.URL))

	var body struct {
		Type  string `json:"type"`
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadGateway || body.Type != "error" || body.Error.Type != "api_error" {
		t.Fatalf("got %d %+v", resp.StatusCode, body)
	}
}

func TestNewRejectsInvalidConfig(t *testing.T) {
	tests := map[string]upstream.Config{
		"relative url":    {Name: "x", BaseURL: &url.URL{Path: "/api"}, Auth: upstream.AuthPassthrough},
		"missing url":     {Name: "x", Auth: upstream.AuthPassthrough},
		"missing api key": {Name: "x", BaseURL: &url.URL{Scheme: "https", Host: "h"}, Auth: upstream.AuthBearer},
		"unknown auth":    {Name: "x", BaseURL: &url.URL{Scheme: "https", Host: "h"}, Auth: "basic"},
		"missing auth":    {Name: "x", BaseURL: &url.URL{Scheme: "https", Host: "h"}, APIKey: "k"},
	}
	for name, cfg := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := upstream.New(cfg, upstream.NewTransport(), slog.New(slog.DiscardHandler)); !errors.Is(err, upstream.ErrInvalidConfig) {
				t.Fatalf("error = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestUpstreamErrorBodyIsLoggedAndRelayed(t *testing.T) {
	errorBody := `{"type":"error","error":{"type":"invalid_request_error","message":"field x is not supported"}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, errorBody)
	}))
	t.Cleanup(server.Close)
	var logs lockedBuffer
	handler, err := upstream.New(upstream.Config{Name: "openrouter", BaseURL: mustURL(t, server.URL), Auth: upstream.AuthBearer, APIKey: "sk-or"}, upstream.NewTransport(), slog.New(slog.NewTextHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewServer(handler)
	t.Cleanup(proxy.Close)

	resp := do(t, newClientRequest(t, proxy.URL))

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusBadRequest || string(body) != errorBody {
		t.Fatalf("response not relayed: %d %s", resp.StatusCode, body)
	}
	if got := logs.String(); !strings.Contains(got, "level=WARN") || !strings.Contains(got, "status=400") || !strings.Contains(got, "field x is not supported") {
		t.Fatalf("error body not logged: %q", got)
	}
}

func TestSuccessBodyIsNotLogged(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"secret":"conversation"}`)
	}))
	t.Cleanup(server.Close)
	var logs lockedBuffer
	handler, err := upstream.New(upstream.Config{Name: "anthropic", BaseURL: mustURL(t, server.URL), Auth: upstream.AuthPassthrough}, upstream.NewTransport(), slog.New(slog.NewTextHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewServer(handler)
	t.Cleanup(proxy.Close)

	resp := do(t, newClientRequest(t, proxy.URL))

	if body, _ := io.ReadAll(resp.Body); string(body) != `{"secret":"conversation"}` {
		t.Fatalf("body %s", body)
	}
	if got := logs.String(); strings.Contains(got, "conversation") {
		t.Fatalf("success body was logged: %q", got)
	}
}

func TestCompressedErrorBodyIsLoggedDecoded(t *testing.T) {
	payload := `{"error":{"message":"field x is not supported","detail":"` + strings.Repeat("compressible ", 50) + `"}}`
	tests := map[string]func(io.Writer) io.WriteCloser{
		"gzip": func(w io.Writer) io.WriteCloser { return gzip.NewWriter(w) },
		"br":   func(w io.Writer) io.WriteCloser { return brotli.NewWriter(w) },
		"zstd": func(w io.Writer) io.WriteCloser {
			zw, _ := zstd.NewWriter(w)
			return zw
		},
	}
	for encoding, newWriter := range tests {
		t.Run(encoding, func(t *testing.T) {
			var compressed bytes.Buffer
			zw := newWriter(&compressed)
			_, _ = io.WriteString(zw, payload)
			_ = zw.Close()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Encoding", encoding)
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write(compressed.Bytes())
			}))
			t.Cleanup(server.Close)
			var logs lockedBuffer
			handler, err := upstream.New(upstream.Config{Name: "anthropic", BaseURL: mustURL(t, server.URL), Auth: upstream.AuthPassthrough}, upstream.NewTransport(), slog.New(slog.NewTextHandler(&logs, nil)))
			if err != nil {
				t.Fatal(err)
			}
			proxy := httptest.NewServer(handler)
			t.Cleanup(proxy.Close)
			req := newClientRequest(t, proxy.URL)
			req.Header.Set("Accept-Encoding", encoding)

			resp := do(t, req)

			if body, _ := io.ReadAll(resp.Body); !bytes.Equal(body, compressed.Bytes()) {
				t.Fatalf("compressed body not relayed untouched")
			}
			if got := logs.String(); !strings.Contains(got, "field x is not supported") {
				t.Fatalf("decoded error body not logged: %q", got)
			}
		})
	}
}

func TestUndecodableErrorBodyIsSummarized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "compress")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte{0x1f, 0x9d, 0x90, 0xff})
	}))
	t.Cleanup(server.Close)
	var logs lockedBuffer
	handler, err := upstream.New(upstream.Config{Name: "anthropic", BaseURL: mustURL(t, server.URL), Auth: upstream.AuthPassthrough}, upstream.NewTransport(), slog.New(slog.NewTextHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewServer(handler)
	t.Cleanup(proxy.Close)

	do(t, newClientRequest(t, proxy.URL))

	if got := logs.String(); !strings.Contains(got, `content-encoding \"compress\" not decoded`) || strings.Contains(got, `\x9d`) {
		t.Fatalf("undecodable body not summarized: %q", got)
	}
}

func TestMidConversationUpdatesAreFlattened(t *testing.T) {
	const bash = `{"name":"Bash","description":"run","input_schema":{"type":"object"}}`
	const weather = `{"name":"get_weather","description":"weather","input_schema":{"type":"object","properties":{"city":{"type":"string"}}}}`
	const deferredWeather = `{"name":"get_weather","description":"old","defer_loading":true,"input_schema":{"type":"object"}}`
	const deferredDocs = `{"name":"docs","description":"docs","defer_loading":true,"input_schema":{"type":"object"}}`
	tests := map[string]struct {
		inline bool
		body   string
		want   string
	}{
		"definition is moved to tools and the block removed": {
			inline: true,
			body:   `{"model":"m","max_tokens":12345678901,"tools":[` + bash + `],"messages":[{"role":"user","content":"hi"},{"role":"system","content":[{"type":"text","text":"added"},{"type":"tool_addition","tool":{"type":"tool_definition","definition":` + weather + `},"cache_control":{"type":"ephemeral"}}]}]}`,
			want:   `{"model":"m","max_tokens":12345678901,"tools":[` + bash + `,` + weather + `],"messages":[{"role":"user","content":"hi"},{"role":"system","content":[{"type":"text","text":"added"}]}]}`,
		},
		"definition replaces a deferred tool of the same name": {
			inline: true,
			body:   `{"model":"m","tools":[` + bash + `,` + deferredWeather + `],"messages":[{"role":"system","content":[{"type":"text","text":"added"},{"type":"tool_addition","tool":{"type":"tool_definition","definition":` + weather + `}}]}]}`,
			want:   `{"model":"m","tools":[` + bash + `,` + weather + `],"messages":[{"role":"system","content":[{"type":"text","text":"added"}]}]}`,
		},
		"reference undefers the tool": {
			inline: true,
			body:   `{"model":"m","tools":[` + bash + `,` + deferredDocs + `],"messages":[{"role":"system","content":[{"type":"text","text":"added"},{"type":"tool_addition","tool":{"type":"tool_reference","name":"docs"}}]}]}`,
			want:   `{"model":"m","tools":[` + bash + `,{"name":"docs","description":"docs","input_schema":{"type":"object"}}],"messages":[{"role":"system","content":[{"type":"text","text":"added"}]}]}`,
		},
		"message left empty is dropped": {
			inline: true,
			body:   `{"model":"m","messages":[{"role":"user","content":"hi"},{"role":"system","content":[{"type":"tool_addition","tool":{"type":"tool_definition","definition":` + weather + `}}]}]}`,
			want:   `{"model":"m","tools":[` + weather + `],"messages":[{"role":"user","content":"hi"}]}`,
		},
		"message output_config replaces the request one": {
			inline: true,
			body:   `{"model":"m","output_config":{"effort":"high"},"messages":[{"role":"user","content":"hi"},{"role":"system","output_config":{"effort":"medium"},"content":[{"type":"text","text":"x"}]}]}`,
			want:   `{"model":"m","output_config":{"effort":"medium"},"messages":[{"role":"user","content":"hi"},{"role":"system","content":[{"type":"text","text":"x"}]}]}`,
		},
		"latest message output_config wins": {
			inline: true,
			body:   `{"model":"m","messages":[{"role":"system","output_config":{"effort":"low"},"content":[{"type":"text","text":"a"}]},{"role":"user","content":"hi"},{"role":"system","output_config":{"effort":"max"},"content":[{"type":"text","text":"b"}]}]}`,
			want:   `{"model":"m","output_config":{"effort":"max"},"messages":[{"role":"system","content":[{"type":"text","text":"a"}]},{"role":"user","content":"hi"},{"role":"system","content":[{"type":"text","text":"b"}]}]}`,
		},
		"request-level output_config alone is byte identical": {
			inline: true,
			body:   `{"model":"m", "output_config":{"effort":"high"}, "messages":[{"role":"user","content":"hi"}]}`,
			want:   `{"model":"m", "output_config":{"effort":"high"}, "messages":[{"role":"user","content":"hi"}]}`,
		},
		"request without additions is byte identical": {
			inline: true,
			body:   `{"model":"m", "tools":[` + bash + `], "messages":[{"role":"user","content":"tool_addition"}]}`,
			want:   `{"model":"m", "tools":[` + bash + `], "messages":[{"role":"user","content":"tool_addition"}]}`,
		},
		"disabled by default": {
			body: `{"model":"m","messages":[{"role":"system","content":[{"type":"tool_addition","tool":{"type":"tool_definition","definition":` + weather + `}}]}]}`,
			want: `{"model":"m","messages":[{"role":"system","content":[{"type":"tool_addition","tool":{"type":"tool_definition","definition":` + weather + `}}]}]}`,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			server, requests := newCapturingServer(t)
			proxy := newProxy(t, upstream.Config{Name: "openrouter", BaseURL: mustURL(t, server.URL), Auth: upstream.AuthBearer, APIKey: "sk-or", FlattenMidConversationUpdates: tt.inline})
			req, err := http.NewRequest(http.MethodPost, proxy.URL+"/v1/messages", strings.NewReader(tt.body))
			if err != nil {
				t.Fatal(err)
			}

			do(t, req)

			got := (<-requests).body
			if tt.body == tt.want {
				if got != tt.want {
					t.Fatalf("body was modified\n got %s\nwant %s", got, tt.want)
				}
				return
			}
			if !jsonEqual(t, got, tt.want) {
				t.Fatalf("upstream body\n got %s\nwant %s", got, tt.want)
			}
		})
	}
}

func jsonEqual(t *testing.T, a, b string) bool {
	t.Helper()
	decode := func(s string) any {
		dec := json.NewDecoder(strings.NewReader(s))
		dec.UseNumber()
		var v any
		if err := dec.Decode(&v); err != nil {
			t.Fatalf("invalid JSON %s: %v", s, err)
		}
		return v
	}
	return reflect.DeepEqual(decode(a), decode(b))
}

func TestNonPassthroughStripsClaudeIdentity(t *testing.T) {
	server, requests := newCapturingServer(t)
	proxy := newProxy(t, upstream.Config{Name: "openrouter", BaseURL: mustURL(t, server.URL), Auth: upstream.AuthBearer, APIKey: "sk-or"})

	req := newClientRequest(t, proxy.URL)
	req.Header.Set("X-Project", "owner/repo")
	req.Header.Set("X-App", "cli")
	req.Header.Set("X-Stainless-Lang", "js")
	req.Header.Set("X-Anthropic-Something", "x")
	req.Header.Set("X-Claude-Code-Session-Id", "sess-1")
	do(t, req)

	got := <-requests
	for _, name := range []string{"X-Project", "X-App", "X-Stainless-Lang", "X-Anthropic-Something", "X-Claude-Code-Session-Id", "Anthropic-Beta", "X-Api-Key"} {
		if got.header.Get(name) != "" {
			t.Errorf("header %s leaked: %q", name, got.header.Get(name))
		}
	}
	if ua := got.header.Get("User-Agent"); strings.Contains(ua, "claude") {
		t.Errorf("claude user-agent leaked: %q", ua)
	}
	if got.header.Get("Anthropic-Version") != "2023-06-01" {
		t.Errorf("protocol header dropped: %q", got.header.Get("Anthropic-Version"))
	}
}

func TestOnlyProtocolHeadersReachThirdPartyProviders(t *testing.T) {
	unknown := map[string]string{
		"X-Claude-Code-Future": "id-1",
		"Traceparent":          "00-abc-def-01",
		"Some-Client-Header":   "value",
	}
	protocol := map[string]string{
		"Accept":            "application/json",
		"Anthropic-Version": "2023-06-01",
		"Content-Type":      "application/json",
		"Content-Encoding":  "identity",
	}
	for name, tc := range map[string]struct {
		cfg         upstream.Config
		wantUnknown bool
	}{
		"bearer drops unknown headers":    {cfg: upstream.Config{Name: "openrouter", Auth: upstream.AuthBearer, APIKey: "sk-or"}},
		"x-api-key drops unknown headers": {cfg: upstream.Config{Name: "opencode-go", Auth: upstream.AuthAPIKey, APIKey: "sk-oc"}},
		"passthrough keeps them":          {cfg: upstream.Config{Name: "anthropic", Auth: upstream.AuthPassthrough}, wantUnknown: true},
	} {
		t.Run(name, func(t *testing.T) {
			server, requests := newCapturingServer(t)
			tc.cfg.BaseURL = mustURL(t, server.URL)
			proxy := newProxy(t, tc.cfg)

			req := newClientRequest(t, proxy.URL)
			for k, v := range unknown {
				req.Header.Set(k, v)
			}
			for k, v := range protocol {
				req.Header.Set(k, v)
			}
			do(t, req)

			got := <-requests
			for k, v := range unknown {
				if forwarded := got.header.Get(k) == v; forwarded != tc.wantUnknown {
					t.Errorf("header %s forwarded = %v, want %v", k, forwarded, tc.wantUnknown)
				}
			}
			for k, v := range protocol {
				if got.header.Get(k) != v {
					t.Errorf("protocol header %s = %q, want %q", k, got.header.Get(k), v)
				}
			}
		})
	}
}

func TestProxyMetaHeaderNeverReachesUpstream(t *testing.T) {
	server, requests := newCapturingServer(t)
	proxy := newProxy(t, upstream.Config{Name: "anthropic", BaseURL: mustURL(t, server.URL), Auth: upstream.AuthPassthrough})

	req := newClientRequest(t, proxy.URL)
	req.Header.Set("X-Project", "owner/repo")
	do(t, req)

	if got := (<-requests).header.Get("X-Project"); got != "" {
		t.Fatalf("X-Project leaked to upstream: %q", got)
	}
}

func TestNonPassthroughStripsClaudeMetadata(t *testing.T) {
	tests := map[string]struct{ body, want string }{
		"metadata is removed": {
			body: `{"model":"m","metadata":{"user_id":"u"},"messages":[]}`,
			want: `{"model":"m","messages":[]}`,
		},
		"attribution block is removed": {
			body: `{"model":"m","system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=1"},{"type":"text","text":"real"}],"messages":[]}`,
			want: `{"model":"m","system":[{"type":"text","text":"real"}],"messages":[]}`,
		},
		"system emptied is dropped": {
			body: `{"model":"m","system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=1"}],"messages":[]}`,
			want: `{"model":"m","messages":[]}`,
		},
		"plain string system is untouched": {
			body: `{"model":"m","system":"x-anthropic-billing-header: nope","messages":[]}`,
			want: `{"model":"m","system":"x-anthropic-billing-header: nope","messages":[]}`,
		},
		"nothing to strip is byte identical": {
			body: `{"model":"m","messages":[]}`,
			want: `{"model":"m","messages":[]}`,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			server, requests := newCapturingServer(t)
			proxy := newProxy(t, upstream.Config{Name: "openrouter", BaseURL: mustURL(t, server.URL), Auth: upstream.AuthBearer, APIKey: "sk-or"})
			req, err := http.NewRequest(http.MethodPost, proxy.URL+"/v1/messages", strings.NewReader(tt.body))
			if err != nil {
				t.Fatal(err)
			}
			do(t, req)
			got := (<-requests).body
			if tt.body == tt.want {
				if got != tt.want {
					t.Fatalf("body changed unexpectedly\n got %s\nwant %s", got, tt.want)
				}
				return
			}
			if !jsonEqual(t, got, tt.want) {
				t.Fatalf("upstream body\n got %s\nwant %s", got, tt.want)
			}
		})
	}
}

func TestPassthroughKeepsClaudeMetadata(t *testing.T) {
	server, requests := newCapturingServer(t)
	proxy := newProxy(t, upstream.Config{Name: "anthropic", BaseURL: mustURL(t, server.URL), Auth: upstream.AuthPassthrough})

	body := `{"model":"m","metadata":{"user_id":"u"},"messages":[]}`
	req, err := http.NewRequest(http.MethodPost, proxy.URL+"/v1/messages", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	do(t, req)

	if got := (<-requests).body; got != body {
		t.Fatalf("passthrough body was modified\n got %s\nwant %s", got, body)
	}
}

func TestKeepClaudeMetadataOptsOut(t *testing.T) {
	server, requests := newCapturingServer(t)
	proxy := newProxy(t, upstream.Config{Name: "openrouter", BaseURL: mustURL(t, server.URL), Auth: upstream.AuthBearer, APIKey: "sk-or", KeepClaudeMetadata: true})

	body := `{"model":"m","metadata":{"user_id":"u"},"messages":[]}`
	req, err := http.NewRequest(http.MethodPost, proxy.URL+"/v1/messages", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	do(t, req)

	if got := (<-requests).body; got != body {
		t.Fatalf("opt-out body was modified\n got %s\nwant %s", got, body)
	}
}

func TestAcceptEncodingIsNeverForwarded(t *testing.T) {
	for name, cfg := range map[string]func(*url.URL) upstream.Config{
		"passthrough": func(u *url.URL) upstream.Config {
			return upstream.Config{Name: "anthropic", BaseURL: u, Auth: upstream.AuthPassthrough}
		},
		"bearer": func(u *url.URL) upstream.Config {
			return upstream.Config{Name: "other", BaseURL: u, Auth: upstream.AuthBearer, APIKey: "sk-x"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			server, requests := newCapturingServer(t)
			proxy := newProxy(t, cfg(mustURL(t, server.URL)))
			req := newClientRequest(t, proxy.URL)
			req.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")

			do(t, req)

			// A compressed response cannot be read for its usage counters, so
			// the provider is asked for an identity encoding.
			if got := (<-requests).header.Get("Accept-Encoding"); got != "" {
				t.Errorf("Accept-Encoding = %q, want it removed", got)
			}
		})
	}
}
