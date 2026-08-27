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
