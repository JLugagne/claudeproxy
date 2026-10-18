package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConsoleMuxServesDashboardAndForwardsEverythingElse(t *testing.T) {
	var proxyHit, consoleHit bool
	mux := consoleMux{
		proxy:   http.HandlerFunc(func(http.ResponseWriter, *http.Request) { proxyHit = true }),
		console: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { consoleHit = true }),
	}

	serve := func(path string) {
		proxyHit, consoleHit = false, false
		mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}

	dashboardPaths := []string{"/dashboard", "/dashboard/", "/dashboard/projects", "/dashboard/assets/app.js"}
	for _, path := range dashboardPaths {
		serve(path)
		if !consoleHit || proxyHit {
			t.Errorf("%s: console=%v proxy=%v, want the console only", path, consoleHit, proxyHit)
		}
	}

	proxyPaths := []string{"/", "/v1/messages", "/dashboardx", "/_claudeproxy/health"}
	for _, path := range proxyPaths {
		serve(path)
		if !proxyHit || consoleHit {
			t.Errorf("%s: console=%v proxy=%v, want the proxy only", path, consoleHit, proxyHit)
		}
	}
}

func TestConsoleIsAbsentWhenMetricsAreDisabled(t *testing.T) {
	proxyHit := false
	mux := consoleMux{
		proxy: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { proxyHit = true }),
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/dashboard/", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if proxyHit {
		t.Fatal("a disabled console must not fall through to the proxy")
	}
}
