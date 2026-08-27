package dag

import (
	crand "crypto/rand"
	"encoding/binary"
	"math"
	"math/rand"
	"sync"

	"github.com/Grape-Chain/Grape-Dag/utils"
)

/*
Tip selection: which existing sites a new site approves.

The point of selecting by random walk rather than uniformly is that the choice
should follow where the graph already agrees. A walk that starts below the tips
and steps towards them, preferring at each step the branch more of the graph
confirms, leaves a site that nobody else built on unlikely to be approved - so
flooding the graph with such sites buys an attacker nothing, and a side chain
built in private stays unapproved when it is finally published. A uniform pick
among tips gives all of that away: every tip is equally attractive no matter how
much or how little of the graph stands behind it.

What was here before did not walk. AddTxDag passed the tip set in as the walk's
starting point, and the loop condition is "keep stepping until the particle is a
tip", so it was satisfied before the first step every single time. Every
algorithm setting therefore reduced to a uniform random pick among tips, and the
three weighting functions, the alpha coefficient and the cumulative weights were
all dead weight - computed, then either never consulted or consulted in a form
that could not select anything (see weightedChoice below). The walk also drew
from a generator seeded with a constant, so every node on the network made the
same sequence of choices and a restart repeated it.

Three things are fixed here:

  - The walk starts at the root frontier of the active region - the sites still
    open that approve nothing else still open - and steps forwards along
    approvals to the first site with room for another approval. The region is a
    finite DAG, so every such site is reachable from some root, and the region is
    bounded by confirmation rather than by the size of the ledger.

  - The step probability is exp(-alpha * (W(from) - W(to))), where W is the
    number of the graph's current tips that confirm a site: the very quantity
    the section 5.1 confirmation rule measures. Because a tip that confirms a
    site also confirms its ancestors, W does not increase as the walk moves
    towards the tips, so the exponent is a non-negative difference and each
    weight lands in (0,1]. The form that was here put an absolute cumulative
    weight in the exponent, and cumulative weight roughly doubles per level, so
    exp(-alpha*w) underflowed to zero for every candidate within a few dozen
    sites of genesis - after which nothing could be distinguished from anything
    else.

  - The generator is seeded from crypto/rand.

Own weight is deliberately not in the exponent yet. A site's txWeight is a
random normal draw (see genRandomTxWeight), so it measures nothing; making it
mean something - work done, or fee paid - belongs with the fee work, and the
paper's own-weight term is worth adding only once there is a benchmark to show
what it costs. Until then "mcmc+" and "mcmc++" select identically, which is
stated at startup rather than left for someone to discover.
*/

// walkStepBudget - how many hops one walk may take before it approves wherever
// it stands. The active region is bounded by confirmation, so a walk to a tip is
// short; this is a guard against a graph that should not exist (a cycle a peer
// managed to introduce), not a normal exit.
const walkStepBudget = 4096

// walkCollisionBudget - how many times selection re-walks when the second walk
// lands on the site the first one chose. A new site approving the same site
// twice is not two approvals, so a collision has to be resolved or dropped.
const walkCollisionBudget = 8

// dagRand - the source for every selection decision, seeded from crypto/rand
// and guarded because selection is not its only caller.
var dagRand = newLockedRand()

type lockedRand struct {
	mu sync.Mutex
	r  *rand.Rand
}

func newLockedRand() *lockedRand {
	var seed [8]byte
	if _, err := crand.Read(seed[:]); err != nil {
		// Not observed in practice. math/rand's own global is seeded randomly,
		// so fall through to it rather than back to a constant.
		return &lockedRand{r: rand.New(rand.NewSource(rand.Int63()))}
	}
	return &lockedRand{r: rand.New(rand.NewSource(int64(binary.LittleEndian.Uint64(seed[:]))))}
}

func (l *lockedRand) Float64() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.r.Float64()
}

func (l *lockedRand) Intn(n int) int {
	if n <= 1 {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.r.Intn(n)
}

// weightedChoice - pick one node with probability proportional to its weight:
// one cumulative sum, one uniform draw.
//
// The three samplers this replaces could not do that. Each drew from a normal
// distribution and then compared the draw against individual weights rather
// than against a running total - "if r >= x && r <= v" tests the draw against
// one weight, not against an interval - and one of them assigned the running
// sum instead of adding to it. Most draws matched nothing and fell through to
// the uniform pick at the bottom of each function, which is what actually
// selected.
func weightedChoice(nodes []*Node, weights []float64) *Node {
	n := len(nodes)
	if n == 0 {
		return nil
	}
	if len(weights) < n {
		n = len(weights)
	}
	total := 0.0
	for i := 0; i < n; i++ {
		if w := weights[i]; w > 0 && !math.IsInf(w, 0) && !math.IsNaN(w) {
			total += w
		}
	}
	if total <= 0 {
		// Every candidate weighed nothing: there is no bias to apply, so do not
		// pretend to one.
		return nodes[dagRand.Intn(len(nodes))]
	}
	u := dagRand.Float64() * total
	acc := 0.0
	for i := 0; i < n; i++ {
		w := weights[i]
		if w <= 0 || math.IsInf(w, 0) || math.IsNaN(w) {
			continue
		}
		acc += w
		if u < acc {
			return nodes[i]
		}
	}
	// Only reachable through floating-point rounding on the last comparison.
	return nodes[n-1]
}

// transitionWeights - exp(-alpha * (potential - nextPotential)) per candidate,
// clamped at a non-negative exponent. alpha <= 0 means an unweighted walk, which
// is the honest reading of "no bias" and is what the distribution tests compare
// against.
//
// The smallest difference is subtracted from all of them first. Only the ratios
// between the weights matter - weightedChoice normalises by their total - so
// this leaves every probability exactly where it was, and it keeps the largest
// weight at 1 instead of letting the whole set slide towards zero. Without it
// the bias would quietly disappear at scale: the difference is a count of tips,
// so on a frontier of a few thousand a shared offset of 1416 is enough for
// exp(-0.5*d) to underflow for every candidate at once, and a set of all-zero
// weights is a uniform pick. That is the same failure the old absolute-weight
// form had, and it would have come back at exactly the throughput this is
// being built for.
func transitionWeights(potential int, nextPotential []int, alpha float64) []float64 {
	w := make([]float64, len(nextPotential))
	if alpha <= 0 || len(nextPotential) == 0 {
		for i := range w {
			w[i] = 1
		}
		return w
	}
	diffs := make([]float64, len(nextPotential))
	least := math.Inf(1)
	for i, p := range nextPotential {
		d := float64(potential - p)
		if d < 0 {
			// A candidate confirmed by more tips than the site it approves would
			// mean confirmation is not closed downwards. Treat it as no
			// difference rather than as a reward.
			d = 0
		}
		diffs[i] = d
		if d < least {
			least = d
		}
	}
	for i, d := range diffs {
		w[i] = math.Exp(-alpha * (d - least))
	}
	return w
}

// walkToTip - step a particle from a root of the active region to the first site
// along its path that still has room to be approved. Returns nil when this walk
// found nowhere to finish, so the caller can start another one.
//
// "Room to be approved" is the stopping rule, rather than "nothing approves it
// at all", and the difference is not cosmetic. A site stops being selectable
// here once approvetx other sites reference it, and every new site makes
// approvetx approvals - so supply and demand balance only if each site is
// approved exactly approvetx times. A walk that stopped only at untouched sites
// would give every site its first approval and never its second: nothing would
// ever reach the threshold, no tip would ever retire, the denominator would grow
// without bound and nothing would ever be confirmed. TestEverySiteCanReachTheApprovalThreshold
// is that failure, written down.
//
// Stopping early is not a concern, because the sites the walk passes through on
// the way are the ones already at the threshold: that is why they are not
// stopping points. A partly approved site deeper in the region is a stopping
// point, and should be - it is waiting for exactly the approval this walk is
// about to make.
func (dag *Dag) walkToTip(start *Node, alpha float64) *Node {
	particle := start
	for steps := 0; steps < walkStepBudget; steps++ {
		if particle == nil {
			return nil
		}
		selectable, potential, next, nextPotential := tipCache().walkFrom(particle)
		if selectable {
			return particle
		}
		if len(next) == 0 {
			// At the approval threshold, yet nothing in the region approves it.
			// A site inside the region has all of its approvers inside it too -
			// confirmation is closed downwards, so an approver cannot be
			// confirmed and leave while the site it approves is still open - so
			// this is a site the region no longer holds, or a detached one
			// waiting on targets it has never seen. Abandon the walk rather than
			// approve it.
			return nil
		}
		step := weightedChoice(next, transitionWeights(potential, nextPotential, alpha))
		if step == nil {
			return nil
		}
		particle = step
	}
	logger.Warnf("[tipselect] A walk used its whole %d-step budget without reaching a tip; approving %s where it stands",
		walkStepBudget, particle.id.id.String())
	return particle
}

// selectTips - the sites a new site will approve: two independent walks, or one
// site when the graph offers only one.
//
// Never returns an empty slice while the graph holds anything approvable.
// AddTxDag drops the site on the floor when selection comes back empty (its
// approval gate fails, and the site is silently not added), so the fallbacks
// below matter: a walk that finds nothing must not cost a transaction.
func (dag *Dag) selectTips(alpha float64) []*Node {
	roots := tipCache().walkRoots()

	first := dag.walkFromRoots(roots, alpha)
	if first == nil {
		// Either there is no active region to walk (the legacy confirmation
		// rule keeps none) or every root led nowhere. Fall back to the uniform
		// pick that selection used to do.
		return dag.uniformTips()
	}

	for attempt := 0; attempt < walkCollisionBudget; attempt++ {
		second := dag.walkFromRoots(roots, alpha)
		if second == nil {
			break
		}
		if second.id.id != first.id.id {
			return []*Node{first, second}
		}
	}

	// Both walks keep landing in the same place. Take any other tip rather than
	// approve one site twice; a single approval is valid, so this is a last
	// resort and not an error.
	if other := dag.anyTipExcept(first); other != nil {
		return []*Node{first, other}
	}
	return []*Node{first}
}

// walkFromRoots - one walk, from a uniformly chosen root. Every root is tried
// before giving up, because a root that is detached and unapproved is a dead
// end and there is no reason to let it stall selection.
func (dag *Dag) walkFromRoots(roots []*Node, alpha float64) *Node {
	if len(roots) == 0 {
		return nil
	}
	start := dagRand.Intn(len(roots))
	for i := 0; i < len(roots); i++ {
		if tip := dag.walkToTip(roots[(start+i)%len(roots)], alpha); tip != nil {
			return tip
		}
	}
	return nil
}

// uniformTips - up to two distinct tips, chosen uniformly. The fallback when
// there is no region to walk, and what the "random" algorithm setting means.
func (dag *Dag) uniformTips() []*Node {
	tips := tipCache().getTips()
	if len(tips) == 0 {
		if g := dag.getGenesis(); g != nil {
			return []*Node{g}
		}
		return nil
	}
	if len(tips) == 1 {
		return []*Node{tips[0]}
	}
	i := dagRand.Intn(len(tips))
	j := dagRand.Intn(len(tips) - 1)
	if j >= i {
		j++
	}
	return []*Node{tips[i], tips[j]}
}

// anyTipExcept - a tip other than the given one, if the graph has one.
func (dag *Dag) anyTipExcept(not *Node) *Node {
	tips := tipCache().getTips()
	if len(tips) == 0 {
		return nil
	}
	start := dagRand.Intn(len(tips))
	for i := 0; i < len(tips); i++ {
		t := tips[(start+i)%len(tips)]
		if t != nil && (not == nil || t.id.id != not.id.id) {
			return t
		}
	}
	return nil
}

// logTipSelection - say at startup what selection is actually doing, including
// where the configured algorithm does less than its name suggests. The previous
// arrangement accepted three algorithm names and gave identical behaviour for
// all three without saying so.
func logTipSelection() {
	switch dagAlgorithm() {
	case DAG_ALGO_RANDOM.Type():
		logger.Warnf("[tipselect] algorithm=%s: approval targets are picked uniformly from the tips, with no bias towards well-confirmed branches",
			DAG_ALGO_RANDOM.Type())
	case DAG_ALGO_MCMCP.Type(), DAG_ALGO_MCMCPP.Type():
		utils.ColorizeInfo(logger, "[tipselect] algorithm=%s: weighted random walk over the unconfirmed region, alpha=%.3f",
			dagAlgorithm(), dagConfig.Alpha)
		if dagAlgorithm() == DAG_ALGO_MCMCPP.Type() {
			logger.Warnf("[tipselect] %s currently selects identically to %s: the own-weight term needs a site weight that measures something, and txWeight is a random draw",
				DAG_ALGO_MCMCPP.Type(), DAG_ALGO_MCMCP.Type())
		}
	default:
		logger.Warnf("[tipselect] algorithm=%q is not one of %s, %s or %s; falling back to the weighted walk with alpha=%.3f",
			dagConfig.Algorithm, DAG_ALGO_MCMCP.Type(), DAG_ALGO_MCMCPP.Type(), DAG_ALGO_RANDOM.Type(), dagConfig.Alpha)
	}
}
