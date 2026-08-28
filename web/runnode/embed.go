// Package runnode holds the bundled reference page for turning a machine into a
// processing node, and is the package that embeds it.
//
// It exists as a package only because Go's embed directive cannot see outside
// the directory of the package that declares it, and these assets belong beside
// the other web assets rather than inside a Go service package. The handler that
// serves them lives in services/node.
package runnode

import (
	"embed"
	"fmt"
)

// IndexFile - the only asset. The page is deliberately one self-contained file:
// no build step, no framework, and nothing fetched from anywhere else.
const IndexFile = "index.html"

//go:embed index.html
var Assets embed.FS

// Index - the page itself, read once at start-up. It is a fixed few kilobytes
// and there is no reason for every request to re-read it.
var Index = mustRead(IndexFile)

// mustRead - a missing embedded asset is a build-time mistake, not a runtime
// condition a node should try to carry on with.
func mustRead(name string) []byte {
	b, err := Assets.ReadFile(name)
	if err != nil {
		panic(fmt.Errorf("embedded asset %s is missing: %w", name, err))
	}
	return b
}
