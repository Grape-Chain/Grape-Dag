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

// withSeededSelection - make every selection decision in one test reproducible.
//
// The tests that measure a distribution - how often the walk takes the lightly
// confirmed branch, how evenly the tip set is sampled - are sampling, and their
// assertions are about a rate. Left on the production source, seeded from
// crypto/rand, each of them fails at whatever rate its margin allows and passes
// the rest of the time, which is indistinguishable from a real regression that
// happens to be intermittent. Seeded, the run is the same every time: the
// assertion either holds for these seeds or it does not, and a change that
// weakens the bias fails immediately rather than eventually.
//
// Several seeds per test, because one seed proves the property for one sample.
func withSeededSelection(t *testing.T, seed int64) {
	t.Helper()
	prev := dagRand
	dagRand = newSeededRand(seed)
	t.Cleanup(func() { dagRand = prev })
}

// selectionSeeds - the seeds every sampling test runs over. Arbitrary, fixed,
// and more than one.
var selectionSeeds = []int64{1, 20260828, 777}

// deepWalk - a walk from as far below the tips as the region reaches. Throwing
// the particle with an effectively unlimited depth stops it at the region floor,
// which is the deepest a walk can start, so this exercises the longest forward
// path the graph allows.
func deepWalk(d *Dag, alpha float64) *Node {
	start := d.walkStart(1 << 20)
	if start == nil {
		return nil
	}
	return d.walkToTip(start, alpha)
}

// Every walk has to finish somewhere a new site may legitimately approve. The
// walk this replaces returned its own starting point, so this is the assertion
// that would have caught it.
func TestWalkEndsOnAnApprovableTip(t *testing.T) {
	const approveTx = 2
	tr := newConfirmTracker(0, 1000)
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
			tips := tipsOf(nodes)
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

	walked := 0
	for trial := 0; trial < 400; trial++ {
		got := deepWalk(d, 0.5)
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
	tr := newConfirmTracker(0, 1000)
	withTracker(t, tr)
	d := &Dag{}

	genesis := tnode(0)
	tr.add(genesis)
	d.genesis = genesis
	nodes := []*Node{genesis}

	// Built in batches that each choose against the same view, because a tip is
	// a site nothing approves: insert one at a time and the graph holds exactly
	// one tip, which is not a graph that can test reachability.
	rng := rand.New(rand.NewSource(31))
	step := 1
	for round := 0; round < 20; round++ {
		tips := tipsOf(nodes)
		batch := []*Node{}
		for k := 0; k < 4; k++ {
			n := tnode(step)
			step++
			i := rng.Intn(len(tips))
			tlink(n, tips[i])
			if len(tips) > 1 {
				j := rng.Intn(len(tips))
				for j == i {
					j = rng.Intn(len(tips))
				}
				tlink(n, tips[j])
			}
			batch = append(batch, n)
		}
		for _, n := range batch {
			nodes = append(nodes, n)
			tr.add(n)
		}
	}

	want := map[uuid.UUID]bool{}
	for _, tip := range tr.getTips() {
		want[tip.id.id] = true
	}
	if len(want) < 2 {
		t.Fatalf("expected the graph to hold several tips, got %d", len(want))
	}

	seen := map[uuid.UUID]bool{}
	for trial := 0; trial < 20000 && len(seen) < len(want); trial++ {
		if got := deepWalk(d, 0.5); got != nil {
			seen[got.id.id] = true
		}
	}
	for id := range want {
		if !seen[id] {
			t.Fatalf("tip %s was never reached by any walk", id.String())
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
	for _, seed := range selectionSeeds {
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			withSeededSelection(t, seed)
			walkPrefersTheBranchMoreOfTheGraphConfirms(t)
		})
	}
}

func walkPrefersTheBranchMoreOfTheGraphConfirms(t *testing.T) {
	tr := newConfirmTracker(0, 1000)
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
			// Thrown from the shared root on purpose: this measures the step
			// choice out of one site, not where the particle lands.
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
	for _, seed := range selectionSeeds {
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			withSeededSelection(t, seed)
			testWeightedChoiceFollowsItsWeights(t)
		})
	}
}

func testWeightedChoiceFollowsItsWeights(t *testing.T) {
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
	tr := newConfirmTracker(0, 1000)
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

// Selection must not offer genesis as a consolation prize. On a follower or a
// recovered node the genesis site is held as a pointer but is in no local index
// (adoptGenesis keeps the pointer and empties the maps), so a site that approves
// it names an approval the local node cannot resolve: it stays detached in the
// tracker, out of both the denominator and the tip set, and is re-requested
// every second for the life of the process. Returning nothing instead makes
// AddTxDag refuse the transaction, which the publisher retries.
func TestSelectTipsOffersNothingRatherThanGenesis(t *testing.T) {
	tr := newConfirmTracker(0, 1000)
	withTracker(t, tr)
	genesis := tnode(0)
	d := &Dag{genesis: genesis}

	if tips := d.selectTips(0.5); len(tips) != 0 {
		t.Fatalf("expected no approval target on an empty graph, got %d", len(tips))
	}
	if tips := d.uniformTips(); len(tips) != 0 {
		t.Fatalf("expected no approval target from the uniform fallback on an empty graph, got %d", len(tips))
	}
}

// dag.approvetx says how many sites a new site approves, and the confirmation
// tracker retires a tip once that many sites reference it. Selection has to read
// the same setting: if it emits two approvals while a tip needs three to retire,
// no tip ever retires, the confirmation denominator only grows, and the ledger
// confirms almost nothing.
func TestSelectTipsHonoursTheConfiguredApprovalCount(t *testing.T) {
	prev := dagConfig.Approvetx
	t.Cleanup(func() { dagConfig.Approvetx = prev })

	for _, want := range []uint16{2, 3, 4} {
		dagConfig.Approvetx = want
		tr := newConfirmTracker(0, 1000)
		withTracker(t, tr)

		// A frontier comfortably wider than the number of approvals asked for.
		root := tnode(0)
		tr.add(root)
		for i := 1; i <= 12; i++ {
			n := tnode(i)
			tlink(n, root)
			tr.add(n)
		}
		d := &Dag{genesis: root}

		tips := d.selectTips(0.5)
		if len(tips) != int(want) {
			t.Fatalf("approvetx=%d: expected %d approval targets, got %d", want, want, len(tips))
		}
		seen := map[uuid.UUID]bool{}
		for _, tip := range tips {
			if seen[tip.id.id] {
				t.Fatalf("approvetx=%d: the same site was offered twice", want)
			}
			seen[tip.id.id] = true
		}
	}
}

// The confirmation threshold has to be reachable at every configured approval
// count, not only at two.
func TestApprovalThresholdIsReachedAtEveryConfiguredCount(t *testing.T) {
	prev := dagConfig.Approvetx
	t.Cleanup(func() { dagConfig.Approvetx = prev })

	for _, want := range []uint16{2, 3, 4} {
		dagConfig.Approvetx = want
		tr := newConfirmTracker(0, 1000)
		withTracker(t, tr)

		genesis := tnode(0)
		d := &Dag{genesis: genesis}
		tr.add(genesis)
		confirmed := 0
		for step := 1; step <= 600; step++ {
			targets := d.selectTips(0.5)
			if len(targets) == 0 {
				t.Fatalf("approvetx=%d step %d: selection offered nothing to approve", want, step)
			}
			n := tnode(step)
			for _, target := range targets {
				tlink(n, target)
			}
			tr.add(n)
			confirmed += len(tr.pop())
		}
		if confirmed < 300 {
			_, tips, _ := tr.stats()
			t.Fatalf("approvetx=%d: only %d of 600 sites confirmed, with %d tips still open - the frontier is not settling",
				want, confirmed, tips)
		}
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
	// Seeded: walkMisses and the confirmed count below are both rates over six
	// hundred selections, and a rate measured on an unseeded source is a test
	// that fails at whatever its margin allows and is then widened until it
	// tests nothing.
	withSeededSelection(t, 4242)
	tr := newConfirmTracker(0, 1000)
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
		if step > 20 && deepWalk(d, 0.5) == nil {
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

	// Driven by real selection, so this is where a tip index that drifts under
	// sampling or under the collision fallback shows up. The tracker's own unit
	// tests drive those calls directly; this one drives them the way the node
	// does.
	assertTipIndexIsConsistent(t, tr, "after 600 selected inserts")

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
	tr := newConfirmTracker(0, 1000)
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

// The three narrow reads selection makes of the tip set have to answer what the
// whole-set read did. They exist so that selection does not copy the tip set to
// take one element of it, and each of them is a place where "cheaper" could
// quietly become "different".

// A site already chosen for one of this site's approvals must not be offered for
// the other. Approving the same site twice is one approval, not two, so a
// selection that does it makes half the approvals it was asked for and the graph
// stops widening. This is the fallback path selection takes when the walks keep
// colliding, so it is not reached by driving selection on a healthy graph.
func TestATipAlreadyTakenIsNotOfferedAgain(t *testing.T) {
	tr := newConfirmTracker(0, 1000)
	root := tnode(0)
	tr.add(root)
	a, b := tnode(1), tnode(2)
	tlink(a, root)
	tlink(b, root)
	tr.add(a)
	tr.add(b)

	taken := map[uuid.UUID]struct{}{a.id.id: {}}
	for trial := 0; trial < 100; trial++ {
		got := tr.tipExcept(taken)
		if got == nil {
			t.Fatalf("trial %d: no tip was offered while one was free", trial)
		}
		if got.id.id == a.id.id {
			t.Fatalf("trial %d: a tip already taken was offered again", trial)
		}
	}
	// And nothing is offered once everything is taken, rather than a duplicate.
	taken[b.id.id] = struct{}{}
	if got := tr.tipExcept(taken); got != nil {
		t.Fatalf("a tip was offered when every tip was already taken: %s", got.id.id.String())
	}
}

// Throwing the particle back has to pick uniformly among the approvals a site
// made. Always taking the same one turns the paper's W into a walk down one
// fixed branch, so the sites on every other branch are never reachable from a
// throw and only ever get approved by the uniform fallback.
func TestThrowingAParticleBackChoosesUniformlyAmongApprovals(t *testing.T) {
	for _, seed := range selectionSeeds {
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			withSeededSelection(t, seed)
			testThrowingAParticleBackChoosesUniformlyAmongApprovals(t)
		})
	}
}

func testThrowingAParticleBackChoosesUniformlyAmongApprovals(t *testing.T) {
	tr := newConfirmTracker(0, 1000)
	targets := make([]*Node, 0, 3)
	for i := 0; i < 3; i++ {
		n := tnode(i)
		tr.add(n)
		targets = append(targets, n)
	}
	// A tip on its own branch, so the three approvals below are not confirmed by
	// the whole tip set the moment they are approved. Without it they would be,
	// and they would leave the region before anything could step back to them.
	tr.add(tnode(8))

	from := tnode(9)
	for _, target := range targets {
		tlink(from, target)
	}
	tr.add(from)

	const draws = 3000
	hits := map[uuid.UUID]int{}
	for i := 0; i < draws; i++ {
		got := tr.stepBack(from)
		if got == nil {
			t.Fatalf("draw %d: nothing to step back to while three approvals are in the region", i)
		}
		hits[got.id.id]++
	}
	// Generous: this is a fairness check, not a test of the generator.
	want := draws / len(targets)
	for _, target := range targets {
		if got := hits[target.id.id]; got < want/2 || got > want*2 {
			t.Fatalf("approval %s was stepped back to %d times in %d draws, expected about %d",
				target.id.id.String(), got, draws, want)
		}
	}
}

// Sampling the tip set has to reach all of it. The sample is drawn by shuffling
// the set in place, so a shuffle that does not move anything returns the same
// sites every time - the uniform fallback would then hand out the same approval
// targets for the life of the process, and every other tip would go unapproved.
func TestSamplingTheTipSetReachesEveryTip(t *testing.T) {
	for _, seed := range selectionSeeds {
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			withSeededSelection(t, seed)
			testSamplingTheTipSetReachesEveryTip(t)
		})
	}
}

func testSamplingTheTipSetReachesEveryTip(t *testing.T) {
	tr := newConfirmTracker(0, 1000)
	root := tnode(0)
	tr.add(root)
	tips := make([]*Node, 0, 6)
	for i := 1; i <= 6; i++ {
		n := tnode(i)
		tlink(n, root)
		tr.add(n)
		tips = append(tips, n)
	}

	const rounds = 2000
	hits := map[uuid.UUID]int{}
	for i := 0; i < rounds; i++ {
		for _, n := range tr.sampleTips(2, nil) {
			hits[n.id.id]++
		}
	}
	want := rounds * 2 / len(tips)
	for _, tip := range tips {
		if got := hits[tip.id.id]; got < want/2 || got > want*2 {
			t.Fatalf("tip %s was sampled %d times in %d rounds of two, expected about %d",
				tip.id.id.String(), got, rounds, want)
		}
	}
}

// A walk reuses one set of buffers for every step it takes and every walk one
// selection makes, so the buffers have to give exactly the answers fresh ones
// would. A buffer that is appended to rather than reset leaves the previous
// step's weights in front of this step's candidates, and weightedChoice reads
// as many weights as it has candidates - so the walk would be biased by a
// comparison it made somewhere else, with nothing to show for it.
func TestReusingAWalkBufferGivesTheSameWeights(t *testing.T) {
	cases := [][]int{{9, 7, 3}, {4}, {6, 6}, {}}
	// Deliberately dirty, and longer than any of the cases below.
	buf := []float64{99, 98, 97, 96, 95, 94}
	for i, next := range cases {
		want := transitionWeights(10, next, 0.5)
		buf = transitionWeightsInto(10, next, 0.5, buf)
		if len(buf) != len(next) {
			t.Fatalf("case %d: the reused buffer holds %d weights for %d candidates",
				i, len(buf), len(next))
		}
		for j := range want {
			if buf[j] != want[j] {
				t.Fatalf("case %d: reused buffer gave weight %v at %d, a fresh one gives %v",
					i, buf[j], j, want[j])
			}
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
	tr := newConfirmTracker(0, 1000)
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
	for _, seed := range selectionSeeds {
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			withSeededSelection(t, seed)
			testBothApprovalsAreBiased(t)
		})
	}
}

func testBothApprovalsAreBiased(t *testing.T) {
	tr := newConfirmTracker(0, 1000)
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

// A site's approvals must be counted once, at the point it becomes part of the
// graph - not once when it arrives with unresolved targets and again when they
// are resolved.
//
// The consequence of counting twice is not a stray number: a tip retires from
// the denominator after one real approver instead of two, so the confirmation
// share is measured against a denominator that has already shrunk, and sites are
// confirmed that the rest of the network has approved once. On any peer that is
// briefly out of sync - the ordinary case, since missing targets are reconciled
// every second - that is the fixed-threshold failure the share rule exists to
// remove.
func TestAnApprovalIsCountedOnce(t *testing.T) {
	tr := newConfirmTracker(0, 1000)

	// An unrelated branch, so the site under test is not confirmed and swept out
	// of the region between assertions.
	other := tnode(10)
	tr.add(other)
	spare := tnode(11)
	tlink(spare, other)
	tr.add(spare)

	target := tnode(0)
	tr.add(target)

	// Arrives approving target, but also names a target it has never seen.
	late := tnode(1)
	tlink(late, target)
	late.missingTargets = map[string]bool{uuid.New().String(): true}
	tr.add(late)

	// While detached it confirms nothing, so its approval must not count.
	if got := approversOf(t, tr, target); got != 0 {
		t.Fatalf("a detached site's approval was counted: target has %d approver(s)", got)
	}

	// The gap is filled.
	late.missingTargets = nil
	tr.relink(late)

	if got := approversOf(t, tr, target); got != 1 {
		t.Fatalf("one approval should be counted once, got %d", got)
	}
}

// approversOf - the tracker's approval count for a site, or -1 if it has left
// the active region.
func approversOf(t *testing.T, tr *ConfirmTracker, n *Node) int {
	t.Helper()
	tr.mu.Lock()
	defer tr.mu.Unlock()
	st, ok := tr.sites[n.id.id]
	if !ok {
		return -1
	}
	return st.approvers
}

// Nothing on the receiving side checks that a peer named two distinct approval
// targets, so the tracker has to.
func TestDuplicateApprovalTargetsCountOnce(t *testing.T) {
	tr := newConfirmTracker(0, 1000)
	// An unrelated branch, so the site under test is not confirmed and swept out
	// of the region before it can be inspected.
	other := tnode(10)
	tr.add(other)
	spare := tnode(11)
	tlink(spare, other)
	tr.add(spare)

	target := tnode(0)
	tr.add(target)

	greedy := tnode(1)
	tlink(greedy, target)
	tlink(greedy, target) // the same site named twice
	tr.add(greedy)

	if got := approversOf(t, tr, target); got != 1 {
		t.Fatalf("naming the same site twice is one approval, got %d", got)
	}
}

// The test the whole selection design rests on, and the one whose absence let
// two different broken designs look healthy.
//
// Every other test drives the graph one site at a time, each choosing its
// approvals against a graph that already contains every site before it. A real
// network never does that: sites are published concurrently, so several choose
// against the same view and none of them approves another. The difference is not
// a detail - it is the difference between confirming everything and confirming
// nothing, and it is invisible to sequential tests.
//
// The measured boundary, 6000 inserts per setting, tip timeout off so the
// liveness valve cannot be what makes it pass:
//
//	concurrent publishers   share 1000   share 667
//	1                          100.0%      100.0%
//	4                           99.7%       99.8%
//	16                           0.0%       98.1%
//	64                           0.0%       32.5%
//
// So the technical paper's literal 100% rule works for a handful of concurrent
// publishers and stops dead beyond that: it needs every one of the live tips to
// cover a site, and the tip set grows with concurrency while new tips keep
// arriving. A share below 100% converges because it does not wait for the
// stragglers, which is why the default is two thirds. Both settings are
// exercised here so that neither can regress and so the boundary above is
// checked rather than remembered.
func TestConfirmationConvergesUnderConcurrentArrival(t *testing.T) {
	prevA, prevW := dagConfig.Approvetx, dagConfig.Walkdepth
	dagConfig.Approvetx, dagConfig.Walkdepth = 2, 10
	t.Cleanup(func() { dagConfig.Approvetx, dagConfig.Walkdepth = prevA, prevW })

	cases := []struct {
		fanout int
		share  uint16
	}{
		// The default has to hold across the whole range.
		{1, DAG_CONFIRM_SHARE}, {2, DAG_CONFIRM_SHARE}, {4, DAG_CONFIRM_SHARE},
		{8, DAG_CONFIRM_SHARE}, {16, DAG_CONFIRM_SHARE},
		// The paper's literal rule, at the only concurrency it survives. Kept so
		// that the setting still works for anyone who needs it, and so the
		// boundary in the table above stays honest rather than remembered.
		{1, 1000}, {4, 1000},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(fmt.Sprintf("fanout%d_share%d", tc.fanout, tc.share), func(t *testing.T) {
			// Seeded, because the assertions below are about a rate over four
			// thousand selections and an unseeded source makes them a rate that
			// is right most of the time. The seven cases are seven different
			// arrival patterns, so this is not one trajectory standing in for
			// the rule; it is each pattern being the same run every time.
			withSeededSelection(t, 1+int64(tc.fanout)*1000+int64(tc.share))
			// tiptimeout 0: the expiry valve is a liveness safeguard for
			// abandoned tips, and if convergence depended on it then the
			// confirmation rule would not work - it would just be hidden.
			tr := newConfirmTracker(0, tc.share)
			withTracker(t, tr)

			genesis := tnode(0)
			d := &Dag{genesis: genesis}
			tr.add(genesis)

			const inserts = 4000
			rounds := inserts / tc.fanout
			confirmed, id := 0, 1
			for round := 0; round < rounds; round++ {
				// Every site in the batch chooses against the same view.
				batch := make([][]*Node, 0, tc.fanout)
				for k := 0; k < tc.fanout; k++ {
					targets := d.selectTips(0.5)
					if len(targets) == 0 {
						t.Fatalf("round %d: selection offered nothing to approve", round)
					}
					batch = append(batch, targets)
				}
				for _, targets := range batch {
					n := tnode(id)
					id++
					for _, target := range targets {
						tlink(n, target)
					}
					tr.add(n)
				}
				confirmed += len(tr.pop())
			}

			assertTipIndexIsConsistent(t, tr, "after the run")

			inserted := id - 1
			active, tips, _ := tr.stats()
			t.Logf("fanout=%d share=%d inserted=%d confirmed=%d (%.1f%%) active=%d tips=%d",
				tc.fanout, tc.share, inserted, confirmed,
				100*float64(confirmed)/float64(inserted), active, tips)

			// Most of what went in has to settle. The last rounds are
			// legitimately still open and a batch cannot confirm its own
			// members, so this is deliberately not "all of it".
			if want := inserted * 3 / 4; confirmed < want {
				t.Fatalf("only %d of %d sites confirmed (wanted at least %d): the frontier is not settling",
					confirmed, inserted, want)
			}
			// The denominator has to stay bounded by the arrival pattern rather
			// than growing with the ledger. Unbounded growth is the failure
			// mode, and it shows as a tip count that tracks the insert count.
			if tips > 20*tc.fanout+20 {
				t.Fatalf("the tip set grew to %d after %d inserts at fanout %d: new tips are not being approved",
					tips, inserted, tc.fanout)
			}
			if active > inserted/4 {
				t.Fatalf("the active region holds %d of %d sites: confirmation is not keeping up",
					active, inserted)
			}
		})
	}
}
