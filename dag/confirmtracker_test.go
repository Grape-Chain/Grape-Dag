package dag

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------- test graph

func tnode(n int) *Node {
	id := uuid.New()
	return &Node{id: NodeID{id: id, idMajor: uint64(n)}}
}

func tlink(child, parent *Node) {
	child.targets = append(child.targets, parent)
	parent.sources = append(parent.sources, child)
}

// oracleConfirmed - the definition, computed the slow and obvious way: walk
// backwards from every current tip and mark what it reaches, then a site is
// confirmed when every tip reached it. Used to check the incremental tracker.
//
// A tip is a site nothing approves. That is the ordinary meaning and it is not
// dag.approvetx, which says how many sites a new site approves.
func oracleConfirmed(nodes []*Node, share uint16) map[uuid.UUID]bool {
	tips := tipsOf(nodes)
	if len(tips) == 0 {
		return map[uuid.UUID]bool{}
	}
	reached := map[uuid.UUID]int{}
	for _, t := range tips {
		seen := map[uuid.UUID]bool{}
		stack := append([]*Node{}, t.targets...)
		for len(stack) > 0 {
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if n == nil || seen[n.id.id] {
				continue
			}
			seen[n.id.id] = true
			reached[n.id.id]++
			stack = append(stack, n.targets...)
		}
	}
	need := (len(tips)*int(share) + 999) / 1000
	if need < 1 {
		need = 1
	}
	out := map[uuid.UUID]bool{}
	for _, n := range nodes {
		// A tip never confirms itself, so it can reach the threshold below a
		// 100% share without being confirmable.
		if len(n.sources) == 0 {
			continue
		}
		if reached[n.id.id] >= need {
			out[n.id.id] = true
		}
	}
	return out
}

func tipsOf(nodes []*Node) []*Node {
	tips := []*Node{}
	for _, n := range nodes {
		if len(n.sources) == 0 {
			tips = append(tips, n)
		}
	}
	return tips
}

// ---------------------------------------------------------------- oracle test

// The incremental tracker must agree with the definition at every step: it may
// never confirm a site the definition does not (soundness), and it may not miss
// one it does (completeness).
func TestTrackerAgreesWithTheDefinition(t *testing.T) {
	const approveTx = 2
	for _, seed := range []int64{1, 2, 3, 7, 11, 42} {
		seed := seed
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			rng := rand.New(rand.NewSource(seed))
			// tiptimeout 0: the expiry valve would legitimately diverge from the
			// definition, and it is exercised separately.
			tr := newConfirmTracker(0, 1000)

			genesis := tnode(0)
			nodes := []*Node{genesis}
			tr.add(genesis)

			everConfirmed := map[uuid.UUID]bool{}
			width := 5

			for step := 1; step <= 220; step++ {
				n := tnode(step)
				if len(nodes) <= width {
					tlink(n, genesis)
				} else {
					tips := tipsOf(nodes)
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

				oracle := oracleConfirmed(nodes, 1000)

				// Invariant the sweep relies on: a tip never confirms itself, so
				// its coverage is always short of the denominator and a tip can
				// never be confirmed. Checked here because it is the reason the
				// sweep can treat tips as ineligible.
				tr.mu.Lock()
				for id, track := range tr.sites {
					if track.slot >= 0 && track.count >= tr.tipCount {
						tr.mu.Unlock()
						t.Fatalf("step %d: tip %s has coverage %d with %d tips - a tip must never reach the denominator",
							step, id.String(), track.count, tr.tipCount)
					}
					if track.count > tr.tipCount {
						tr.mu.Unlock()
						t.Fatalf("step %d: site %s is confirmed by %d tips but only %d exist",
							step, id.String(), track.count, tr.tipCount)
					}
				}
				tr.mu.Unlock()

				// soundness: everything handed over is genuinely confirmed
				for _, got := range tr.pop() {
					if !oracle[got.id.id] {
						t.Fatalf("step %d: tracker confirmed %s but the definition does not",
							step, got.id.id.String())
					}
					if everConfirmed[got.id.id] {
						t.Fatalf("step %d: tracker confirmed %s twice", step, got.id.id.String())
					}
					everConfirmed[got.id.id] = true
				}

				// completeness: nothing the definition confirms is left behind
				for id := range oracle {
					if !everConfirmed[id] {
						t.Fatalf("step %d: definition confirms %s but the tracker has not",
							step, id.String())
					}
				}
			}

			if len(everConfirmed) == 0 {
				t.Fatalf("nothing was ever confirmed - the test is not exercising the rule")
			}
			t.Logf("confirmed %d of %d sites", len(everConfirmed), len(nodes))
		})
	}
}

// ---------------------------------------------------------------- unit cases

// The whole point of the change: two direct approvals are not enough on their
// own. A site is confirmed when the tips that exist all confirm it.
func TestTwoApprovalsAloneDoNotConfirm(t *testing.T) {
	tr := newConfirmTracker(0, 1000)

	genesis := tnode(0)
	tr.add(genesis)
	// two independent branches off genesis
	a, b := tnode(1), tnode(2)
	tlink(a, genesis)
	tlink(b, genesis)
	tr.add(a)
	tr.add(b)

	// c approves a twice over? no - c and d each approve a, retiring it as a tip
	c, d := tnode(3), tnode(4)
	tlink(c, a)
	tlink(d, a)
	tr.add(c)
	tr.add(d)

	// a now has two direct approvers, which the old rule counted as confirmed.
	// But tip b has no path to a, so not every tip confirms it.
	confirmedEver := map[uuid.UUID]bool{}
	for _, got := range tr.pop() {
		if got.id.id == a.id.id {
			t.Fatalf("site with two approvers was confirmed while a tip (b) had no path to it")
		}
		confirmedEver[got.id.id] = true
	}
	if share, ok := tr.shareOf(a.id.id); !ok || share >= 1 {
		t.Fatalf("share of a = %v (ok=%v), want a value below 1", share, ok)
	}

	// Fold b's branch in, and retire b as a tip, so that every remaining tip
	// has a path to a.
	e := tnode(5)
	tlink(e, b)
	tlink(e, c)
	tr.add(e)
	h := tnode(6)
	tlink(h, b)
	tlink(h, d)
	tr.add(h)

	for _, got := range tr.pop() {
		confirmedEver[got.id.id] = true
	}
	if !confirmedEver[a.id.id] {
		t.Fatalf("a should be confirmed once every tip has a path to it")
	}
	// confirmation is closed downwards, so genesis went first
	if !confirmedEver[genesis.id.id] {
		t.Fatalf("genesis should be confirmed too: confirmation is closed downwards")
	}
}

func TestConfirmedSitesLeaveTheActiveRegion(t *testing.T) {
	tr := newConfirmTracker(0, 1000)
	genesis := tnode(0)
	tr.add(genesis)
	nodes := []*Node{genesis}
	rng := rand.New(rand.NewSource(9))

	for step := 1; step <= 400; step++ {
		n := tnode(step)
		tips := tipsOf(nodes)
		if len(tips) < 2 {
			tlink(n, tips[0])
		} else {
			i, j := rng.Intn(len(tips)), rng.Intn(len(tips))
			for j == i {
				j = rng.Intn(len(tips))
			}
			tlink(n, tips[i])
			tlink(n, tips[j])
		}
		nodes = append(nodes, n)
		tr.add(n)
		tr.pop()
	}

	active, tips, _ := tr.stats()
	// The active region is the frontier, not the ledger: it must not grow with
	// total transactions. Generous bound - the point is that it is bounded.
	if active > 120 {
		t.Fatalf("active region held %d sites after 400 inserts (tips=%d); it should stay near the frontier", active, tips)
	}
	t.Logf("after 400 inserts: active=%d tips=%d", active, tips)
}

func TestHarvestedSitesAreNotConfirmedAgain(t *testing.T) {
	tr := newConfirmTracker(0, 1000)
	genesis := tnode(0)
	tr.add(genesis)
	tr.markHarvested(genesis.id.id)

	a := tnode(1)
	tlink(a, genesis)
	tr.add(a)
	b := tnode(2)
	tlink(b, genesis)
	tr.add(b)
	c := tnode(3)
	tlink(c, a)
	tlink(c, b)
	tr.add(c)

	for _, got := range tr.pop() {
		if got.id.id == genesis.id.id {
			t.Fatalf("a harvested site was confirmed again")
		}
	}
	// re-adding a harvested site must not resurrect it as a tip either
	tr.add(genesis)
	if tr.isTip(genesis.id.id) {
		t.Fatalf("a harvested site came back as a tip")
	}
}

// A site inserted before its approval targets have arrived cannot be trusted to
// confirm anything, and must not count towards the denominator.
func TestDetachedSitesDoNotCountOrConfirm(t *testing.T) {
	tr := newConfirmTracker(0, 1000)
	genesis := tnode(0)
	tr.add(genesis)

	a := tnode(1)
	tlink(a, genesis)
	tr.add(a)
	b := tnode(2)
	tlink(b, genesis)
	tr.add(b)

	_, tipsBefore, _ := tr.stats()

	orphan := tnode(3)
	orphan.missingTargets = map[string]bool{uuid.New().String(): true}
	tr.add(orphan)

	if tr.isTip(orphan.id.id) {
		t.Fatalf("a site with unresolved targets was counted as a tip")
	}
	if _, tipsAfter, _ := tr.stats(); tipsAfter != tipsBefore {
		t.Fatalf("denominator moved from %d to %d for a detached site", tipsBefore, tipsAfter)
	}

	// once relinked it joins in
	orphan.missingTargets = nil
	tlink(orphan, a)
	tr.relink(orphan)
	if !tr.isTip(orphan.id.id) {
		t.Fatalf("a relinked site did not become a tip")
	}
	// The relinked site joins the denominator and the site it approves leaves
	// it, because a tip is a site nothing approves. Net zero.
	if _, tipsAfter, _ := tr.stats(); tipsAfter != tipsBefore {
		t.Fatalf("denominator is %d after relink, want %d", tipsAfter, tipsBefore)
	}
}

// One tip that is never approved would otherwise hold the denominator hostage
// and stall confirmation for the whole ledger.
func TestAbandonedTipStopsHoldingUpConfirmation(t *testing.T) {
	tr := newConfirmTracker(50*time.Millisecond, 1000)
	genesis := tnode(0)
	tr.add(genesis)

	abandoned := tnode(1)
	tlink(abandoned, genesis)
	tr.add(abandoned)

	// a branch that never references the abandoned tip
	prev := genesis
	live := []*Node{}
	for i := 2; i <= 6; i++ {
		n := tnode(i)
		tlink(n, prev)
		tr.add(n)
		live = append(live, n)
		prev = n
	}
	for _, got := range tr.pop() {
		if got.id.id == live[0].id.id {
			t.Fatalf("confirmed a site while an unapproved tip still had no path to it")
		}
	}

	time.Sleep(120 * time.Millisecond)
	// any activity triggers the expiry check
	n := tnode(7)
	tlink(n, prev)
	tr.add(n)

	if tr.holdsSlot(abandoned.id.id) {
		t.Fatalf("the abandoned tip still counts towards the denominator after the timeout")
	}
	if !tr.isTip(abandoned.id.id) {
		t.Fatalf("the abandoned tip should still be selectable so it can be approved later")
	}
	confirmed := map[uuid.UUID]bool{}
	for _, got := range tr.pop() {
		confirmed[got.id.id] = true
	}
	if len(confirmed) == 0 {
		t.Fatalf("nothing was confirmed after the abandoned tip stopped counting")
	}
}

// A tip dropped from the denominator for going unapproved must stay selectable.
// If it disappeared from tip selection it could never gain approvals, would
// never be confirmed, and its transaction would never reach a commit tx.
func TestExpiredTipStaysSelectable(t *testing.T) {
	tr := newConfirmTracker(40*time.Millisecond, 1000)
	genesis := tnode(0)
	tr.add(genesis)

	abandoned := tnode(1)
	tlink(abandoned, genesis)
	tr.add(abandoned)

	// keep the tracker busy elsewhere so the expiry sweep runs
	prev := genesis
	for i := 2; i <= 4; i++ {
		n := tnode(i)
		tlink(n, prev)
		tr.add(n)
		prev = n
	}
	time.Sleep(100 * time.Millisecond)
	nudge := tnode(5)
	tlink(nudge, prev)
	tr.add(nudge)

	// out of the denominator, but still offered for approval
	if !tr.isTip(abandoned.id.id) {
		t.Fatalf("an expired tip must stay selectable, or its transaction is stranded")
	}
	found := false
	for _, n := range tr.getTips() {
		if n.id.id == abandoned.id.id {
			found = true
		}
	}
	if !found {
		t.Fatalf("an expired tip is missing from tip selection")
	}

	// and once approved it confirms like any other site
	x := tnode(6)
	tlink(x, abandoned)
	tr.add(x)
	y := tnode(7)
	tlink(y, abandoned)
	tr.add(y)
	if tr.isTip(abandoned.id.id) {
		t.Fatalf("a site with its full approvals should no longer be selectable")
	}

	// Merge every branch, including the one that never referenced the abandoned
	// site, so that no live tip is left without a path to it.
	m1 := tnode(8)
	tlink(m1, nudge)
	tlink(m1, x)
	tr.add(m1)
	m2 := tnode(9)
	tlink(m2, nudge)
	tlink(m2, y)
	tr.add(m2)
	m3 := tnode(10)
	tlink(m3, m1)
	tlink(m3, m2)
	tr.add(m3)
	m4 := tnode(11)
	tlink(m4, m1)
	tlink(m4, m2)
	tr.add(m4)

	confirmed := map[uuid.UUID]bool{}
	for _, got := range tr.pop() {
		confirmed[got.id.id] = true
	}
	if !confirmed[abandoned.id.id] {
		t.Fatalf("a previously abandoned site was never confirmed after being approved")
	}
}

func TestShareIsTheFractionOfTipsThatConfirm(t *testing.T) {
	tr := newConfirmTracker(0, 1000)
	genesis := tnode(0)
	tr.add(genesis)
	a, b := tnode(1), tnode(2)
	tlink(a, genesis)
	tlink(b, genesis)
	tr.add(a)
	tr.add(b)

	// Both tips confirm genesis, so it is already confirmed and has left the
	// active region - that is the rule working, not a missing site.
	if _, ok := tr.shareOf(genesis.id.id); ok {
		t.Fatalf("genesis should be confirmed and out of the active region once every tip confirms it")
	}
	confirmed := map[uuid.UUID]bool{}
	for _, got := range tr.pop() {
		confirmed[got.id.id] = true
	}
	if !confirmed[genesis.id.id] {
		t.Fatalf("genesis was not handed over as confirmed")
	}

	// One more site approving a. That takes a out of the tip set - a tip is a
	// site nothing approves - leaving b and c as the two tips, and only c
	// confirms a.
	c := tnode(3)
	tlink(c, a)
	tr.add(c)

	share, ok := tr.shareOf(a.id.id)
	if !ok {
		t.Fatalf("a should still be in the active region")
	}
	if want := 0.5; share < want-1e-9 || share > want+1e-9 {
		t.Fatalf("share of a = %v, want %v (one of two tips confirms it)", share, want)
	}
}

func TestBitsetSetClearCount(t *testing.T) {
	var b bitset
	if b.test(0) || b.count() != 0 {
		t.Fatalf("a fresh bitset is not empty")
	}
	if !b.set(130) || !b.test(130) || b.count() != 1 {
		t.Fatalf("setting bit 130 did not take, count=%d", b.count())
	}
	if b.set(130) {
		t.Fatalf("setting an already-set bit reported a change")
	}
	if !b.set(3) || b.count() != 2 {
		t.Fatalf("count is %d after two sets", b.count())
	}
	if !b.clear(130) || b.test(130) || b.count() != 1 {
		t.Fatalf("clearing bit 130 did not take, count=%d", b.count())
	}
	if b.clear(130) {
		t.Fatalf("clearing an already-clear bit reported a change")
	}
	if b.set(-1) || b.clear(-1) || b.test(-1) {
		t.Fatalf("a negative index should be ignored")
	}
}
