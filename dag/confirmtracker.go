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
	// ones at 100% is a lookup instead of a scan over the active region.
	//
	// Indexed by the count itself rather than keyed by it. A count cannot
	// exceed the number of live tips, so this is frontier-sized, and the sweep
	// probes a run of buckets on every insert - slice indexing rather than
	// hashing an int for each of them. An emptied bucket keeps its map instead
	// of being freed: every insert moves sites from one bucket to the next, and
	// re-allocating the map they land in was an allocation per insert.
	byCount []map[uuid.UUID]struct{}
	// maxCount - an upper bound on the highest occupied bucket, trimmed lazily
	// at the start of a sweep. Without it the sweep probes every bucket between
	// the threshold and the tip count, most of them empty.
	maxCount int
	// promoted - scratch for the sweep, which runs once per insert. Reused
	// rather than allocated per call; it is only ever touched under the lock.
	promoted []uuid.UUID

	// slots - slot index to the tip that owns it; freeSlots recycles them.
	slots     []uuid.UUID
	freeSlots []int
	tipCount  int

	// tipRing, tipAt - the selectable tip set as an array plus an index into
	// it, so a uniformly chosen tip costs one random draw and no allocation.
	//
	// Selection asks for a random tip several times per inserted site, and
	// reading the set by scanning the active region made each of those O(active
	// region) with a fresh slice to show for it. Membership here is exactly the
	// predicate that scan applied - tracked, not detached, nothing approves it -
	// maintained at the four points where those can change, so the answer is
	// the same and only the cost differs. A tip dropped from the denominator
	// for going unapproved is still a member: it stays selectable, which is
	// what lets it eventually be approved.
	tipRing []*siteTrack
	tipAt   map[uuid.UUID]int

	// confirmed - reached the threshold, waiting to be written into a commit tx.
	// A settled site is removed by nilling its slot rather than by closing the
	// gap, so a commit that settles n sites does not memmove the queue n times.
	confirmed []*Node
	// confirmedSet - id to its slot in confirmed, which is what makes that
	// removal a lookup. It was a plain set, and markHarvested scanned the whole
	// queue for every settled site.
	confirmedSet   map[uuid.UUID]int
	confirmedHoles int
	// harvested - already written into a commit tx. A site can be referenced as
	// an approval target long after it was pinned (targets are resolved out of
	// past pins), and without this it would re-enter the tip set and be
	// confirmed - and, once fees land, paid - twice.
	//
	// Bounded by handing the guarantee over to the slice archive rather than by
	// forgetting; see pruneHarvested.
	harvested      map[uuid.UUID]struct{}
	harvestPruneAt int
	harvestPruned  bool

	// markStack, unmarkStack - scratch for the two backward closures. Separate
	// so that neither has to reason about the other being in progress; each is
	// used by one function that does not call the other.
	markStack   []*Node
	unmarkStack []*Node

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
		sites:          make(map[uuid.UUID]*siteTrack),
		tipAt:          make(map[uuid.UUID]int),
		confirmedSet:   make(map[uuid.UUID]int),
		harvested:      make(map[uuid.UUID]struct{}),
		harvestPruneAt: harvestPruneFloor,
		confirmShare:   share,
		tipTimeout:     tipTimeout,
	}
}

// ---------------------------------------------------------------- bookkeeping

func (c *ConfirmTracker) bucket(id uuid.UUID, count int) {
	if count < 0 {
		return
	}
	for len(c.byCount) <= count {
		c.byCount = append(c.byCount, nil)
	}
	m := c.byCount[count]
	if m == nil {
		m = make(map[uuid.UUID]struct{})
		c.byCount[count] = m
	}
	m[id] = struct{}{}
	if count > c.maxCount {
		c.maxCount = count
	}
}

func (c *ConfirmTracker) unbucket(id uuid.UUID, count int) {
	if count < 0 || count >= len(c.byCount) {
		return
	}
	delete(c.byCount[count], id)
}

// ------------------------------------------------------------------- tip set

// addTip - record a site as selectable. Idempotent, because the callers that
// know a site's membership may have changed do not all know what it was.
func (c *ConfirmTracker) addTip(id uuid.UUID, tr *siteTrack) {
	if tr == nil || tr.node == nil {
		// The scan this replaces skipped a track with no site behind it, so
		// this does too rather than hand selection a nil to approve.
		return
	}
	if _, in := c.tipAt[id]; in {
		return
	}
	c.tipAt[id] = len(c.tipRing)
	c.tipRing = append(c.tipRing, tr)
}

// dropTip - the site is no longer selectable. Swap-remove, so the cost does not
// depend on where in the ring it sat; the order of the ring is not meaningful,
// every reader of it either picks at random or starts at a random offset.
func (c *ConfirmTracker) dropTip(id uuid.UUID) {
	i, in := c.tipAt[id]
	if !in {
		return
	}
	delete(c.tipAt, id)
	last := len(c.tipRing) - 1
	if i != last {
		moved := c.tipRing[last]
		c.tipRing[i] = moved
		c.tipAt[moved.node.id.id] = i
	}
	c.tipRing[last] = nil
	c.tipRing = c.tipRing[:last]
}

// untrack - the site has left the active region. One place, because every
// caller has to keep the bucket, the tip ring and the region in step.
func (c *ConfirmTracker) untrack(id uuid.UUID, tr *siteTrack) {
	c.unbucket(id, tr.count)
	c.dropTip(id)
	delete(c.sites, id)
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
// The duplicate check is a scan over the targets already counted rather than a
// map. dag.approvetx is 2 or 3, so the map was an allocation per inserted site
// to hold two entries; the crossover where hashing wins is far above any
// approval count this graph uses, and dedupSpan says where it is assumed to be.
func (c *ConfirmTracker) countApprovals(vertex *Node) {
	var seen map[uuid.UUID]struct{}
	if len(vertex.targets) > dedupSpan {
		seen = make(map[uuid.UUID]struct{}, len(vertex.targets))
	}
	for i, t := range vertex.targets {
		if t == nil {
			continue
		}
		if seen != nil {
			if _, dup := seen[t.id.id]; dup {
				continue
			}
			seen[t.id.id] = struct{}{}
		} else if duplicateTarget(vertex.targets[:i], t.id.id) {
			continue
		}
		ttr, ok := c.sites[t.id.id]
		if !ok {
			continue
		}
		ttr.approvers++
		if ttr.approvers >= 1 {
			// One approval is enough: a tip is a site nothing approves.
			//
			// Deliberately not "== 1", even though only the first approval can
			// take a slot away in the ordinary case: a site that was detached
			// when it was first approved is given a slot again when it
			// relinks, and this is what takes it back off. Narrowing the
			// condition would leave that site in the denominator.
			c.dropTip(t.id.id)
			c.retireTip(t.id.id, ttr)
		}
	}
}

// dedupSpan - up to this many approval targets are de-duplicated by scanning
// rather than by building a set.
const dedupSpan = 8

func duplicateTarget(earlier []*Node, id uuid.UUID) bool {
	for _, e := range earlier {
		if e != nil && e.id.id == id {
			return true
		}
	}
	return false
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
// The stack is a field rather than a local: this runs once per inserted site,
// and a fresh slice per call was one of the allocations that put the insert path
// ahead of signature verification in a profile. It is safe to reuse because the
// caller holds the lock and nothing reachable from here marks again.
func (c *ConfirmTracker) markAncestors(from []*Node, slot int) {
	if slot < 0 {
		return
	}
	stack := append(c.markStack[:0], from...)
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
	// Written back because append may have moved it, and the point of the
	// field is to keep whatever capacity the deepest closure needed. Not a
	// deferred assignment: a closure over the local would make it escape, which
	// is the allocation this is removing.
	c.markStack = stack[:0]
}

// unmarkAncestors - the mirror of markAncestors, for a tip that has stopped
// counting: take its bit back off its ancestors.
func (c *ConfirmTracker) unmarkAncestors(from []*Node, slot int) {
	if slot < 0 {
		return
	}
	stack := append(c.unmarkStack[:0], from...)
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
	c.unmarkStack = stack[:0]
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

// ----------------------------------------------------------------- harvested

// harvestPruneFloor - how many harvested ids are kept before the first prune is
// worth attempting. Small enough that a busy node prunes within a second or
// two, large enough that a node whose commit transactions settle a handful of
// sites never scans.
const harvestPruneFloor = 1024

// isHarvested - has this site already been written into a commit transaction.
// Caller holds the lock.
//
// Two records, consulted in order of cost. The map is what this tracker has
// harvested and not yet seen settled; the archive is the ledger's record of
// every site a commit transaction has made irrevocable, and it is only
// consulted once something has actually been pruned out of the map.
func (c *ConfirmTracker) isHarvested(id uuid.UUID) bool {
	if _, done := c.harvested[id]; done {
		return true
	}
	if !c.harvestPruned {
		// Nothing has been dropped from the map, so the map is the whole
		// answer and no node pays for the second lookup until it has to.
		return false
	}
	return settledInArchive(id)
}

// pruneHarvested - drop the ids the slice archive has taken responsibility for.
// Caller holds the lock.
//
// The map exists so that a settled site cannot re-enter the tip set and be
// confirmed - and paid - a second time, and it used to keep one entry per
// settled site for the life of the process: at a few thousand sites a second it
// is the largest thing in the tracker and it only grows.
//
// It is not bounded by forgetting. Forgetting an id resurrects the site, and a
// resurrected site pays its processor twice, so no size cap or generation
// rotation is acceptable here however generous. It is bounded by moving the
// guarantee to the component that already has to hold it: every id in this map
// is on its way into the slice archive, and the archive must keep a settled
// site findable for the life of the ledger because a late arrival can still
// name one as an approval target. An id the archive holds is therefore not
// forgotten - it is remembered once instead of twice, and isHarvested reads
// both.
//
// What is left is the window the archive cannot cover: between a site being
// handed to a commit transaction and that transaction being applied. That is
// bounded by the commit interval rather than by the ledger.
//
// The scan is amortised against the map's own growth - the next one waits until
// it has doubled - because markHarvested is called once per settled site and a
// scan per call would be the O(N^2) this is trying to avoid. With no archive
// configured nothing is pruned and the map behaves exactly as it did.
func (c *ConfirmTracker) pruneHarvested() {
	if len(c.harvested) < c.harvestPruneAt || sliceArchive == nil {
		return
	}
	for id := range c.harvested {
		if settledInArchive(id) {
			delete(c.harvested, id)
			c.harvestPruned = true
		}
	}
	c.harvestPruneAt = 2*len(c.harvested) + harvestPruneFloor
}

// settledInArchive - whether the ledger has settled this site. Mirrors
// settledSite, which resolves the site itself; this only asks the question, so
// it does not build a node to throw away.
func settledInArchive(id uuid.UUID) bool {
	if sliceArchive == nil {
		return false
	}
	return sliceArchive.Has(id)
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
	// Trim the bucket high-water mark before anything else. It only ever rises
	// as sites gain coverage, so this is where it comes back down, and the
	// total it can fall is bounded by the total it has risen - amortised
	// nothing, and it is what keeps the scan below proportional to the buckets
	// that hold something rather than to the tip count.
	for c.maxCount > 0 && len(c.byCount[c.maxCount]) == 0 {
		c.maxCount--
	}
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
		top := c.maxCount
		if top > c.tipCount {
			// A count above the tip count cannot be confirmed: the share is
			// measured against the live tips, so this is the same ceiling the
			// scan has always had.
			top = c.tipCount
		}
		if top < need {
			return
		}
		promoted := c.promoted[:0]
		for count := need; count <= top; count++ {
			for id := range c.byCount[count] {
				promoted = append(promoted, id)
			}
		}
		// Keep whatever capacity it grew to; the next call reuses it from the
		// front. Assigned before the walk below rather than after because the
		// walk can return early.
		c.promoted = promoted
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
			// untrack rather than an unbucket and a delete, so the tip ring
			// cannot be left holding a site the region no longer has. Nothing
			// reaching here is in the ring: a ring member is approved by
			// nothing, so nothing has a path to it and its coverage is zero,
			// which is short of any threshold. That is an invariant rather than
			// an accident, so it is asserted -
			// see assertTipIndexIsConsistent - and the removal stays because a
			// removal path with a hole in it is the kind of thing that is
			// discovered by a site being approved after it was settled.
			c.untrack(id, tr)
			if c.isHarvested(id) {
				continue
			}
			if _, queued := c.confirmedSet[id]; queued {
				continue
			}
			c.confirmedSet[id] = len(c.confirmed)
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
	if c.isHarvested(id) {
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
	if !tr.detached && tr.approvers == 0 {
		c.addTip(id, tr)
	}

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
	if tr.approvers == 0 {
		// Selectable now that it is part of the graph. If something already
		// approved it while it was detached it is not a tip, and countApprovals
		// below will not put it back either.
		c.addTip(vertex.id.id, tr)
	}
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
		// Cleared whatever happens: the queue is about to be emptied, so an
		// index left behind here would point into a slot that no longer holds
		// the site it names.
		delete(c.confirmedSet, id)
		if c.isHarvested(id) {
			continue
		}
		c.harvested[id] = struct{}{}
		out = append(out, n)
	}
	c.confirmed = c.confirmed[:0]
	c.confirmedHoles = 0
	c.pruneHarvested()
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
	out := make([]*Node, 0, len(c.confirmed)-c.confirmedHoles)
	for _, n := range c.confirmed {
		if n == nil {
			continue
		}
		if c.isHarvested(n.id.id) {
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
			// A site settled out of band. Compacting it away here is what keeps
			// the holes markHarvested leaves from accumulating.
			continue
		}
		id := n.id.id
		if _, ok := want[id]; !ok {
			// Re-indexed as it moves: the queue is being compacted in place, so
			// the slot a site sat in is not the slot it keeps.
			c.confirmedSet[id] = len(kept)
			kept = append(kept, n)
			continue
		}
		delete(c.confirmedSet, id)
		if c.isHarvested(id) {
			continue
		}
		c.harvested[id] = struct{}{}
		out = append(out, n)
	}
	for i := len(kept); i < len(c.confirmed); i++ {
		c.confirmed[i] = nil // let the settled sites go
	}
	c.confirmed = kept
	c.confirmedHoles = 0
	c.pruneHarvested()
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

// getTips - every site a new one may approve.
//
// A copy, because the caller reads it after the lock is released and the ring
// itself is mutated by every insert. Selection does not use this: it wants one
// tip at a time and takes it through randomTip, tipExcept and sampleTips, none
// of which copies. What is left here is the whole-set view - Dag.GetTips, the
// legacy fallback path in selection, and the tests - so the cost is now
// proportional to the tip set rather than to the active region, and it is off
// the per-insert path entirely.
func (c *ConfirmTracker) getTips() []*Node {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.expireStaleTips(time.Now())
	tips := make([]*Node, 0, len(c.tipRing))
	for _, tr := range c.tipRing {
		if tr == nil || tr.node == nil {
			continue
		}
		tips = append(tips, tr.node)
	}
	return tips
}

func (c *ConfirmTracker) tip() []*Node { return c.getTips() }

// randomTip - one tip, uniformly. What tip selection actually asks for, several
// times per inserted site, and the reason the tip set is kept as an array.
func (c *ConfirmTracker) randomTip() *Node {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.expireStaleTips(time.Now())
	if len(c.tipRing) == 0 {
		return nil
	}
	tr := c.tipRing[dagRand.Intn(len(c.tipRing))]
	if tr == nil {
		return nil
	}
	return tr.node
}

// tipExcept - a tip that is not already taken. Starts at a random offset and
// scans forward, which is what the slice-copying version did.
func (c *ConfirmTracker) tipExcept(taken map[uuid.UUID]struct{}) *Node {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.expireStaleTips(time.Now())
	n := len(c.tipRing)
	if n == 0 {
		return nil
	}
	start := dagRand.Intn(n)
	for i := 0; i < n; i++ {
		tr := c.tipRing[(start+i)%n]
		if tr == nil || tr.node == nil {
			continue
		}
		if _, dup := taken[tr.node.id.id]; dup {
			continue
		}
		return tr.node
	}
	return nil
}

// sampleTips - up to want distinct tips, uniformly, appended to out.
//
// A partial Fisher-Yates over the ring itself rather than over a copy of it:
// distinct picks without rejection sampling, which matters when the tip set is
// barely larger than want, and no copy of the tip set to make it with. Shuffling
// the ring in place is harmless because its order carries no meaning - every
// reader picks at random or starts at a random offset - but the index has to
// move with the entries, which is what makes this a method here rather than a
// loop in the caller.
func (c *ConfirmTracker) sampleTips(want int, out []*Node) []*Node {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.expireStaleTips(time.Now())
	if want > len(c.tipRing) {
		want = len(c.tipRing)
	}
	for i := 0; i < want; i++ {
		j := i + dagRand.Intn(len(c.tipRing)-i)
		if i != j {
			c.tipRing[i], c.tipRing[j] = c.tipRing[j], c.tipRing[i]
			c.tipAt[c.tipRing[i].node.id.id] = i
			c.tipAt[c.tipRing[j].node.id.id] = j
		}
		out = append(out, c.tipRing[i].node)
	}
	return out
}

func (c *ConfirmTracker) markHarvested(id uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.harvested[id] = struct{}{}
	if tr, ok := c.sites[id]; ok {
		c.retireTip(id, tr)
		c.untrack(id, tr)
	}
	// Nilled in place rather than closed over. A commit transaction calls this
	// once per site it settled, and closing the gap moved the rest of the queue
	// each time - O(queue) per site, so O(queue^2) over one commit. Every reader
	// of the queue already skips a nil, and pop and take compact it.
	if i, queued := c.confirmedSet[id]; queued {
		if i >= 0 && i < len(c.confirmed) && c.confirmed[i] != nil {
			c.confirmed[i] = nil
			c.confirmedHoles++
		}
		delete(c.confirmedSet, id)
	}
	c.pruneHarvested()
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
	// pending discounts the holes a site settled out of band leaves in the
	// queue, so the gauge still reports sites rather than slots.
	return len(c.sites), c.tipCount, len(c.confirmed) - c.confirmedHoles
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
	return c.walkFromInto(from, nil, nil)
}

// walkFromInto - walkFrom, appending the candidates and their potentials to
// buffers the caller owns.
//
// A walk takes several steps and there are several walks per inserted site, so
// the two slices walkFrom returns were four allocations per step counting the
// weights the caller then derives from them. The buffers belong to the walk
// rather than to the tracker: the caller reads them after the lock is released,
// so a tracker-owned buffer would be handed to one walk while another was still
// reading it.
func (c *ConfirmTracker) walkFromInto(from *Node, next []*Node, pot []int) (bool, int, []*Node, []int) {
	if from == nil {
		return false, 0, next, pot
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	tr, ok := c.sites[from.id.id]
	if !ok {
		// Confirmed, pinned, or never tracked: outside the region the walk runs
		// over, so it is neither selectable nor a place to step from.
		return false, 0, next, pot
	}
	selectable := !tr.detached && tr.approvers == 0
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

// stepBack - one uniformly chosen in-region site that this one approves, which
// is all throwing a walk particle ever needed from walkBack. Returns nil at the
// region floor.
//
// Two passes over at most approvetx targets rather than a slice of them: the
// throw takes walkdepth steps per walk and every one of them allocated. The
// random draw is made once, on the count, so the sequence of draws is the same
// as picking an index into the slice - a different sequence would change which
// sites get approved without changing anything about the rule.
func (c *ConfirmTracker) stepBack(from *Node) *Node {
	if from == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	inRegion := 0
	for _, t := range from.targets {
		if t == nil {
			continue
		}
		if tr, inside := c.sites[t.id.id]; inside && tr.node != nil {
			inRegion++
		}
	}
	if inRegion == 0 {
		return nil
	}
	wanted := 0
	for _, t := range from.targets {
		if t == nil {
			continue
		}
		tr, inside := c.sites[t.id.id]
		if !inside || tr.node == nil {
			continue
		}
		if wanted == 0 {
			return tr.node
		}
		wanted--
	}
	return nil
}
