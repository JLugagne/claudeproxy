package dashboard

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"
)

func TestHandlerServesTheShellForAppRoutes(t *testing.T) {
	h := NewHandler(Config{})
	for _, path := range []string{Prefix, Prefix + "/", Prefix + "/projects", Prefix + "/requests"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", path, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Errorf("%s: content-type = %q, want text/html", path, ct)
		}
		if !strings.Contains(rec.Body.String(), `id="root"`) {
			t.Errorf("%s: body does not look like the app shell", path)
		}
	}
}

func TestHandlerServesHashedAssetsImmutable(t *testing.T) {
	matches, err := fs.Glob(dist, "dist/assets/*.js")
	if err != nil || len(matches) == 0 {
		t.Fatalf("no embedded javascript asset found (err=%v)", err)
	}

	rec := httptest.NewRecorder()
	NewHandler(Config{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, Prefix+"/assets/"+path.Base(matches[0]), nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("content-type = %q, want javascript", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("cache-control = %q, want immutable", cc)
	}
}

func TestHandlerReturns404ForMissingAsset(t *testing.T) {
	rec := httptest.NewRecorder()
	NewHandler(Config{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, Prefix+"/assets/missing.js", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
