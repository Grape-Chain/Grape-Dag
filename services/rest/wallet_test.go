package rest

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeWalletFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>wallet</h1>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "wallet.wasm"), []byte("\x00asm-not-really"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func get(t *testing.T, h http.Handler, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestWalletAssetsPresent(t *testing.T) {
	dir := writeWalletFixture(t)
	index, wasm := walletAssetsPresent(dir)
	if !index || !wasm {
		t.Fatalf("assets not detected: index=%v wasm=%v", index, wasm)
	}

	empty := t.TempDir()
	index, wasm = walletAssetsPresent(empty)
	if index || wasm {
		t.Fatalf("assets reported for an empty dir: index=%v wasm=%v", index, wasm)
	}
}

func TestWalletServesIndex(t *testing.T) {
	h := walletHandler(writeWalletFixture(t))
	rec := get(t, h, "/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "wallet") {
		t.Fatalf("body = %q, want the index content", rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store (the page handles private keys)", got)
	}
}

// instantiateStreaming refuses a wasm module that is not served as
// application/wasm, so this content type is load-bearing.
func TestWalletServesWasmWithTheRequiredContentType(t *testing.T) {
	h := walletHandler(writeWalletFixture(t))
	rec := get(t, h, "/wallet.wasm", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/wasm" {
		t.Fatalf("Content-Type = %q, want application/wasm", got)
	}
	if rec.Header().Get("Content-Encoding") != "" {
		t.Fatalf("served compressed without the client asking for it")
	}
}

func TestWalletPrefersPrecompressedWasm(t *testing.T) {
	dir := writeWalletFixture(t)
	// a .gz alongside the module is what `make wallet` produces
	raw := []byte("\x00asm-compressed-payload")
	f, err := os.Create(filepath.Join(dir, "wallet.wasm.gz"))
	if err != nil {
		t.Fatal(err)
	}
	zw := gzip.NewWriter(f)
	if _, err := zw.Write(raw); err != nil {
		t.Fatal(err)
	}
	zw.Close()
	f.Close()

	h := walletHandler(dir)
	rec := get(t, h, "/wallet.wasm", map[string]string{"Accept-Encoding": "gzip, deflate, br"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/wasm" {
		t.Fatalf("Content-Type = %q, want application/wasm even when compressed", got)
	}
	if got := rec.Header().Get("Vary"); got != "Accept-Encoding" {
		t.Errorf("Vary = %q, want Accept-Encoding", got)
	}
	// and the body must really be the gzip stream
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("body is not gzip: %s", err.Error())
	}
	got, _ := io.ReadAll(zr)
	if string(got) != string(raw) {
		t.Fatalf("decompressed body = %q, want %q", got, raw)
	}
}

func TestWalletFallsBackToUncompressedWasm(t *testing.T) {
	// no .gz present, but the client accepts gzip
	h := walletHandler(writeWalletFixture(t))
	rec := get(t, h, "/wallet.wasm", map[string]string{"Accept-Encoding": "gzip"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Header().Get("Content-Encoding") == "gzip" {
		t.Fatalf("claimed gzip encoding with no .gz file present")
	}
}

func TestWalletRejectsPathTraversal(t *testing.T) {
	dir := writeWalletFixture(t)
	secret := filepath.Join(filepath.Dir(dir), "secret.txt")
	if err := os.WriteFile(secret, []byte("private key material"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := walletHandler(dir)
	for _, path := range []string{
		"/../secret.txt",
		"/..%2Fsecret.txt",
		"/subdir/../../secret.txt",
	} {
		rec := get(t, h, path, nil)
		if strings.Contains(rec.Body.String(), "private key material") {
			t.Fatalf("%s escaped the wallet directory", path)
		}
	}
}

func TestAcceptsGzip(t *testing.T) {
	cases := map[string]bool{
		"":                    false,
		"gzip":                true,
		"GZIP":                true,
		" gzip ":              true,
		"gzip;q=1.0, deflate": true,
		"deflate, gzip;q=0.8": true,
		"br":                  false,
		"deflate":             false,
		"x-gzip":              false,
		"notgzip":             false,
	}
	for header, want := range cases {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if header != "" {
			req.Header.Set("Accept-Encoding", header)
		}
		if got := acceptsGzip(req); got != want {
			t.Errorf("acceptsGzip(%q) = %v, want %v", header, got, want)
		}
	}
}
