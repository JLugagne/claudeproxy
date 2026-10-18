package proxy

import (
	"strings"

	"net/http"

	"github.com/JLugagne/claudeproxy/internal/proxy/inbound/dashboard"
)

// consoleMux serves the embedded usage console under dashboard.Prefix and
// forwards every other path to the proxy untouched. It is a plain prefix check
// rather than an http.ServeMux so request paths reach the upstream byte for
// byte, with no cleaning or redirect.
type consoleMux struct {
	proxy   http.Handler
	console http.Handler
}

func (m consoleMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == dashboard.Prefix || strings.HasPrefix(r.URL.Path, dashboard.Prefix+"/") {
		if m.console == nil {
			http.NotFound(w, r)
			return
		}
		m.console.ServeHTTP(w, r)
		return
	}
	m.proxy.ServeHTTP(w, r)
}
