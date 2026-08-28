package dag

import (
	"sync"
	"time"

	"github.com/Grape-Chain/Grape-Dag/stats"

	"github.com/google/uuid"
)

/*
ConfirmTracker implements the confirmation rule from the technical paper's
section 5.1: for every site that is not yet confirmed, track the share of the
DAG's current vertices (tips) that confirm it, directly or indirectly. When that
share reaches 100% - every current tip has a path to the site - the site is
irrevocably confirmed and is handed to the next commit transaction.

What it replaces: a site used to count as confirmed as soon as two other sites
referenced it directly. That is a fixed local threshold, not a share, and it says
nothing about the rest of the graph: a site could be "confirmed" while most of
the DAG had never heard of it.

How the share is maintained without walking the graph on every insert:

  - Every tip owns a slot, an index into a bit vector. Each tracked site carries
    one bit per slot, set when that tip confirms the site.

  - A new site is a tip, so it takes a fresh slot, and every one of its
    ancestors gains that bit. Marking walks backwards from its approval targets
    and stops as soon as it meets a site that already carries the bit - if a
    site has it, so do all of its ancestors, because bits are always set over a
    complete backward closure.

  - A site stops being a tip the moment anything approves it. Its slot is then
    cleared from its ancestors the same way (stopping where the bit is already
    clear) and returned to the free list.

    This is the ordinary meaning of "tip" - a vertex nothing points at - and it
    is not the same as dag.approvetx, which says how many sites a new site
    approves. Treating a site as a tip until approvetx sites referenced it,
    which is what this did first, kept partly approved sites in the denominator
    and made them the sites selection kept landing on, so the genuinely new tips
    were never approved and never left. Measured, with sites arriving
    concurrently: 10 of 6000 confirmed against 5980 once the definitions were
    separated.

  - A site is confirmed when its bit count equals the number of live tips.
    Sites are bucketed by bit count, so the check is a lookup rather than a scan.

Two consequences worth stating, because they are what bound the work:

  - Confirmation is closed downwards. If every tip confirms a site, every tip
    also confirms that site's ancestors, so they are confirmed too. Marking can
    therefore stop at a confirmed site, and confirmed sites can leave the
    tracker. What is left is the frontier, not the ledger.

  - A tip that is never approved would otherwise hold its slot forever and stop
    anything newer from ever reaching 100%. Tip selection is supposed to prevent
    that; to keep the ledger live when it does not, a tip that has gone
    unapproved for longer than dag.tiptimeout stops counting towards the
    denominator. It keeps being offered for approval, which is what lets it
    eventually be approved and confirmed - dropping it from selection as well
    would strand its transaction outside every commit transaction. Set
    dag.tiptimeout to 0 to disable the valve entirely.

    This is a deliberate departure from the paper, which assumes tips are always
    approved: while a tip is expired its own confirmations are not counted, so
    the share of its ancestors is measured against the tips that remain.
*/
type ConfirmTracker struct {
	mu sync.Mutex

	// sites - the active region: everything tracked but not yet confirmed.
	sites map[uuid.UUID]*siteTrack
	// byCount - sites grouped by how many tips confirm them, so finding the
	// ones at 100% is a map lookup instead of a scan over the active region.
	byCount map[int]map[uuid.UUID]struct{}

	// slots - slot index to the tip that owns it; freeSlots recycles them.
	slots     []uuid.UUID
	freeSlots []int
	tipCount  int

	// confirmed - reached 100%, waiting to be written into a commit tx.
	confirmed    []*Node
	confirmedSet map[uuid.UUID]struct{}
	// harvested - already written into a commit tx. A site can be referenced as
	// an approval target long after it was pinned (targets are resolved out of
	// past pins), and without this it would re-enter the tip set and be
	// confirmed - and, once fees land, paid - twice.
	harvested map[uuid.UUID]struct{}

	// confirmShare - the share of live tips that must confirm a site before it
	// is irrevocably confirmed, in permille. 1000 is the technical paper's
	// literal 100%.
	//
	// Below 1000 this is a deliberate departure, and the reason is measured
	// rather than assumed: the 100% rule cannot converge once the tip set is
	// large, because it needs every one of the live tips to cover a site while
	// new tips keep arriving. With sites arriving concurrently - several chosen
	// against a view that does not yet contain each other, which is what any
	// real network produces - the tip set grows and confirmation stops. See
	// TestConfirmationConvergesUnderConcurrentArrival.
	confirmShare uint16
	tipTimeout   time.Duration
	lastSweepAt  time.Time
}

type siteTrack struct {
	node      *Node
	cover     bitset
	count     int // popcount(cover), maintained incrementally
	approvers int
	slot      int  // -1 when this site does not count towards the denominator
	expired   bool // dropped from the denominator for going unapproved, still selectable
	detached  bool
	tipSince  time.Time
}

func newConfirmTracker(tipTimeout time.Duration, share uint16) *ConfirmTracker {
	if share == 0 || share > 1000 {
		share = DAG_CONFIRM_SHARE
	}
	return &ConfirmTracker{
		sites:        make(map[uuid.UUID]*siteTrack),
		byCount:      make(map[int]map[uuid.UUID]struct{}),
		confirmedSet: make(map[uuid.UUID]struct{}),
		harvested:    make(map[uuid.UUID]struct{}),
		confirmShare: share,
		tipTimeout:   tipTimeout,
	}
}

// ---------------------------------------------------------------- bookkeeping

func (c *ConfirmTracker) bucket(id uuid.UUID, count int) {
	m, ok := c.byCount[count]
	if !ok {
		m = make(map[uuid.UUID]struct{})
		c.byCount[count] = m
	}
	m[id] = struct{}{}
}

func (c *ConfirmTracker) unbucket(id uuid.UUID, count int) {
	if m, ok := c.byCount[count]; ok {
		delete(m, id)
		if len(m) == 0 {
			delete(c.byCount, count)
		}
	}
}

func (c *ConfirmTracker) recount(id uuid.UUID, tr *siteTrack, delta int) {
	c.unbucket(id, tr.count)
	tr.count += delta
	c.bucket(id, tr.count)
}

// countApprovals - record that this site approves its targets, retiring any that
// have reached the threshold. Called exactly once per site, at the point it
// becomes part of the graph. Caller holds the lock.
//
// Duplicate targets are counted once. Nothing on the receiving side checks that
// a peer named distinct approval targets, and two entries for the same site
// would otherwise count as two approvals of it.
func (c *ConfirmTracker) countApprovals(vertex *Node) {
	var seen map[uuid.UUID]struct{}
	if len(vertex.targets) > 1 {
		seen = make(map[uuid.UUID]struct{}, len(vertex.targets))
	}
	for _, t := range vertex.targets {
		if t == nil {
			continue
		}
		if seen != nil {
			if _, dup := seen[t.id.id]; dup {
				continue
			}
			seen[t.id.id] = struct{}{}
		}
		ttr, ok := c.sites[t.id.id]
		if !ok {
			continue
		}
		ttr.approvers++
		if ttr.approvers >= 1 {
			// One approval is enough: a tip is a site nothing approves.
			c.retireTip(t.id.id, ttr)
		}
	}
}

func (c *ConfirmTracker) takeSlot(id uuid.UUID) int {
	if n := len(c.freeSlots); n > 0 {
		slot := c.freeSlots[n-1]
		c.freeSlots = c.freeSlots[:n-1]
		c.slots[slot] = id
		return slot
	}
	c.slots = append(c.slots, id)
	return len(c.slots) - 1
}

func (c *ConfirmTracker) releaseSlot(slot int) {
	if slot < 0 || slot >= len(c.slots) {
		return
	}
	c.slots[slot] = uuid.Nil
	c.freeSlots = append(c.freeSlots, slot)
}

// ---------------------------------------------------------------- marking

// markAncestors - give every ancestor of the given sites the bit for slot.
// Stops at sites the tracker no longer holds (already confirmed, so their
// ancestors are confirmed too) and at sites that already carry the bit (so do
// their ancestors, since bits are always set over a full backward closure).
func (c *ConfirmTracker) markAncestors(from []*Node, slot int) {
	if slot < 0 {
		return
	}
	stack := append([]*Node(nil), from...)
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n == nil {
			continue
		}
		tr, ok := c.sites[n.id.id]
		if !ok {
			continue
		}
		if !tr.cover.set(slot) {
			continue
		}
		c.recount(n.id.id, tr, 1)
		stack = append(stack, n.targets...)
	}
}

// unmarkAncestors - the mirror of markAncestors, for a tip that has stopped
// counting: take its bit back off its ancestors.
func (c *ConfirmTracker) unmarkAncestors(from []*Node, slot int) {
	if slot < 0 {
		return
	}
	stack := append([]*Node(nil), from...)
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n == nil {
			continue
		}
		tr, ok := c.sites[n.id.id]
		if !ok {
			continue
		}
		if !tr.cover.clear(slot) {
			continue
		}
		c.recount(n.id.id, tr, -1)
		stack = append(stack, n.targets...)
	}
}

// retireTip - stop counting a site as a current vertex.
func (c *ConfirmTracker) retireTip(id uuid.UUID, tr *siteTrack) {
	if tr.slot < 0 {
		return
	}
	slot := tr.slot
	tr.slot = -1
	c.unmarkAncestors(tr.node.targets, slot)
	c.releaseSlot(slot)
	c.tipCount--
}

// ---------------------------------------------------------------- confirming

// sweep - move every site now confirmed by all live tips into the confirmed
// queue. Confirmation is closed downwards, so a confirmed site can leave the
// tracker: later marking walks stop there.
// confirmationThreshold - how many of the live tips must confirm a site before
// it counts as confirmed. Caller holds the lock.
func (c *ConfirmTracker) confirmationThreshold() int {
	if c.tipCount <= 0 {
		return 0
	}
	if c.confirmShare >= 1000 {
		return c.tipCount
	}
	// Round up, so a share below 100% never means "fewer tips than the share
	// asks for", and never means zero while any tip exists.
	need := (c.tipCount*int(c.confirmShare) + 999) / 1000
	if need < 1 {
		need = 1
	}
	return need
}

func (c *ConfirmTracker) sweep() {
	for {
		need := c.confirmationThreshold()
		if need == 0 {
			// No live tips means no denominator: nothing is confirmed yet.
			return
		}
		// At 100% this is the single bucket the count has to land in. Below it,
		// every bucket from the threshold up qualifies - bounded by the number
		// of live tips, and only walked when a bucket at or above the threshold
		// is actually occupied.
		promoted := make([]uuid.UUID, 0)
		for count := need; count <= c.tipCount; count++ {
			for id := range c.byCount[count] {
				promoted = append(promoted, id)
			}
		}
		if len(promoted) == 0 {
			return
		}
		progressed := false
		for _, id := range promoted {
			tr, ok := c.sites[id]
			if !ok {
				continue
			}
			if tr.detached || tr.slot >= 0 {
				// A detached site has unresolved targets, so its coverage is not
				// yet meaningful. A tip is excluded outright: it does not confirm
				// itself, and below a 100% share its count can reach the
				// threshold anyway, so this is what keeps a tip from confirming
				// itself into a commit transaction.
				continue
			}
			c.unbucket(id, tr.count)
			delete(c.sites, id)
			if _, done := c.harvested[id]; done {
				continue
			}
			if _, queued := c.confirmedSet[id]; queued {
				continue
			}
			c.confirmedSet[id] = struct{}{}
			c.confirmed = append(c.confirmed, tr.node)
			progressed = true
		}
		if !progressed {
			return
		}
	}
}

// expireStaleTips - drop tips that have gone unapproved for too long from the
// denominator, so one abandoned tip cannot stall confirmation for the whole
// ledger. Caller holds the lock.
func (c *ConfirmTracker) expireStaleTips(now time.Time) {
	if c.tipTimeout <= 0 {
		return
	}
	// Cheap rate limit so this does not run on every insert. Tied to the
	// timeout, or a short timeout would never be observed.
	every := time.Second
	if half := c.tipTimeout / 2; half < every {
		every = half
	}
	if now.Sub(c.lastSweepAt) < every {
		return
	}
	c.lastSweepAt = now
	for _, id := range c.slots {
		if id == uuid.Nil {
			continue
		}
		tr, ok := c.sites[id]
		if !ok || tr.slot < 0 {
			continue
		}
		if now.Sub(tr.tipSince) > c.tipTimeout {
			logger.Debugf("[confirmation] Tip %s unapproved for %s, dropping it from the denominator",
				id.String(), now.Sub(tr.tipSince).Truncate(time.Second))
			tr.expired = true
			stats.TipsExpired.Inc()
			c.retireTip(id, tr)
		}
	}
}

// ---------------------------------------------------------------- public API

func (c *ConfirmTracker) add(vertex *Node) {
	if vertex == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.track(vertex)
}

// track - register a site and fold it into the coverage bookkeeping.
// Caller holds the lock.
func (c *ConfirmTracker) track(vertex *Node) {
	id := vertex.id.id
	if _, done := c.harvested[id]; done {
		return // already pinned; do not resurrect it
	}
	if _, queued := c.confirmedSet[id]; queued {
		return
	}
	if existing, ok := c.sites[id]; ok {
		// Re-registered: the same site can arrive down more than one path. Keep
		// the coverage we already have, but pick up newly resolved targets.
		if existing.detached && len(vertex.missingTargets) == 0 {
			c.resolve(existing, vertex)
		}
		return
	}

	now := time.Now()
	tr := &siteTrack{
		node:      vertex,
		slot:      -1,
		detached:  len(vertex.missingTargets) > 0,
		approvers: len(vertex.sources),
		tipSince:  now,
	}
	c.sites[id] = tr
	c.bucket(id, 0)

	if !tr.detached {
		tr.slot = c.takeSlot(id)
		c.tipCount++
		c.markAncestors(vertex.targets, tr.slot)
		// The sites this one approves are one approval closer to retirement.
		//
		// Counted here only for a site that is actually part of the graph. A
		// detached site is waiting on approval targets it has never seen, so it
		// confirms nothing; letting its approval retire a tip would take that
		// tip out of the denominator without putting any coverage in, which is
		// the fixed-threshold failure this rule exists to remove. Worse, the
		// approval would then be counted a second time when the site is
		// resolved - resolve() runs the same loop over the same targets - so a
		// tip retired after one real approver instead of two. On a peer that is
		// out of sync, which is the ordinary case, that made sites confirmed
		// that the rest of the network had approved once.
		c.countApprovals(vertex)
	}

	c.expireStaleTips(now)
	c.sweep()
}

// resolve - a site that was inserted with unresolved targets has been relinked,
// so it can start confirming. Caller holds the lock.
func (c *ConfirmTracker) resolve(tr *siteTrack, vertex *Node) {
	tr.detached = false
	tr.node = vertex
	tr.tipSince = time.Now()
	if tr.slot < 0 {
		tr.slot = c.takeSlot(vertex.id.id)
		c.tipCount++
	}
	c.markAncestors(vertex.targets, tr.slot)
	// Now that it is part of the graph, its approvals count. They were not
	// counted while it was detached, so this is the first and only time.
	c.countApprovals(vertex)
	c.sweep()
}

// relink - called once a site's missing approval targets have been resolved.
func (c *ConfirmTracker) relink(vertex *Node) {
	if vertex == nil || len(vertex.missingTargets) > 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if tr, ok := c.sites[vertex.id.id]; ok && tr.detached {
		c.resolve(tr, vertex)
		return
	}
	c.track(vertex)
}

// pop - hand over every site confirmed since the last call. Sites are recorded
// as harvested, so a later reference cannot get them confirmed twice.
func (c *ConfirmTracker) pop() []*Node {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.expireStaleTips(time.Now())
	c.sweep()
	if len(c.confirmed) == 0 {
		return []*Node{}
	}
	out := make([]*Node, 0, len(c.confirmed))
	for _, n := range c.confirmed {
		if n == nil {
			continue
		}
		id := n.id.id
		if _, done := c.harvested[id]; done {
			continue
		}
		c.harvested[id] = struct{}{}
		delete(c.confirmedSet, id)
		out = append(out, n)
	}
	c.confirmed = c.confirmed[:0]
	return out
}

// peek - the confirmed sites, without consuming them.
//
// A validator reports what it holds confirmed before anything is agreed, and
// several times over if the round has to be repeated. Reporting through pop()
// would consume the set on the first report, so a round that failed would leave
// the node with nothing to settle and the sites it had already harvested would
// never reach a commit transaction.
func (c *ConfirmTracker) peek() []*Node {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.expireStaleTips(time.Now())
	c.sweep()
	out := make([]*Node, 0, len(c.confirmed))
	for _, n := range c.confirmed {
		if n == nil {
			continue
		}
		if _, done := c.harvested[n.id.id]; done {
			continue
		}
		out = append(out, n)
	}
	return out
}

// take - consume exactly these sites and no others, which is what a commit
// transaction settles. Sites confirmed while the round was being agreed are not
// in the agreed set, so they stay for the next commit transaction rather than
// being harvested by a pin that does not name them.
func (c *ConfirmTracker) take(ids []uuid.UUID) []*Node {
	c.mu.Lock()
	defer c.mu.Unlock()
	want := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		want[id] = struct{}{}
	}
	out := make([]*Node, 0, len(ids))
	kept := c.confirmed[:0]
	for _, n := range c.confirmed {
		if n == nil {
			continue
		}
		id := n.id.id
		if _, ok := want[id]; !ok {
			kept = append(kept, n)
			continue
		}
		if _, done := c.harvested[id]; done {
			continue
		}
		c.harvested[id] = struct{}{}
		delete(c.confirmedSet, id)
		out = append(out, n)
	}
	c.confirmed = kept
	return out
}

// isTip - whether a site may still be picked as an approval target: nothing
// approves it yet. A tip dropped from the denominator for going unapproved stays
// selectable, which is how it eventually gets approved and confirmed rather than
// stranding its transaction.
func (c *ConfirmTracker) isTip(id uuid.UUID) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	tr, ok := c.sites[id]
	return ok && !tr.detached && tr.approvers == 0
}

func (c *ConfirmTracker) getTips() []*Node {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.expireStaleTips(time.Now())
	tips := make([]*Node, 0, c.tipCount)
	for _, tr := range c.sites {
		if tr.node == nil || tr.detached || tr.approvers != 0 {
			continue
		}
		tips = append(tips, tr.node)
	}
	return tips
}

func (c *ConfirmTracker) tip() []*Node { return c.getTips() }

func (c *ConfirmTracker) markHarvested(id uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.harvested[id] = struct{}{}
	delete(c.confirmedSet, id)
	if tr, ok := c.sites[id]; ok {
		c.retireTip(id, tr)
		c.unbucket(id, tr.count)
		delete(c.sites, id)
	}
	for i, n := range c.confirmed {
		if n != nil && n.id.id == id {
			c.confirmed = append(c.confirmed[:i], c.confirmed[i+1:]...)
			break
		}
	}
}

// holdsSlot - whether this site currently counts towards the confirmation
// denominator. Distinct from isTip, which is about selectability.
func (c *ConfirmTracker) holdsSlot(id uuid.UUID) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	tr, ok := c.sites[id]
	return ok && tr.slot >= 0
}

// stats - for tests and diagnostics.
func (c *ConfirmTracker) stats() (active, tips, pending int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.sites), c.tipCount, len(c.confirmed)
}

// shareOf - the fraction of current tips that confirm the given site, in
// [0,1]. Reports false when the site is not being tracked (already confirmed,
// pinned, or unknown).
func (c *ConfirmTracker) shareOf(id uuid.UUID) (float64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	tr, ok := c.sites[id]
	if !ok || c.tipCount == 0 {
		return 0, false
	}
	return float64(tr.count) / float64(c.tipCount), true
}

// ------------------------------------------------------------- tip selection

// walkFrom - one step's worth of the active region as seen from a site: whether
// it may be approved, how much of the graph confirms it, and the sites that
// approve it and are still in the region with the same measure for each.
//
// Returned together because a walk takes many steps and each step would
// otherwise take the lock several times. The potential is the site's
// confirmation count - the number of current tips that confirm it, the same
// quantity section 5.1 measures. It is non-increasing as the walk moves towards
// the tips: a tip that confirms a site confirms all of that site's ancestors.
func (c *ConfirmTracker) walkFrom(from *Node) (bool, int, []*Node, []int) {
	if from == nil {
		return false, 0, nil, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	tr, ok := c.sites[from.id.id]
	if !ok {
		// Confirmed, pinned, or never tracked: outside the region the walk runs
		// over, so it is neither selectable nor a place to step from.
		return false, 0, nil, nil
	}
	selectable := !tr.detached && tr.approvers == 0
	next := make([]*Node, 0, len(from.sources))
	pot := make([]int, 0, len(from.sources))
	for _, s := range from.sources {
		if s == nil {
			continue
		}
		str, inside := c.sites[s.id.id]
		if !inside || str.node == nil {
			continue
		}
		next = append(next, str.node)
		pot = append(pot, str.count)
	}
	return selectable, tr.count, next, pot
}

// walkBack - the sites this one approves that are still in the active region.
// Stepping backwards along these is how a walk particle is thrown a bounded
// depth below the tips; running out of them means the region floor, which is
// where the particle stops.
func (c *ConfirmTracker) walkBack(from *Node) []*Node {
	if from == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*Node, 0, len(from.targets))
	for _, t := range from.targets {
		if t == nil {
			continue
		}
		tr, inside := c.sites[t.id.id]
		if !inside || tr.node == nil {
			continue
		}
		out = append(out, tr.node)
	}
	return out
}
