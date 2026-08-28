package node

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/Grape-Chain/Grape-Dag/web/runnode"
)

func TestTheBundledPageIsServedAtTheMountPoint(t *testing.T) {
	svc := NewService(&fakeLedger{})
	for _, path := range []string{WebPath, WebPath + runnode.IndexFile} {
		rec := httptest.NewRecorder()
		svc.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200", path, rec.Code)
		}
		if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
			t.Errorf("GET %s Content-Type = %q", path, got)
		}
		if !strings.Contains(rec.Body.String(), "Run a node") {
			t.Errorf("GET %s did not return the page", path)
		}
	}
}

func TestAnAssetThePageDoesNotHaveIsNotFound(t *testing.T) {
	svc := NewService(&fakeLedger{})
	rec := httptest.NewRecorder()
	svc.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, WebPath+"secrets.txt", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET %ssecrets.txt = %d, want 404", WebPath, rec.Code)
	}
}

// external - anything that would take the page off this node: an absolute URL,
// a protocol-relative one, or an import from somewhere else.
var external = regexp.MustCompile(`(?i)(https?:)?//[a-z0-9.-]+\.[a-z]{2,}`)

func TestTheBundledPageMakesNoExternalRequests(t *testing.T) {
	page := string(runnode.Index)
	for _, line := range strings.Split(page, "\n") {
		// The xmlns-style comment prose in the header is not a request; only
		// attributes and script sources can be one.
		if !strings.Contains(line, "src=") && !strings.Contains(line, "href=") &&
			!strings.Contains(line, "import ") && !strings.Contains(line, "fetch(") {
			continue
		}
		if match := external.FindString(line); match != "" {
			t.Errorf("the page reaches outside the node: %s\n  in: %s", match, strings.TrimSpace(line))
		}
	}
	for _, forbidden := range []string{"cdn.", "googleapis", "unpkg", "jsdelivr", "<link rel=\"stylesheet\""} {
		if strings.Contains(page, forbidden) {
			t.Errorf("the page contains %q, which means it depends on something it does not ship", forbidden)
		}
	}
}

func TestTheBundledPageUsesTheSamePathsTheServiceMounts(t *testing.T) {
	// The page fetches relative paths, so a rename of a route constant has to
	// show up here rather than as a blank panel in somebody's browser.
	page := string(runnode.Index)
	for _, path := range []string{StatusPath, EarningsPath, ProcessingPath} {
		leaf := path[strings.LastIndex(path, "/")+1:]
		if !strings.Contains(page, `"`+leaf+`"`) {
			t.Errorf("the page does not fetch %q, the leaf of %s", leaf, path)
		}
	}
}

func TestTheBundledPageIsServedWithAPolicyThatForbidsLeavingTheNode(t *testing.T) {
	rec := httptest.NewRecorder()
	WebHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, WebPath, nil))
	csp := rec.Header().Get("Content-Security-Policy")
	for _, want := range []string{"default-src 'none'", "connect-src 'self'", "frame-ancestors 'none'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("Content-Security-Policy = %q, want it to contain %q", csp, want)
		}
	}
}
