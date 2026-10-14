package http_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JLugagne/claudeproxy/internal/proxy/domain"
	"github.com/JLugagne/claudeproxy/internal/proxy/domain/usecases/routeuc/routeuctest"
	inboundhttp "github.com/JLugagne/claudeproxy/internal/proxy/inbound/http"
)

type forwarded struct {
	provider      domain.ProviderName
	body          string
	contentLength int64
}

func newFakeUpstreams(names ...domain.ProviderName) (map[domain.ProviderName]http.Handler, *[]forwarded) {
	var calls []forwarded
	upstreams := make(map[domain.ProviderName]http.Handler, len(names))
	for _, name := range names {
		upstreams[name] = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			calls = append(calls, forwarded{provider: name, body: string(body), contentLength: r.ContentLength})
			w.WriteHeader(http.StatusAccepted)
		})
	}
	return upstreams, &calls
}

func staticRoutes(t *testing.T, wantModel domain.Model, decision domain.Decision) *routeuctest.MockRouteQueries {
	t.Helper()
	return &routeuctest.MockRouteQueries{
		ResolveRouteFn: func(_ context.Context, model domain.Model) (domain.Decision, error) {
			if model != wantModel {
				t.Errorf("resolved model %q, want %q", model, wantModel)
			}
			return decision, nil
		},
	}
}

func serve(handler http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestPassthroughKeepsBodyByteIdentical(t *testing.T) {
	body := `{ "model" : "claude-opus-5-5", "messages":[{"role":"user","content":"héllo"}] }`
	upstreams, calls := newFakeUpstreams("anthropic", "openrouter")
	routes := staticRoutes(t, "claude-opus-5-5", domain.Decision{Provider: "anthropic", Model: "claude-opus-5-5", Requested: "claude-opus-5-5"})
	handler := inboundhttp.NewHandler(routes, upstreams, 1<<20, slog.New(slog.DiscardHandler))

	rec := serve(handler, httptest.NewRequest(http.MethodPost, "/v1/messages?beta=true", strings.NewReader(body)))

	if rec.Code != http.StatusAccepted || len(*calls) != 1 {
		t.Fatalf("status %d, calls %+v", rec.Code, *calls)
	}
	got := (*calls)[0]
	if got.provider != "anthropic" || got.body != body || got.contentLength != int64(len(body)) {
		t.Fatalf("forwarded %+v", got)
	}
}

func TestRewrittenModelIsSplicedIntoBody(t *testing.T) {
	upstreams, calls := newFakeUpstreams("openrouter")
	routes := staticRoutes(t, "claude-sonnet-5-5", domain.Decision{Provider: "openrouter", Model: "deepseek/deepseek-v4.1-flash", Requested: "claude-sonnet-5-5"})
	handler := inboundhttp.NewHandler(routes, upstreams, 1<<20, slog.New(slog.DiscardHandler))

	serve(handler, httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", strings.NewReader(`{"model":"claude-sonnet-5-5","messages":[]}`)))

	want := `{"model":"deepseek/deepseek-v4.1-flash","messages":[]}`
	got := (*calls)[0]
	if got.body != want || got.contentLength != int64(len(want)) {
		t.Fatalf("forwarded %+v, want body %s", got, want)
	}
}

func TestNonMessagesRequestsUseEmptyModel(t *testing.T) {
	upstreams, calls := newFakeUpstreams("anthropic")
	routes := staticRoutes(t, "", domain.Decision{Provider: "anthropic"})
	handler := inboundhttp.NewHandler(routes, upstreams, 1<<20, slog.New(slog.DiscardHandler))

	serve(handler, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	serve(handler, httptest.NewRequest(http.MethodPost, "/api/event_logging/batch", strings.NewReader(`{"model":"ignored"}`)))

	if len(*calls) != 2 {
		t.Fatalf("calls %+v", *calls)
	}
}

func TestUnparsableBodyFallsBackToEmptyModel(t *testing.T) {
	upstreams, calls := newFakeUpstreams("anthropic")
	routes := staticRoutes(t, "", domain.Decision{Provider: "anthropic"})
	handler := inboundhttp.NewHandler(routes, upstreams, 1<<20, slog.New(slog.DiscardHandler))

	serve(handler, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`not json`)))

	if (*calls)[0].body != "not json" {
		t.Fatalf("calls %+v", *calls)
	}
}

func TestNoRouteReturnsAnthropicError(t *testing.T) {
	upstreams, calls := newFakeUpstreams("anthropic")
	routes := &routeuctest.MockRouteQueries{
		ResolveRouteFn: func(context.Context, domain.Model) (domain.Decision, error) {
			return domain.Decision{}, domain.ErrNoRoute
		},
	}
	handler := inboundhttp.NewHandler(routes, upstreams, 1<<20, slog.New(slog.DiscardHandler))

	rec := serve(handler, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"unknown"}`)))

	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"type":"not_found_error"`) || len(*calls) != 0 {
		t.Fatalf("status %d body %s calls %+v", rec.Code, rec.Body, *calls)
	}
}

func TestBodyTooLargeIsRejected(t *testing.T) {
	upstreams, calls := newFakeUpstreams("anthropic")
	handler := inboundhttp.NewHandler(&routeuctest.MockRouteQueries{}, upstreams, 16, slog.New(slog.DiscardHandler))

	for name, req := range map[string]*http.Request{
		"declared length": httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(strings.Repeat("x", 17))),
		"chunked":         chunked(httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(strings.Repeat("x", 17)))),
	} {
		t.Run(name, func(t *testing.T) {
			rec := serve(handler, req)

			if rec.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rec.Body.String(), "request_too_large") {
				t.Fatalf("status %d body %s", rec.Code, rec.Body)
			}
		})
	}
	if len(*calls) != 0 {
		t.Fatalf("calls %+v", *calls)
	}
}

func TestHealthIsAnsweredLocally(t *testing.T) {
	upstreams, calls := newFakeUpstreams("anthropic")
	handler := inboundhttp.NewHandler(&routeuctest.MockRouteQueries{}, upstreams, 16, slog.New(slog.DiscardHandler))

	rec := serve(handler, httptest.NewRequest(http.MethodGet, inboundhttp.HealthPath, nil))

	if rec.Code != http.StatusNoContent || len(*calls) != 0 {
		t.Fatalf("status %d calls %+v", rec.Code, *calls)
	}
}

func chunked(r *http.Request) *http.Request {
	r.ContentLength = -1
	return r
}
