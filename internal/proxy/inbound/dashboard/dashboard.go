// Package dashboard serves the embedded usage console.
//
// The React application lives in web/ and is built by `npm run build` straight
// into dist/, which is embedded at build time. The console is a single-page
// app: a path under Prefix that is not a static asset answers index.html, and
// the client routes on the path.
package dashboard

import (
	"bytes"
	"embed"
	"io/fs"
	"strings"
	"time"

	"net/http"
)

// Prefix is the URL path the console is mounted under.
const Prefix = "/dashboard"

//go:embed all:dist
var dist embed.FS

// NewHandler returns the console handler. Hashed assets are served immutable
// from the embedded build; every other path under Prefix gets the SPA shell.
func NewHandler(cfg Config) http.Handler {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic("dashboard: embedded dist is unreadable: " + err.Error())
	}
	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		panic("dashboard: embedded index.html is unreadable: " + err.Error())
	}
	return &handler{
		cfg:    cfg,
		assets: http.StripPrefix(Prefix, http.FileServerFS(sub)),
		index:  index,
	}
}

type handler struct {
	assets http.Handler
	index  []byte
	cfg    Config
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, Prefix+"/api/") {
		h.serveAPI(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, Prefix+"/assets/") {
		// Vite fingerprints every asset name, so the bytes never change.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		h.assets.ServeHTTP(w, r)
		return
	}
	h.serveIndex(w, r)
}

func (h *handler) serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The shell names the hashed assets, so it must not be cached.
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(h.index))
}
