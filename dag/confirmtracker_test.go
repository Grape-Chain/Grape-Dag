package dag

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/Grape-Chain/Grape-Dag/tx/pb"
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

// assertTipIndexIsConsistent - the tip ring has to hold exactly the sites the
// scan it replaced would have returned, and its index has to point at where
// they actually sit.
//
// This is the whole safety argument for indexing the tip set instead of scanning
// for it. The predicate below is the scan, verbatim: tracked, a site behind it,
// not detached, nothing approves it. If the two ever disagree, selection is
// either offering a site that may not be approved or hiding one that may - and
// which sites get approved is which sites confirm.
func assertTipIndexIsConsistent(t *testing.T, tr *ConfirmTracker, when string) {
	t.Helper()
	tr.mu.Lock()
	defer tr.mu.Unlock()
	want := map[uuid.UUID]bool{}
	for id, track := range tr.sites {
		if track.node == nil || track.detached || track.approvers != 0 {
			continue
		}
		want[id] = true
	}
	if len(tr.tipRing) != len(want) {
		t.Fatalf("%s: the tip ring holds %d sites, the active region says %d are selectable",
			when, len(tr.tipRing), len(want))
	}
	if len(tr.tipAt) != len(tr.tipRing) {
		t.Fatalf("%s: the tip index holds %d entries for a ring of %d",
			when, len(tr.tipAt), len(tr.tipRing))
	}
	for i, track := range tr.tipRing {
		if track == nil || track.node == nil {
			t.Fatalf("%s: the tip ring holds nothing at %d", when, i)
		}
		id := track.node.id.id
		if !want[id] {
			t.Fatalf("%s: the tip ring offers %s, which is not selectable", when, id.String())
		}
		if at, ok := tr.tipAt[id]; !ok || at != i {
			t.Fatalf("%s: the tip index puts %s at %d, the ring has it at %d", when, id.String(), at, i)
		}
		if held, ok := tr.sites[id]; !ok || held != track {
			t.Fatalf("%s: the tip ring holds a site the active region does not: %s", when, id.String())
		}
	}
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

				assertTipIndexIsConsistent(t, tr, fmt.Sprintf("step %d", step))

				// The tip set is the denominator, so it is as much a part of the
				// definition as the coverage count is: a tip is a site nothing
				// approves. Checked against the graph rather than against the
				// tracker's own bookkeeping, because a tracker that agrees with
				// itself proves nothing.
				gotTips := map[uuid.UUID]bool{}
				for _, n := range tr.getTips() {
					gotTips[n.id.id] = true
				}
				wantTips := tipsOf(nodes)
				if len(gotTips) != len(wantTips) {
					t.Fatalf("step %d: the tracker offers %d tips, the graph has %d",
						step, len(gotTips), len(wantTips))
				}
				for _, n := range wantTips {
					if !gotTips[n.id.id] {
						t.Fatalf("step %d: %s is a tip nothing approves and the tracker does not offer it",
							step, n.id.id.String())
					}
				}

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

// Every change to what may be approved has to move the tip index with it. The
// index is read by selection instead of the active region being scanned, so an
// index that drifts is selection offering the wrong sites - which is the tip
// definition being wrong again, one layer down.
func TestTheTipSetTracksEveryChangeToWhatIsSelectable(t *testing.T) {
	tr := newConfirmTracker(0, 1000)

	genesis := tnode(0)
	tr.add(genesis)
	assertTipIndexIsConsistent(t, tr, "one site")
	if got := len(tr.getTips()); got != 1 {
		t.Fatalf("a lone site is the only tip, got %d", got)
	}

	// A first approval takes a site out of the tip set, and a second must not
	// take anything else out with it.
	a, b := tnode(1), tnode(2)
	tlink(a, genesis)
	tlink(b, genesis)
	tr.add(a)
	assertTipIndexIsConsistent(t, tr, "genesis approved once")
	tr.add(b)
	assertTipIndexIsConsistent(t, tr, "genesis approved twice")
	for _, n := range tr.getTips() {
		if n.id.id == genesis.id.id {
			t.Fatalf("an approved site is still offered as a tip")
		}
	}

	// A detached site is not selectable, and becomes so the moment it relinks.
	orphan := tnode(3)
	orphan.missingTargets = map[string]bool{uuid.New().String(): true}
	tr.add(orphan)
	assertTipIndexIsConsistent(t, tr, "detached site tracked")
	for _, n := range tr.getTips() {
		if n.id.id == orphan.id.id {
			t.Fatalf("a detached site was offered as a tip")
		}
	}
	orphan.missingTargets = nil
	tlink(orphan, a)
	tr.relink(orphan)
	assertTipIndexIsConsistent(t, tr, "detached site relinked")
	found := false
	for _, n := range tr.getTips() {
		if n.id.id == orphan.id.id {
			found = true
		}
	}
	if !found {
		t.Fatalf("a relinked site is a site nothing approves, so it has to be selectable")
	}

	// A site harvested out of band leaves the region, and the tip set with it.
	tr.markHarvested(b.id.id)
	assertTipIndexIsConsistent(t, tr, "site harvested")
	for _, n := range tr.getTips() {
		if n.id.id == b.id.id {
			t.Fatalf("a harvested site is still offered as a tip")
		}
	}
}

// An expired tip is the one case where a site can be confirmed while it is still
// selectable: it is out of the denominator, so nothing stops its own coverage
// reaching the threshold. It has to leave the tip set when it does, or selection
// keeps offering a site that has been settled.
func TestAConfirmedExpiredTipLeavesTheTipSet(t *testing.T) {
	tr := newConfirmTracker(30*time.Millisecond, 1000)
	genesis := tnode(0)
	tr.add(genesis)

	abandoned := tnode(1)
	tlink(abandoned, genesis)
	tr.add(abandoned)

	prev := genesis
	for i := 2; i <= 5; i++ {
		n := tnode(i)
		tlink(n, prev)
		tr.add(n)
		prev = n
	}
	time.Sleep(80 * time.Millisecond)
	nudge := tnode(6)
	tlink(nudge, prev)
	tr.add(nudge)
	if tr.holdsSlot(abandoned.id.id) {
		t.Fatalf("the abandoned tip should have left the denominator")
	}
	assertTipIndexIsConsistent(t, tr, "tip expired")

	// Now approve it, and merge every branch so the whole live tip set confirms
	// it. It goes from expired-and-selectable to confirmed.
	x := tnode(7)
	tlink(x, abandoned)
	tr.add(x)
	m := tnode(8)
	tlink(m, x)
	tlink(m, nudge)
	tr.add(m)
	m2 := tnode(9)
	tlink(m2, m)
	tr.add(m2)
	assertTipIndexIsConsistent(t, tr, "expired tip confirmed")

	settled := map[uuid.UUID]bool{}
	for _, n := range tr.pop() {
		settled[n.id.id] = true
	}
	if !settled[abandoned.id.id] {
		t.Fatalf("the abandoned site was never confirmed after being approved")
	}
	for _, n := range tr.getTips() {
		if n.id.id == abandoned.id.id {
			t.Fatalf("a confirmed site is still offered as an approval target")
		}
	}
}

// A commit transaction takes some of the confirmed queue and leaves the rest,
// and a site settled afterwards has to be the site that leaves. The queue is
// compacted by the take, so the position a site sat in beforehand is not the
// position it keeps - an index that is not moved with it removes somebody else's
// site from the queue, and that site then never reaches a commit transaction.
func TestSettlingASiteAfterAPartialTakeRemovesTheRightOne(t *testing.T) {
	tr := newConfirmTracker(0, 1000)
	held := make([]*Node, 0, 6)
	for i := 0; i < 6; i++ {
		n := tnode(i)
		tr.mu.Lock()
		tr.confirmedSet[n.id.id] = len(tr.confirmed)
		tr.confirmed = append(tr.confirmed, n)
		tr.mu.Unlock()
		held = append(held, n)
	}

	// A commit settles the first two, leaving four.
	took := tr.take([]uuid.UUID{held[0].id.id, held[1].id.id})
	if len(took) != 2 {
		t.Fatalf("the commit took %d sites, want 2", len(took))
	}

	// Now settle one of the survivors out of band, as slicing does.
	tr.markHarvested(held[4].id.id)

	left := map[uuid.UUID]bool{}
	for _, n := range tr.peek() {
		left[n.id.id] = true
	}
	if left[held[4].id.id] {
		t.Fatalf("the settled site is still queued for a commit transaction")
	}
	for _, want := range []*Node{held[2], held[3], held[5]} {
		if !left[want.id.id] {
			t.Fatalf("site %s was removed from the queue by settling a different site",
				want.id.id.String())
		}
	}
	if len(left) != 3 {
		t.Fatalf("the queue holds %d sites, want 3", len(left))
	}
}

// Sampling the tip set shuffles it in place, which is the only way to draw
// distinct tips without copying the set first. The index has to move with the
// entries or every later read of the tip set is reading the wrong slot.
func TestSamplingTheTipSetKeepsItsIndexInStep(t *testing.T) {
	tr := newConfirmTracker(0, 1000)
	genesis := tnode(0)
	tr.add(genesis)
	for i := 1; i <= 12; i++ {
		n := tnode(i)
		tlink(n, genesis)
		tr.add(n)
	}
	assertTipIndexIsConsistent(t, tr, "before sampling")

	for round := 0; round < 40; round++ {
		got := tr.sampleTips(3, nil)
		if len(got) != 3 {
			t.Fatalf("round %d: sampled %d tips, want 3", round, len(got))
		}
		seen := map[uuid.UUID]bool{}
		for _, n := range got {
			if seen[n.id.id] {
				t.Fatalf("round %d: the same tip was sampled twice", round)
			}
			seen[n.id.id] = true
		}
		assertTipIndexIsConsistent(t, tr, fmt.Sprintf("after sampling round %d", round))
	}

	// And the index still points at the right sites once one of them is dropped
	// from a position the shuffling moved it to.
	tips := tr.getTips()
	tr.markHarvested(tips[0].id.id)
	assertTipIndexIsConsistent(t, tr, "after dropping a shuffled tip")
	for _, n := range tr.getTips() {
		if n.id.id == tips[0].id.id {
			t.Fatalf("a harvested tip is still in the tip set")
		}
	}
}

// The harvest record is bounded by handing the guarantee to the slice archive,
// not by forgetting: an id is only dropped once the archive holds it, and the
// tracker then consults both. Getting this wrong pays a processor twice.
func TestASettledSiteStaysRefusedAfterTheHarvestRecordIsPruned(t *testing.T) {
	prevArchive := sliceArchive
	sliceArchive = newRamArchive()
	t.Cleanup(func() { sliceArchive = prevArchive })

	tr := newConfirmTracker(0, 1000)

	genesis := tnode(0)
	tr.add(genesis)
	a, b := tnode(1), tnode(2)
	tlink(a, genesis)
	tlink(b, genesis)
	tr.add(a)
	tr.add(b)
	harvested := tr.pop()
	if len(harvested) == 0 {
		t.Fatalf("nothing was confirmed, so there is no harvest record to prune")
	}
	settled := harvested[0]

	// The commit transaction is applied: the site is archived, and only then is
	// the tracker's own record redundant.
	sliceArchive.Archive(1, []*pb.Node{{Id: &pb.Node_NodeId{Id: settled.id.id[:]}}})
	// The prune is amortised against the record's own growth, so on a real node
	// it fires after a few thousand settled sites. Pull the threshold down to
	// the next settle rather than driving thousands of them through.
	tr.mu.Lock()
	tr.harvestPruneAt = 1
	tr.mu.Unlock()
	tr.markHarvested(settled.id.id) // any settle path triggers the prune

	tr.mu.Lock()
	_, stillHeld := tr.harvested[settled.id.id]
	pruned := tr.harvestPruned
	tr.mu.Unlock()
	if !pruned || stillHeld {
		t.Fatalf("the archived id was not pruned out of the harvest record (held=%v pruned=%v)",
			stillHeld, pruned)
	}

	// And the guarantee still holds: a late arrival naming it must not get it
	// tracked, confirmed, or paid a second time. Checked at the region rather
	// than through isTip, because a settled site with approvers on it is not a
	// tip whether the guard held or not - the question is whether it is back in
	// the region at all, since that is what puts it back in the denominator and
	// back in line for a commit transaction.
	tr.add(settled)
	tr.mu.Lock()
	_, back := tr.sites[settled.id.id]
	tr.mu.Unlock()
	if back {
		t.Fatalf("a settled site was tracked again once its harvest record had been pruned")
	}
	c := tnode(3)
	tlink(c, a)
	tlink(c, b)
	tr.add(c)
	for _, n := range tr.pop() {
		if n.id.id == settled.id.id {
			t.Fatalf("a settled site was confirmed a second time after its harvest record was pruned")
		}
	}
}

// A detached site is still part of the path between the sites that approve it
// and the sites it approves, and coverage has to travel through it.
//
// This is the reason a site whose approval targets never arrive cannot simply be
// evicted from the active region to bound it. Marking stops at a site the
// tracker does not hold, on the grounds that such a site is confirmed and so are
// its ancestors; a detached site is the opposite case, and stopping there would
// leave its ancestors short of the coverage they have genuinely been given. They
// would confirm later than the definition says, or not at all.
func TestCoverageTravelsThroughADetachedSite(t *testing.T) {
	tr := newConfirmTracker(0, 1000)
	rootA, rootB := tnode(0), tnode(1)
	tr.add(rootA)
	// A second root, which never has a path to anything on the first one. It
	// keeps the denominator above one, so the site being measured is not
	// confirmed and gone before there is anything to measure.
	tr.add(rootB)

	target := tnode(2)
	tlink(target, rootA)
	tr.add(target)

	// A site that approves the target but is waiting on a target of its own that
	// it has never seen.
	detached := tnode(3)
	tlink(detached, target)
	detached.missingTargets = map[string]bool{uuid.New().String(): true}
	tr.add(detached)

	before, ok := tr.shareOf(target.id.id)
	if !ok {
		t.Fatalf("the target left the active region before the measurement")
	}

	// And a site that approves the detached one. Its coverage has to reach the
	// target, two edges away, through a site that confirms nothing itself.
	approver := tnode(4)
	tlink(approver, detached)
	tr.add(approver)

	after, ok := tr.shareOf(target.id.id)
	if !ok {
		t.Fatalf("the target left the active region: no tip confirms it yet")
	}
	if after <= before {
		t.Fatalf("the share of the target is %v after a site approved the detached site that approves it, was %v: coverage did not travel through the detached site",
			after, before)
	}
}

// A site can be confirmed while it is still selectable, and it has to leave the
// tip set when it is. Selection would otherwise keep offering a site that has
// been settled, and a new site approving one names an approval that no peer can
// resolve once the site has been sliced out of the live graph.
//
// The shape needed to get there is narrow and it is worth writing down. A site
// is normally either a tip, and excluded from confirmation outright, or approved,
// and out of the tip set. Both have to be false at once: the site has to be out
// of the denominator, which only the unapproved-tip timeout does, while the
// tracker still counts nothing as approving it - which is what a detached
// approver is, since a detached site's approvals are not counted until it
// relinks. Coverage still travels through that detached site, so the site's
// share climbs while it is still being offered for approval.
func TestASiteConfirmedWhileStillSelectableLeavesTheTipSet(t *testing.T) {
	tr := newConfirmTracker(30*time.Millisecond, 1000)
	genesis := tnode(0)
	tr.add(genesis)

	stranded := tnode(1)
	tlink(stranded, genesis)
	tr.add(stranded)

	// Nothing approves it for longer than the timeout, so it stops counting
	// towards the denominator while staying selectable.
	time.Sleep(80 * time.Millisecond)
	detached := tnode(2)
	tlink(detached, stranded)
	detached.missingTargets = map[string]bool{uuid.New().String(): true}
	tr.add(detached)
	if tr.holdsSlot(stranded.id.id) {
		t.Fatalf("the unapproved tip should have left the denominator")
	}
	if !tr.isTip(stranded.id.id) {
		t.Fatalf("an expired tip must stay selectable; a detached approver does not count")
	}

	// Coverage now reaches it through the detached site, and there is no live
	// tip that does not confirm it.
	approver := tnode(3)
	tlink(approver, detached)
	tr.add(approver)

	confirmed := map[uuid.UUID]bool{}
	for _, n := range tr.pop() {
		confirmed[n.id.id] = true
	}
	if !confirmed[stranded.id.id] {
		t.Fatalf("the site was not confirmed, so this test is no longer exercising the case it was written for")
	}
	for _, n := range tr.getTips() {
		if n.id.id == stranded.id.id {
			t.Fatalf("a site that has been handed to a commit transaction is still offered as an approval target")
		}
	}
	assertTipIndexIsConsistent(t, tr, "after a selectable site was confirmed")
}

// A site that was approved while it was detached is given a slot again when it
// relinks, which is the tracker's own doing; the next approval has to take that
// slot back off it. Narrowing the retirement condition to the first approval -
// which is all it looks like it needs, since a tip is a site nothing approves -
// leaves such a site counting towards the denominator for good, and the
// denominator is what every confirmation is measured against.
func TestASiteApprovedWhileDetachedLeavesTheDenominatorWhenApprovedAgain(t *testing.T) {
	tr := newConfirmTracker(0, 1000)
	genesis := tnode(0)
	tr.add(genesis)

	// A site arrives before one of the sites it approves.
	detached := tnode(1)
	tlink(detached, genesis)
	detached.missingTargets = map[string]bool{uuid.New().String(): true}
	tr.add(detached)
	if tr.holdsSlot(detached.id.id) {
		t.Fatalf("a detached site must not count towards the denominator")
	}

	// Something approves it while it is still detached.
	first := tnode(2)
	tlink(first, detached)
	tr.add(first)
	if approvers := approversOf(t, tr, detached); approvers != 1 {
		t.Fatalf("the detached site has %d approvers, want 1", approvers)
	}

	// Its missing target arrives, so it joins the graph - and the tracker gives
	// it a slot.
	detached.missingTargets = nil
	tr.relink(detached)

	// A second approval. Whatever the site's history, something approves it, so
	// it is not a tip and must not be in the denominator.
	second := tnode(3)
	tlink(second, detached)
	tr.add(second)
	if tr.holdsSlot(detached.id.id) {
		t.Fatalf("a site with two approvers still counts towards the confirmation denominator")
	}
	if tr.isTip(detached.id.id) {
		t.Fatalf("a site with two approvers is still offered as an approval target")
	}
	assertTipIndexIsConsistent(t, tr, "after the second approval")
}

// The other half of the bound: a harvest the archive does not hold yet must not
// be dropped. That is the window between a site being handed to a commit
// transaction and that transaction being applied, and forgetting an id inside it
// puts the site back in the tip set - which is what pays for it twice.
func TestAnUnarchivedHarvestIsNotForgottenByThePrune(t *testing.T) {
	prevArchive := sliceArchive
	sliceArchive = newRamArchive()
	t.Cleanup(func() { sliceArchive = prevArchive })

	tr := newConfirmTracker(0, 1000)
	applied, inFlight := tnode(1), tnode(2)
	tr.markHarvested(applied.id.id)
	tr.markHarvested(inFlight.id.id)

	// Only one of the two commit transactions has been applied.
	sliceArchive.Archive(1, []*pb.Node{{Id: &pb.Node_NodeId{Id: applied.id.id[:]}}})

	tr.mu.Lock()
	tr.harvestPruneAt = 1
	tr.pruneHarvested()
	_, appliedHeld := tr.harvested[applied.id.id]
	_, inFlightHeld := tr.harvested[inFlight.id.id]
	tr.mu.Unlock()

	if appliedHeld {
		t.Fatalf("an archived harvest was kept, so the record is not bounded at all")
	}
	if !inFlightHeld {
		t.Fatalf("a harvest the archive does not hold was forgotten: that site can be confirmed and paid twice")
	}

	tr.add(inFlight)
	tr.mu.Lock()
	_, back := tr.sites[inFlight.id.id]
	tr.mu.Unlock()
	if back {
		t.Fatalf("a harvested site the archive has not seen yet was tracked again")
	}
}

// Nothing is pruned while there is no archive to prune into, because then the
// map is the only record there is.
func TestTheHarvestRecordIsKeptWhenThereIsNoArchive(t *testing.T) {
	prevArchive := sliceArchive
	sliceArchive = nil
	t.Cleanup(func() { sliceArchive = prevArchive })

	tr := newConfirmTracker(0, 1000)
	tr.harvestPruneAt = 1
	id := uuid.New()
	tr.markHarvested(id)

	tr.mu.Lock()
	_, held := tr.harvested[id]
	pruned := tr.harvestPruned
	tr.mu.Unlock()
	if !held || pruned {
		t.Fatalf("the harvest record was dropped with no archive holding it (held=%v pruned=%v)", held, pruned)
	}
}

// ---------------------------------------------------------------- benchmarks

/*
The frontier these benchmarks run against is built by concurrent arrival, and it
has to be.

A graph grown one site at a time cannot have more than one tip. Each new site is
the only tip there is, so the next site has nothing else to approve, approving it
retires it, and the tip set never leaves one - a chain, not a DAG. Every number
measured on such a graph is flat in the size of the graph and says nothing about
work that scales with the frontier: the tip set has one member, so scanning the
active region for it costs nothing and choosing between candidates is not a
choice. That is exactly the shape the first version of this measurement had, and
it reported the same 2.8us at a hundred sites and at five thousand.

A real network publishes several sites against the same view of the graph,
because a site published by one node is not visible to the others until it has
propagated. That is what widens the tip set, and it is the arrival pattern
TestConfirmationConvergesUnderConcurrentArrival uses for the same reason.
*/

// growConcurrentFrontier - a graph grown by real tip selection, fanout sites at
// a time against one view, drained as it goes. Returns the dag and its tracker.
func growConcurrentFrontier(tb testing.TB, sites, fanout int) (*Dag, *ConfirmTracker) {
	tb.Helper()
	prevApprove, prevDepth := dagConfig.Approvetx, dagConfig.Walkdepth
	dagConfig.Approvetx, dagConfig.Walkdepth = 2, 10
	tr := newConfirmTracker(0, DAG_CONFIRM_SHARE)
	prev := confirmationCounter
	confirmationCounter = tr
	tb.Cleanup(func() {
		confirmationCounter = prev
		dagConfig.Approvetx, dagConfig.Walkdepth = prevApprove, prevDepth
	})

	genesis := tnode(0)
	d := &Dag{genesis: genesis}
	tr.add(genesis)
	for id := 1; id <= sites; {
		batch := make([][]*Node, 0, fanout)
		for k := 0; k < fanout && id+k <= sites; k++ {
			targets := d.selectTips(0.5)
			if len(targets) == 0 {
				tb.Fatalf("selection offered nothing to approve after %d sites", id)
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
		// Drained, so the timed loop is not paying to empty a backlog the
		// fixture left behind. An undrained queue made an earlier version of
		// this look as if the insert path grew with the size of the graph.
		tr.pop()
	}
	return d, tr
}

func (c frontierCase) name() string {
	return fmt.Sprintf("sites=%d/fanout=%d", c.sites, c.fanout)
}

// reportFrontier - the frontier and tip counts the timings were taken against,
// and a floor under them. A fixture that quietly collapses to a chain has to
// fail rather than report a fast, meaningless number.
func reportFrontier(b *testing.B, tr *ConfirmTracker, fanout int) {
	active, tips, _ := tr.stats()
	selectable := len(tr.getTips())
	b.ReportMetric(float64(active), "frontier")
	b.ReportMetric(float64(selectable), "tips")
	floor := fanout / 2
	if floor < 2 {
		floor = 2
	}
	if selectable < floor {
		b.Fatalf("the fixture built %d tips at fanout %d (frontier %d, denominator %d): that is a chain, not a frontier, and nothing measured on it means anything",
			selectable, fanout, active, tips)
	}
}

// BenchmarkTipSelectionOnAFrontier - what runs once per inserted site, on a tip
// set wide enough for the cost of reading it to show.
func BenchmarkTipSelectionOnAFrontier(b *testing.B) {
	for _, c := range frontierCases {
		b.Run(c.name(), func(b *testing.B) {
			d, tr := growConcurrentFrontier(b, c.sites, c.fanout)
			reportFrontier(b, tr, c.fanout)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if got := d.selectTips(0.5); len(got) == 0 {
					b.Fatal("selection offered nothing to approve")
				}
			}
			b.StopTimer()
			reportFrontier(b, tr, c.fanout)
		})
	}
}

type frontierCase struct {
	sites  int
	fanout int
}

// frontierCases - the graph sizes and arrival concurrencies the frontier
// benchmarks run against. Concurrency is the dimension that widens the tip set;
// the size is there to show that neither the frontier nor the timings track it.
var frontierCases = []frontierCase{
	{100, 8}, {1000, 8}, {5000, 8},
	{1000, 64}, {5000, 64},
}

// BenchmarkGraphGrowthOnAFrontier - selection and confirmation together, which
// is the pair that runs inside the insert path's critical section.
//
// One operation is one round of fanout arrivals, not one site, and that is not a
// convenience: inserting one site per operation is sequential arrival, and
// sequential arrival collapses the tip set to one member within a few hundred
// sites however wide the fixture built it. The benchmark would then be
// measuring a chain again, a few thousand operations in, and reporting a number
// that got faster as the graph it was measuring stopped existing. ns/site is
// reported alongside so the per-insert cost is still readable.
func BenchmarkGraphGrowthOnAFrontier(b *testing.B) {
	for _, c := range frontierCases {
		b.Run(c.name(), func(b *testing.B) {
			d, tr := growConcurrentFrontier(b, c.sites, c.fanout)
			reportFrontier(b, tr, c.fanout)
			id := c.sites
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				batch := make([][]*Node, 0, c.fanout)
				for k := 0; k < c.fanout; k++ {
					batch = append(batch, d.selectTips(0.5))
				}
				for _, targets := range batch {
					id++
					n := tnode(id)
					for _, target := range targets {
						tlink(n, target)
					}
					tr.add(n)
				}
				tr.pop()
			}
			b.StopTimer()
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*c.fanout), "ns/site")
			reportFrontier(b, tr, c.fanout)
		})
	}
}

// BenchmarkConfirmTrackerGetTips - the whole-set read. Off the per-insert path
// now, but it is what Dag.GetTips and the legacy fallback still use, and it is
// the measurement that says whether the read scales with the tip set or with
// the active region.
func BenchmarkConfirmTrackerGetTips(b *testing.B) {
	for _, c := range frontierCases {
		b.Run(c.name(), func(b *testing.B) {
			_, tr := growConcurrentFrontier(b, c.sites, c.fanout)
			reportFrontier(b, tr, c.fanout)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if len(tr.getTips()) == 0 {
					b.Fatal("no tips")
				}
			}
			b.StopTimer()
			reportFrontier(b, tr, c.fanout)
		})
	}
}

// BenchmarkConfirmTrackerSettlesACommit - settling one commit transaction's
// worth of sites, which is what slicing drives once per commit. Every site
// settled used to rescan and memmove the confirmed queue.
func BenchmarkConfirmTrackerSettlesACommit(b *testing.B) {
	for _, sites := range []int{1000, 5000} {
		b.Run(fmt.Sprintf("sites=%d", sites), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				tr, ids := trackerHoldingConfirmed(b, sites)
				b.StartTimer()
				for _, id := range ids {
					tr.markHarvested(id)
				}
			}
		})
	}
}

// trackerHoldingConfirmed - a tracker with n sites confirmed and not yet
// claimed. Grown as a chain on purpose: the queue is what is being measured, and
// a chain fills it with one insert per confirmed site.
func trackerHoldingConfirmed(tb testing.TB, n int) (*ConfirmTracker, []uuid.UUID) {
	tb.Helper()
	tr := newConfirmTracker(0, 1000)
	genesis := tnode(0)
	tr.add(genesis)
	prev := genesis
	for i := 1; ; i++ {
		x := tnode(i)
		tlink(x, prev)
		tr.add(x)
		prev = x
		if _, _, pending := tr.stats(); pending >= n {
			break
		}
	}
	held := tr.peek()
	ids := make([]uuid.UUID, 0, len(held))
	for _, s := range held {
		ids = append(ids, s.id.id)
	}
	return tr, ids
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
