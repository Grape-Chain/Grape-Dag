package dag

import (
	"sync"

	"github.com/golang-collections/collections/set"
	"github.com/google/uuid"
)

type ConfirmationCounter struct {
	mu        sync.Mutex
	tips      map[uuid.UUID][]uuid.UUID
	cache     map[uuid.UUID]*Node
	confirmed *set.Set
	// harvested sites that have already been handed to a pin tx. A site may be
	// referenced again as an approval target long after it was pinned (see
	// InsertTxDag, which resolves targets out of past pins), and without this
	// it would re-enter the tip set and be confirmed - and paid - twice.
	// @Note: this grows with the ledger; it is pruned at the slice boundary
	// once slicing lands.
	harvested map[uuid.UUID]struct{}
}

// isTip - return true/false if a a vertex with id is a tip
// Since we keep cache for other purposes we can utilize this
// cached information for fast lookup
func (c *ConfirmationCounter) isTip(id uuid.UUID) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.tips[id]
	return ok
}

func (c *ConfirmationCounter) getTips() []*Node {
	c.mu.Lock()
	defer c.mu.Unlock()
	tipNodes := []*Node{}
	for k := range c.tips {
		if v, ok := c.cache[k]; ok && v != nil {
			tipNodes = append(tipNodes, v)
		}
	}
	return tipNodes
}

// resolveSite - look a site up in the dag when the counter has lost track of it.
// Kept separate so pop() does not depend on the dag lock being free: getById
// takes only the lookup RWMutex, never dag.mux.
func resolveSite(id uuid.UUID) *Node {
	if _dag_ == nil {
		return nil
	}
	return _dag_.getById(id, true)
}

// markHarvested - record that a site has been written into a pin, so it cannot
// re-enter the tip set and be confirmed (and, once fees land, paid) twice. Used
// for sites that reach a pin without going through pop(), i.e. genesis.
func (c *ConfirmationCounter) markHarvested(id uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.harvested[id] = struct{}{}
	delete(c.tips, id)
	delete(c.cache, id)
}

func newConfirmationCounter() *ConfirmationCounter {
	return &ConfirmationCounter{
		mu:        sync.Mutex{},
		tips:      make(map[uuid.UUID][]uuid.UUID),
		cache:     make(map[uuid.UUID]*Node),
		confirmed: set.New(),
		harvested: make(map[uuid.UUID]struct{}),
	}
}

func (c *ConfirmationCounter) add(vertex *Node) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, done := c.harvested[vertex.id.id]; done {
		// already pinned; do not resurrect it as a tip
		return
	}
	c.cache[vertex.id.id] = vertex
	c.tips[vertex.id.id] = []uuid.UUID{vertex.id.id}
	for _, v := range vertex.targets {
		if _, done := c.harvested[v.id.id]; done {
			// this target has already been pinned - it cannot be confirmed again
			continue
		}
		c.tips[v.id.id] = append(c.tips[v.id.id], vertex.id.id)
		if len(c.tips[v.id.id]) > 2 {
			c.confirmed.Insert(v.id.id)
			delete(c.tips, v.id.id)
		}
	}
}

func (c *ConfirmationCounter) pop() []*Node {
	c.mu.Lock()
	defer c.mu.Unlock()
	confirmed := []*Node{}
	c.confirmed.Do(func(i interface{}) {
		id := i.(uuid.UUID)
		node, ok := c.cache[id]
		if !ok || node == nil {
			// Not every insert path registers a site with the counter (a site
			// relinked by insertMissing, or an approval target resolved out of a
			// past pin, arrives as a fresh *Node), so fall back to the dag before
			// giving up. Returning a nil here would be dereferenced while the pin
			// builder sorts sites by time; dropping it silently would lose a
			// confirmed site instead.
			if resolved := resolveSite(id); resolved != nil {
				node = resolved
			} else {
				logger.Warnf("[confirmation] Confirmed site %s is in neither the counter cache nor the dag, skipping", id.String())
				return
			}
		}
		if _, done := c.harvested[id]; done {
			return
		}
		c.harvested[id] = struct{}{}
		delete(c.cache, id)
		confirmed = append(confirmed, node)
	})
	c.confirmed = set.New()
	return confirmed
}

func (c *ConfirmationCounter) tip() []*Node {
	c.mu.Lock()
	defer c.mu.Unlock()
	tips := []*Node{}
	for k := range c.tips {
		if v, ok := c.cache[k]; ok && v != nil {
			tips = append(tips, v)
		}
	}
	return tips
}
