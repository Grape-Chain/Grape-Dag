package rest

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Grape-Chain/Grape-Dag/config"
)

// The bundled testnet web wallet. The assets are served from disk rather than
// embedded because wallet.wasm is ~25 MB - embedding it would inflate every
// grapepeer binary, including nodes that never serve a UI. When the directory is
// absent the route is simply not registered.
//
// Build the assets with: make wallet

// walletDir - where the wallet assets live. Absolute paths are used as given;
// relative paths resolve against the working directory.
func walletDir() string {
	dir := config.GetConfig().Peer.Walletdir
	if dir == "" {
		dir = DEFAULT_WALLET_DIR
	}
	if abs, err := filepath.Abs(dir); err == nil {
		return abs
	}
	return dir
}

// DEFAULT_WALLET_DIR - default location of the web wallet assets
const DEFAULT_WALLET_DIR = "web/wallet"

// walletAssetsPresent - true when the directory holds at least an index.html.
// The wasm module is reported separately so an operator gets a precise message
// rather than a blank page.
func walletAssetsPresent(dir string) (index bool, wasm bool) {
	if st, err := os.Stat(filepath.Join(dir, "index.html")); err == nil && !st.IsDir() {
		index = true
	}
	if st, err := os.Stat(filepath.Join(dir, "wallet.wasm")); err == nil && !st.IsDir() {
		wasm = true
	}
	return index, wasm
}

// walletHandler - static file handler for the wallet, rooted at dir.
//
// It differs from a plain http.FileServer in two ways that matter to the
// browser: .wasm is always served as application/wasm (instantiateStreaming
// refuses anything else), and a pre-compressed wallet.wasm.gz is preferred when
// the client accepts gzip, which takes the transfer from ~25 MB to ~5 MB.
func walletHandler(dir string) http.Handler {
	fileServer := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// keys live in the browser: make sure an intermediary cannot retain the page
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// this UI is self-contained; deny framing and outbound requests it does not need
		w.Header().Set("X-Frame-Options", "DENY")

		upath := strings.TrimPrefix(r.URL.Path, "/")
		if upath == "" {
			upath = "index.html"
		}
		clean := filepath.Clean(filepath.FromSlash(upath))
		if strings.HasPrefix(clean, "..") {
			http.NotFound(w, r)
			return
		}

		if strings.HasSuffix(clean, ".wasm") {
			w.Header().Set("Content-Type", "application/wasm")
			gz := filepath.Join(dir, clean+".gz")
			if acceptsGzip(r) {
				if st, err := os.Stat(gz); err == nil && !st.IsDir() {
					w.Header().Set("Content-Encoding", "gzip")
					w.Header().Set("Vary", "Accept-Encoding")
					http.ServeFile(w, r, gz)
					return
				}
			}
		}
		fileServer.ServeHTTP(w, r)
	})
}

func acceptsGzip(r *http.Request) bool {
	for _, enc := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		if strings.EqualFold(strings.TrimSpace(strings.SplitN(enc, ";", 2)[0]), "gzip") {
			return true
		}
	}
	return false
}
