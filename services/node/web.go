package node

import (
	"net/http"
	"strings"

	"github.com/Grape-Chain/Grape-Dag/web/runnode"
)

// contentSecurityPolicy - the page's own guarantee, enforced by the browser.
//
// default-src 'none' with connect-src 'self' says the page may talk to the node
// that served it and to nothing else, which is the property that makes it safe
// to put a node's controls in a browser. The two 'unsafe-inline' allowances are
// for the page's own inline style and script; it has no external file to load
// instead, and that is the trade being made.
const contentSecurityPolicy = "default-src 'none'; " +
	"style-src 'unsafe-inline'; " +
	"script-src 'unsafe-inline'; " +
	"connect-src 'self'; " +
	"img-src 'self' data:; " +
	"form-action 'none'; " +
	"base-uri 'none'; " +
	"frame-ancestors 'none'"

// WebHandler - the bundled reference page.
//
// It answers for the mount point itself and for index.html under it, and 404s
// for anything else: there is exactly one asset, so a request for a second one
// is a mistake worth reporting rather than a path to guess at. Matching on the
// suffix rather than an absolute path keeps the handler working wherever it is
// mounted.
func WebHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/") && !strings.HasSuffix(r.URL.Path, "/"+runnode.IndexFile) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		// The page carries a control and a live figure; a cached copy would show
		// somebody a toggle in the wrong position.
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(runnode.Index)
	})
}
