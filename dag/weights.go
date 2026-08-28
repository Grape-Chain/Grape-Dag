package dag

import (
	"strings"

	"github.com/ledongthuc/goterators"
)

// dagAlgorithm - the configured tip-selection algorithm, normalised. The two
// dispatch tables this replaces disagreed on whether to lowercase the config
// value, so "MCMC+" picked one code path for the candidate weights and a
// different one for the step - and a value that matched neither table selected
// nothing at all.
func dagAlgorithm() string {
	return strings.ToLower(strings.TrimSpace(dagConfig.Algorithm))
}

// updateCumWeights - recompute cumulative weights over the whole node slice.
//
// NOT called on the insert path any more, and that is the point of this comment.
//
// It walked every site in the live graph on every insert, doing an atomic load,
// a closure-based reduce and an atomic store per site, which made it the
// largest single consumer of CPU in the node: 15.95% cumulative in a profile of
// a node under sustained load, ahead of signature verification.
//
// What it maintains is read by nothing. Tip selection was rewritten to weight
// its walk by confirmation count (see dag/walk.go) and does not consult
// cumWeight at all. The remaining readers are the SiteTips traversals in
// dag/search.go and the vertex conversions in dag/traverse.go, and neither
// cluster has a caller anywhere in the repository - SiteTips.cw is written and
// never read even within search.go. The field is still serialised on pb.Node
// for wire compatibility, and nothing verifies it: no node rebuilds a received
// commit transaction's sites to compare them, and the site attribution
// signature deliberately excludes it precisely because it is recomputed locally.
//
// Kept, rather than deleted, so that reviving a reader is a matter of calling it
// where the value is wanted instead of reimplementing it. Note before doing so
// that the recurrence sums over sources, which counts paths rather than distinct
// descendants and therefore grows exponentially in a wide graph - it is a
// measure that would want rethinking, not just rescheduling.
//
// Only "mcmc+" asks for this, and nothing consults the result any more: tip
// selection now measures how much of the graph confirms a site (see walk.go),
// which the confirmation tracker already maintains incrementally, rather than a
// cumulative weight recomputed over every site on every insert. What is left of
// this function is the value gossiped in Node.cumWeight and drawn on the
// visualisation, and it is O(sites) per insert, so it stays behind the
// algorithm check until the benchmark says what it costs.
//
// Note the required ordering: a node's weight is the sum of the weights of the
// nodes that approve it, so approvers have to be computed first. The node slice
// is in insertion order, oldest first, so this walks it backwards. Callers that
// reverse the slice first (see insert.go and generate.go) walk it oldest-first
// instead and read stale weights for every node with more than one approver.
func updateCumWeights(nodes []*Node, links []Link) []*Node {
	for i := len(nodes) - 1; i >= 0; i-- {
		nodes[i].cumWeight.Store(goterators.Reduce(nodes[i].sources, 0, func(prev float64, cur *Node, idx int, l []*Node) float64 {
			return prev + cur.cumWeight.Load()
		}) + float64(dagConfig.Cummstep))
	}

	return nodes
}
