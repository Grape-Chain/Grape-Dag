package dag

import (
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/google/uuid"
)

// withTracker - point the package's confirmation rule at a tracker for the
// duration of one test. Tip selection reads it through tipCache(), so it has to
// be the global one.
func withTracker(t *testing.T, tr confirmations) {
	t.Helper()
	prev := confirmationCounter
	confirmationCounter = tr
	t.Cleanup(func() { confirmationCounter = prev })
}

// oracleWalkRoots - the definition of a walk root, computed the obvious way:
// a tracked site that approves nothing else tracked.
func oracleWalkRoots(tr *ConfirmTracker) map[uuid.UUID]bool {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	out := map[uuid.UUID]bool{}
	for id, st := range tr.sites {
		if st.node == nil {
			continue
		}
		inside := false
		for _, target := range st.node.targets {
			if target == nil {
				continue
			}
			if _, ok := tr.sites[target.id.id]; ok {
				inside = true
				break
			}
		}
		if !inside {
			out[id] = true
		}
	}
	return out
}

func rootIDs(tr *ConfirmTracker) map[uuid.UUID]bool {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	out := map[uuid.UUID]bool{}
	for id := range tr.roots {
		out[id] = true
	}
	return out
}

// The root set is maintained incrementally on every insert, confirmation and
// harvest, so it has to agree with the definition after each one. If it drifts,
// walks start in the wrong place - or, worse, start nowhere and selection falls
// back to a uniform pick without anything saying so.
func TestWalkRootsMatchTheDefinition(t *testing.T) {
	const approveTx = 2
	for _, seed := range []int64{1, 5, 9, 23, 101} {
		seed := seed
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			rng := rand.New(rand.NewSource(seed))
			tr := newConfirmTracker(approveTx, 0)

			genesis := tnode(0)
			nodes := []*Node{genesis}
			tr.add(genesis)

			width := 5
			for step := 1; step <= 200; step++ {
				n := tnode(step)
				if len(nodes) <= width {
					tlink(n, genesis)
				} else {
					tips := tipsOf(nodes, approveTx)
					if len(tips) == 1 {
						tlink(n, tips[0])
					} else {
						i := rng.Intn(len(tips))
						j := rng.Intn(len(tips))
						for j == i {
							j = rng.Intn(len(tips))
						}
						tlink(n, tips[i])
						tlink(n, tips[j])
					}
				}
				nodes = append(nodes, n)
				tr.add(n)

				assertRootsMatch(t, tr, fmt.Sprintf("after insert %d", step))

				// Confirmation removes sites from the region, which promotes the
				// sites that approved them. Draining is where that shows up.
				if step%7 == 0 {
					tr.pop()
					assertRootsMatch(t, tr, fmt.Sprintf("after pop at %d", step))
				}
			}

			// Harvesting is the other way sites leave the region.
			for _, n := range nodes[:40] {
				tr.markHarvested(n.id.id)
				assertRootsMatch(t, tr, "after harvest")
			}
		})
	}
}

func assertRootsMatch(t *testing.T, tr *ConfirmTracker, when string) {
	t.Helper()
	want := oracleWalkRoots(tr)
	got := rootIDs(tr)
	for id := range want {
		if !got[id] {
			t.Fatalf("%s: %s approves nothing in the active region but is not a walk root", when, id.String())
		}
	}
	for id := range got {
		if !want[id] {
			t.Fatalf("%s: %s is a walk root but still approves something in the active region", when, id.String())
		}
	}
}

// Every walk has to finish somewhere a new site may legitimately approve. The
// walk this replaces returned its own starting point, so this is the assertion
// that would have caught it.
func TestWalkEndsOnAnApprovableTip(t *testing.T) {
	const approveTx = 2
	tr := newConfirmTracker(approveTx, 0)
	withTracker(t, tr)
	d := &Dag{}

	genesis := tnode(0)
	nodes := []*Node{genesis}
	tr.add(genesis)
	d.genesis = genesis

	rng := rand.New(rand.NewSource(7))
	for step := 1; step <= 120; step++ {
		n := tnode(step)
		if len(nodes) <= 5 {
			tlink(n, genesis)
		} else {
			tips := tipsOf(nodes, approveTx)
			i := rng.Intn(len(tips))
			tlink(n, tips[i])
			if len(tips) > 1 {
				j := rng.Intn(len(tips))
				for j == i {
					j = rng.Intn(len(tips))
				}
				tlink(n, tips[j])
			}
		}
		nodes = append(nodes, n)
		tr.add(n)
	}

	roots := tr.walkRoots()
	if len(roots) == 0 {
		t.Fatal("the active region has no walk roots, so no walk can start")
	}

	walked := 0
	for trial := 0; trial < 400; trial++ {
		got := d.walkFromRoots(roots, 0.5)
		if got == nil {
			continue
		}
		walked++
		if !tr.isTip(got.id.id) {
			t.Fatalf("walk finished on %s, which cannot be approved", got.id.id.String())
		}
		if len(got.sources) >= approveTx {
			t.Fatalf("walk finished on %s, which already carries %d approver(s)", got.id.id.String(), len(got.sources))
		}
	}
	if walked == 0 {
		t.Fatal("no walk reached a tip")
	}
}

// A tip the walk can never reach is a transaction that can never be approved,
// so it sits unconfirmed until the timeout valve drops it. Every tip has to be
// reachable from some root.
func TestWalkCanReachEveryTip(t *testing.T) {
	const approveTx = 2
	tr := newConfirmTracker(approveTx, 0)
	withTracker(t, tr)
	d := &Dag{}

	genesis := tnode(0)
	tr.add(genesis)
	d.genesis = genesis
	nodes := []*Node{genesis}

	rng := rand.New(rand.NewSource(31))
	for step := 1; step <= 60; step++ {
		n := tnode(step)
		if len(nodes) <= 5 {
			tlink(n, genesis)
		} else {
			tips := tipsOf(nodes, approveTx)
			i := rng.Intn(len(tips))
			tlink(n, tips[i])
			if len(tips) > 1 {
				j := rng.Intn(len(tips))
				for j == i {
					j = rng.Intn(len(tips))
				}
				tlink(n, tips[j])
			}
		}
		nodes = append(nodes, n)
		tr.add(n)
	}

	want := map[uuid.UUID]bool{}
	for _, tip := range tr.getTips() {
		want[tip.id.id] = true
	}
	if len(want) < 2 {
		t.Fatalf("expected the graph to hold several tips, got %d", len(want))
	}

	roots := tr.walkRoots()
	seen := map[uuid.UUID]bool{}
	for trial := 0; trial < 20000 && len(seen) < len(want); trial++ {
		if got := d.walkFromRoots(roots, 0.5); got != nil {
			seen[got.id.id] = true
		}
	}
	for id := range want {
		if !seen[id] {
			t.Fatalf("tip %s was never reached by any walk out of %d root(s)", id.String(), len(roots))
		}
	}
}

// branchedGraph - a root with a well-approved branch and a bare one, plus a
// second root that exists only to keep the first unconfirmed.
//
//	         a2  a3      <- untouched, so they confirm a1 and root
//	           \ /
//	            a1        <- at the approval threshold
//	root -------+
//	            b1        <- untouched, confirms nothing
//
//	other ----- s1        <- a second root, so not every tip confirms root
//
// The second root is not decoration. Every tip in a single-rooted graph confirms
// the root, which is the definition of confirmed: the root would leave the
// active region immediately and there would be nothing to walk from.
func branchedGraph(tr *ConfirmTracker) (root, a1, a2, a3, b1, other *Node) {
	root = tnode(0)
	a1 = tnode(1)
	a2 = tnode(2)
	a3 = tnode(3)
	b1 = tnode(4)
	other = tnode(5)
	s1 := tnode(6)

	tr.add(root)
	tr.add(other)
	tlink(a1, root)
	tr.add(a1)
	tlink(b1, root)
	tr.add(b1)
	tlink(a2, a1)
	tr.add(a2)
	tlink(a3, a1)
	tr.add(a3)
	tlink(s1, other)
	tr.add(s1)
	return
}

// The bias is the whole reason the walk exists: a branch more of the graph
// confirms has to be more likely to be approved. alpha turns it off, which is
// what makes the effect measurable rather than assumed.
func TestWalkPrefersTheBranchMoreOfTheGraphConfirms(t *testing.T) {
	tr := newConfirmTracker(2, 0)
	withTracker(t, tr)
	d := &Dag{}
	root, a1, _, _, b1, _ := branchedGraph(tr)

	// State the potentials the bias is computed from, so a change in what the
	// tracker counts shows up here rather than as a mysteriously weaker bias.
	tr.mu.Lock()
	site, tracked := tr.sites[root.id.id]
	if !tracked {
		tr.mu.Unlock()
		t.Fatal("the root left the active region, so there is nothing to walk from")
	}
	gotRoot := site.count
	gotA1, gotB1 := tr.sites[a1.id.id].count, tr.sites[b1.id.id].count
	tr.mu.Unlock()
	if gotRoot != 3 || gotA1 != 2 || gotB1 != 0 {
		t.Fatalf("expected confirmation counts root=3 a1=2 b1=0, got root=%d a1=%d b1=%d", gotRoot, gotA1, gotB1)
	}

	measure := func(alpha float64) float64 {
		const trials = 4000
		light := 0
		for i := 0; i < trials; i++ {
			got := d.walkToTip(root, alpha)
			if got == nil {
				t.Fatal("walk found nothing to approve in a graph with three candidates")
			}
			if got.id.id == b1.id.id {
				light++
			}
		}
		return float64(light) / trials
	}

	unbiased := measure(0)
	biased := measure(2)

	// With no bias the step out of the root is a coin flip, so the lightly
	// confirmed branch takes about half the walks.
	if unbiased < 0.4 || unbiased > 0.6 {
		t.Fatalf("with alpha=0 the walk should be an even coin flip out of the root; the light branch took %.2f", unbiased)
	}
	// exp(-2*3) against exp(-2*1) is about 1 in 55.
	if biased > 0.1 {
		t.Fatalf("with alpha=2 the walk should mostly avoid the light branch; it took %.2f of walks", biased)
	}
	if biased >= unbiased {
		t.Fatalf("alpha had no effect: light branch took %.2f unbiased and %.2f biased", unbiased, biased)
	}
}

// The sampler is the part that was quietly broken: the ones it replaces
// compared a normal draw against individual weights and fell through to a
// uniform pick on almost every call, so a weighted selection was never actually
// weighted.
func TestWeightedChoiceFollowsItsWeights(t *testing.T) {
	a, b, c := tnode(1), tnode(2), tnode(3)
	nodes := []*Node{a, b, c}
	weights := []float64{1, 3, 6}

	const trials = 60000
	counts := map[uuid.UUID]int{}
	for i := 0; i < trials; i++ {
		got := weightedChoice(nodes, weights)
		if got == nil {
			t.Fatal("weightedChoice returned nothing for a non-empty candidate set")
		}
		counts[got.id.id]++
	}
	for i, want := range []float64{0.1, 0.3, 0.6} {
		got := float64(counts[nodes[i].id.id]) / trials
		if math.Abs(got-want) > 0.02 {
			t.Fatalf("candidate %d has weight %.0f of 10, so it should be picked %.2f of the time; got %.3f",
				i, weights[i], want, got)
		}
	}
}

func TestWeightedChoiceHandlesDegenerateWeights(t *testing.T) {
	a, b := tnode(1), tnode(2)
	nodes := []*Node{a, b}

	if got := weightedChoice(nil, []float64{1}); got != nil {
		t.Fatal("expected nothing to be chosen from an empty candidate set")
	}
	// All weights zero: there is no bias to apply, but a candidate still has to
	// come back, or the walk stalls and the transaction is dropped.
	if got := weightedChoice(nodes, []float64{0, 0}); got == nil {
		t.Fatal("expected a uniform pick when every weight is zero")
	}
	// Underflowed exponentials look exactly like this.
	if got := weightedChoice(nodes, []float64{math.NaN(), math.Inf(1)}); got == nil {
		t.Fatal("expected a candidate even when every weight is unusable")
	}
	// Fewer weights than candidates must not read past the end.
	seen := map[uuid.UUID]bool{}
	for i := 0; i < 200; i++ {
		got := weightedChoice(nodes, []float64{1})
		if got == nil {
			t.Fatal("expected a candidate when the weight slice is short")
		}
		seen[got.id.id] = true
	}
	if seen[b.id.id] {
		t.Fatal("a candidate with no weight of its own was selected")
	}
}

func TestTransitionWeights(t *testing.T) {
	// alpha off means an unweighted walk, not a zero-weight one.
	for _, w := range transitionWeights(5, []int{5, 3, 0}, 0) {
		if w != 1 {
			t.Fatalf("alpha=0 should weight every candidate equally, got %v", w)
		}
	}
	got := transitionWeights(4, []int{4, 3, 1}, 1)
	want := []float64{math.Exp(0), math.Exp(-1), math.Exp(-3)}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-12 {
			t.Fatalf("candidate %d: want %v got %v", i, want[i], got[i])
		}
	}
	// A candidate confirmed by more tips than the site it approves would mean
	// confirmation is not closed downwards. It must not be rewarded for it.
	if w := transitionWeights(1, []int{9}, 1)[0]; w != 1 {
		t.Fatalf("a negative difference should clamp to no difference, got %v", w)
	}
}

// AddTxDag silently drops a site when selection comes back empty, so selection
// must not come back empty while the graph holds anything approvable.
func TestSelectTipsAlwaysOffersSomethingToApprove(t *testing.T) {
	tr := newConfirmTracker(2, 0)
	withTracker(t, tr)
	root, _, _, _, _, _ := branchedGraph(tr)
	d := &Dag{genesis: root}

	tips := d.selectTips(0.5)
	if len(tips) != 2 {
		t.Fatalf("expected two approval targets from a graph with three tips, got %d", len(tips))
	}
	if tips[0].id.id == tips[1].id.id {
		t.Fatal("a site cannot approve the same site twice")
	}
	for _, tip := range tips {
		if !tr.isTip(tip.id.id) {
			t.Fatalf("%s was offered for approval but cannot be approved", tip.id.id.String())
		}
	}
}

func TestSelectTipsFallsBackWhenThereIsNoRegionToWalk(t *testing.T) {
	// The legacy confirmation rule offers no region to walk, so selection has to
	// fall back to a uniform pick rather than return nothing.
	legacy := newConfirmationCounter()
	withTracker(t, legacy)

	genesis := tnode(0)
	a, b := tnode(1), tnode(2)
	legacy.add(genesis)
	tlink(a, genesis)
	legacy.add(a)
	tlink(b, genesis)
	legacy.add(b)

	d := &Dag{genesis: genesis}
	tips := d.selectTips(0.5)
	if len(tips) == 0 {
		t.Fatal("selection returned nothing with the legacy rule, which would drop the transaction")
	}
	if len(tips) == 2 && tips[0].id.id == tips[1].id.id {
		t.Fatal("a site cannot approve the same site twice")
	}
}

func TestSelectTipsOnAnEmptyGraphOffersGenesis(t *testing.T) {
	tr := newConfirmTracker(2, 0)
	withTracker(t, tr)
	genesis := tnode(0)
	d := &Dag{genesis: genesis}

	tips := d.selectTips(0.5)
	if len(tips) != 1 || tips[0].id.id != genesis.id.id {
		t.Fatalf("expected genesis as the only approval target on an empty graph, got %d target(s)", len(tips))
	}
}

// The generator used to be seeded with a constant, so every node on the network
// made the same choices from the same graph and a restart repeated them.
func TestSelectionIsNotSeededWithAConstant(t *testing.T) {
	draw := func() []float64 {
		r := newLockedRand()
		out := make([]float64, 8)
		for i := range out {
			out[i] = r.Float64()
		}
		return out
	}
	a, b := draw(), draw()
	same := true
	for i := range a {
		if a[i] != b[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("two independently created generators produced the same sequence: the seed is constant")
	}
}

func TestDagAlgorithmNormalisesTheConfiguredValue(t *testing.T) {
	prev := dagConfig.Algorithm
	t.Cleanup(func() { dagConfig.Algorithm = prev })
	for _, in := range []string{"MCMC+", " mcmc+ ", "Mcmc+"} {
		dagConfig.Algorithm = in
		if got := dagAlgorithm(); got != DAG_ALGO_MCMCP.Type() {
			t.Fatalf("%q should normalise to %q, got %q", in, DAG_ALGO_MCMCP.Type(), got)
		}
	}
}

// The load-bearing property of the whole selection design: driving the graph
// with real selection has to keep the tip set turning over. Each new site makes
// approvetx approvals and stops being selectable once it has received approvetx,
// so if selection only ever offered untouched sites, every site would collect
// exactly one approval, nothing would reach the threshold, the confirmation
// denominator would grow without bound and the ledger would never confirm
// anything. That is a whole-system failure that no assertion about a single walk
// would catch.
func TestEverySiteCanReachTheApprovalThreshold(t *testing.T) {
	const approveTx = 2
	tr := newConfirmTracker(approveTx, 0)
	withTracker(t, tr)

	genesis := tnode(0)
	d := &Dag{genesis: genesis}
	tr.add(genesis)

	confirmed := 0
	walkMisses := 0
	const steps = 600
	for step := 1; step <= steps; step++ {
		// Watch the walk on its own as well as through selection. selectTips
		// falls back to a uniform pick when a walk finds nothing, and that
		// fallback is generous enough to keep the graph healthy by itself - so
		// without this the test would pass with the walk permanently broken.
		if step > 20 && d.walkFromRoots(tipCache().walkRoots(), 0.5) == nil {
			walkMisses++
		}

		targets := d.selectTips(0.5)
		if len(targets) == 0 {
			t.Fatalf("step %d: selection offered nothing to approve", step)
		}
		n := tnode(step)
		for _, target := range targets {
			tlink(n, target)
		}
		tr.add(n)
		confirmed += len(tr.pop())
	}

	_, tips, _ := tr.stats()

	if walkMisses > steps/10 {
		t.Fatalf("%d of %d walks found nothing to approve; selection is running on its fallback, not on the walk",
			walkMisses, steps-20)
	}

	if confirmed == 0 {
		t.Fatal("nothing was confirmed in 600 inserts: no site ever reached the approval threshold")
	}
	// Each insert supplies approveTx approvals and each site absorbs approveTx,
	// so the frontier should stay a small fraction of the ledger rather than
	// tracking it.
	if tips > 60 {
		t.Fatalf("the tip set grew to %d after 600 inserts: sites are not reaching the approval threshold", tips)
	}
	if confirmed < 300 {
		t.Fatalf("only %d of 600 sites were confirmed; the frontier is not settling", confirmed)
	}
}

// Selection has to be able to hand back two distinct sites even when the
// frontier is narrow, because approving the same site twice is one approval, not
// two, and the graph would stop widening.
func TestSelectTipsResolvesACollisionOnANarrowFrontier(t *testing.T) {
	const approveTx = 2
	tr := newConfirmTracker(approveTx, 0)
	withTracker(t, tr)

	// A chain of saturated sites ending in exactly two selectable ones, so both
	// walks are forced through the same path.
	root := tnode(0)
	tr.add(root)
	other := tnode(99)
	tr.add(other)
	spare := tnode(98)
	tlink(spare, other)
	tr.add(spare)

	mid := tnode(1)
	tlink(mid, root)
	tr.add(mid)
	mid2 := tnode(2)
	tlink(mid2, root)
	tr.add(mid2)

	leftTip := tnode(3)
	tlink(leftTip, mid)
	tr.add(leftTip)
	rightTip := tnode(4)
	tlink(rightTip, mid)
	tr.add(rightTip)

	d := &Dag{genesis: root}
	for trial := 0; trial < 200; trial++ {
		tips := d.selectTips(0.5)
		if len(tips) < 2 {
			continue
		}
		if tips[0].id.id == tips[1].id.id {
			t.Fatalf("trial %d: selection offered the same site twice", trial)
		}
	}
}

// tdetached - a site whose own approval targets are not all known yet. It is
// tracked and it counts towards its targets' approval threshold, but it cannot
// be approved itself: its coverage is not yet meaningful.
func tdetached(n int, parent *Node) *Node {
	d := tnode(n)
	d.missingTargets = map[string]bool{uuid.New().String(): true}
	tlink(d, parent)
	return d
}

// The direct statement of the stopping rule: a site that has been approved once
// but not twice is a legitimate place for a walk to stop, and has to be, or it
// never receives its second approval.
//
// The graph below is arranged so that the only approvable sites are ones that
// already carry an approval - every untouched site in it is detached. A walk that
// stopped only at untouched sites would come back empty every time.
//
//	x(detached) -- m1 --+
//	                    root
//	w(detached) -- m2 --+
//
//	s1 ----------- other      <- a second root, so root stays unconfirmed
func TestWalkStopsAtPartlyApprovedSites(t *testing.T) {
	const approveTx = 2
	tr := newConfirmTracker(approveTx, 0)
	withTracker(t, tr)

	root := tnode(0)
	tr.add(root)
	other := tnode(10)
	tr.add(other)

	m1 := tnode(1)
	tlink(m1, root)
	tr.add(m1)
	m2 := tnode(2)
	tlink(m2, root)
	tr.add(m2)

	tr.add(tdetached(3, m1))
	tr.add(tdetached(4, m2))
	s1 := tnode(11)
	tlink(s1, other)
	tr.add(s1)

	tr.mu.Lock()
	_, rootTracked := tr.sites[root.id.id]
	tr.mu.Unlock()
	if !rootTracked {
		t.Fatal("the root left the active region, so there is nothing to walk from")
	}
	if !tr.isTip(m1.id.id) || !tr.isTip(m2.id.id) {
		t.Fatal("expected both partly approved sites to be approvable")
	}

	d := &Dag{genesis: root}
	found := map[uuid.UUID]int{}
	for trial := 0; trial < 500; trial++ {
		got := d.walkToTip(root, 0.5)
		if got == nil {
			t.Fatal("the walk found nothing to approve, though two sites are waiting for a second approval")
		}
		if got.id.id != m1.id.id && got.id.id != m2.id.id {
			t.Fatalf("the walk stopped on %s, which is not one of the two approvable sites", got.id.id.String())
		}
		found[got.id.id]++
	}
	if found[m1.id.id] == 0 || found[m2.id.id] == 0 {
		t.Fatalf("the walk only ever reached one of the two approvable sites: %v", found)
	}
}

// Both approvals a new site makes have to come from a walk, not just the first.
// Selection resolves a collision between the two walks by taking any other tip,
// and that path is uniform - so if the second walk's result were dropped, half
// of every site's approvals would be unbiased and the lightly confirmed branch
// would get an approval it should not have. The bias would be half as strong
// with nothing failing.
func TestBothApprovalsAreBiased(t *testing.T) {
	tr := newConfirmTracker(2, 0)
	withTracker(t, tr)
	root, _, _, _, b1, _ := branchedGraph(tr)
	d := &Dag{genesis: root}

	const trials = 4000
	light := 0
	for i := 0; i < trials; i++ {
		for _, tip := range d.selectTips(2) {
			if tip != nil && tip.id.id == b1.id.id {
				light++
			}
		}
	}
	share := float64(light) / trials
	// A biased walk reaches the light branch in well under one walk in fifty; a
	// uniform pick among the tips would reach it in roughly one in four.
	if share > 0.08 {
		t.Fatalf("the lightly confirmed site was approved in %.3f of selections; at alpha=2 both approvals should avoid it", share)
	}
}

// A site leaves the active region when it is confirmed or written into a commit
// transaction, and the sites that approved it may then approve nothing else
// inside the region - which makes them walk roots. If that promotion is missed,
// the region grows a part no walk can enter, and the sites in it are never
// offered for approval.
func TestHarvestingASitePromotesItsApprovers(t *testing.T) {
	tr := newConfirmTracker(2, 0)
	withTracker(t, tr)

	root := tnode(0)
	tr.add(root)
	approver := tnode(1)
	tlink(approver, root)
	tr.add(approver)

	// A second root, so root is not confirmed the moment it is approved.
	other := tnode(10)
	tr.add(other)
	s1 := tnode(11)
	tlink(s1, other)
	tr.add(s1)

	if rootIDs(tr)[approver.id.id] {
		t.Fatal("a site that approves something still in the region is not a walk root")
	}

	tr.markHarvested(root.id.id)

	if !rootIDs(tr)[approver.id.id] {
		t.Fatal("the site it approved was harvested, so it approves nothing in the region and should be a walk root")
	}
	assertRootsMatch(t, tr, "after harvesting an approved site")
}

// The mirror case: a site that arrived before the sites it approves was an entry
// point only because those sites were missing. Once they are resolved it is not
// one any more, and leaving it in the set starts walks above part of the graph.
func TestResolvingADetachedSiteStopsItBeingARoot(t *testing.T) {
	tr := newConfirmTracker(2, 0)
	withTracker(t, tr)

	root := tnode(0)
	tr.add(root)

	// Arrived naming approval targets this node has never seen.
	late := tnode(1)
	late.missingTargets = map[string]bool{uuid.New().String(): true}
	tr.add(late)

	if !rootIDs(tr)[late.id.id] {
		t.Fatal("a site with no resolved targets approves nothing in the region, so it is a walk root")
	}
	assertRootsMatch(t, tr, "while detached")

	// The gap is filled and it is relinked.
	late.missingTargets = nil
	tlink(late, root)
	tr.relink(late)

	if rootIDs(tr)[late.id.id] {
		t.Fatal("its targets were resolved, so it now approves something in the region and is not a walk root")
	}
	assertRootsMatch(t, tr, "after relinking")
}

// The bias must survive a large frontier. The difference in the exponent is a
// count of tips, so on a busy graph it is a big number, and an unnormalised
// exp(-alpha*d) underflows for every candidate at once - which is a uniform pick
// wearing the clothes of a weighted one.
func TestTransitionWeightsSurviveALargeFrontier(t *testing.T) {
	const alpha = 0.5
	w := transitionWeights(20000, []int{9000, 8000}, alpha)
	if w[0] <= 0 || w[1] <= 0 {
		t.Fatalf("both candidates underflowed to zero on a large frontier: %v", w)
	}
	// The nearer candidate is 1000 tips ahead, so it should dominate by
	// exp(0.5*1000) - which is to say, completely - without either weight being
	// zero.
	if w[0] <= w[1] {
		t.Fatalf("the better-confirmed candidate should weigh more: %v", w)
	}

	// Only the ratios matter, so subtracting the shared offset must not change
	// any probability.
	near := transitionWeights(10, []int{5, 4}, alpha)
	far := transitionWeights(1010, []int{1005, 1004}, alpha)
	if math.Abs(near[1]/near[0]-far[1]/far[0]) > 1e-12 {
		t.Fatalf("the same pair of differences gave different odds at different offsets: %v vs %v", near, far)
	}
}
