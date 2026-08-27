package dag

import (
	"strings"
	"sync"
	"time"

	"github.com/Grape-Chain/Grape-Dag/config"
	"github.com/Grape-Chain/Grape-Dag/utils"
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

// relink - the legacy rule has nothing to reconsider when a site's targets are
// resolved: it counts direct approvals as they arrive.
func (c *ConfirmationCounter) relink(vertex *Node) {}

// newConfirmations - build the confirmation rule named by dag.confirmation.
// "share100" (default) is the technical paper's section 5.1 share-of-tips rule;
// "legacy" is the original fixed two-approver count.
func newConfirmations(cfg config.DagConfiguration) confirmations {
	switch strings.ToLower(strings.TrimSpace(cfg.Confirmation)) {
	case "legacy", "approvers":
		logger.Warn("[confirmation] Using the legacy two-approver rule; a site can be confirmed without the rest of the DAG having seen it")
		return newConfirmationCounter()
	default:
		timeout := time.Duration(cfg.Tiptimeout) * time.Second
		share := cfg.Confirmshare
		if share == 0 || share > 1000 {
			share = DAG_CONFIRM_SHARE
		}
		utils.ColorizeInfo(logger, "[confirmation] Confirming a site once %.1f%% of live tips confirm it (tiptimeout=%s)",
			float64(share)/10, timeout)
		switch {
		case share >= 1000:
			// Measured, not guessed: the literal rule needs every live tip to
			// cover a site while new tips keep arriving, so it stops confirming
			// once several sites are chosen against a view that does not yet
			// contain each other. See docs/confirmation.md.
			logger.Warnf("[confirmation] dag.confirmshare=1000 is the technical paper's literal 100%%. It confirms 99.7%% of sites at four concurrent publishers and 0%% at sixteen. Unless the literal rule is required, leave it at the default %d.",
				DAG_CONFIRM_SHARE)
		case share != DAG_CONFIRM_SHARE:
			logger.Warnf("[confirmation] dag.confirmshare=%d is neither the default %d nor the paper's literal 1000. Convergence at this setting has not been measured; see docs/confirmation.md for what has.",
				share, DAG_CONFIRM_SHARE)
		default:
			logger.Infof("[confirmation] This is a documented departure from the technical paper's literal 100%% rule, which does not converge under concurrent publishing. See docs/confirmation.md.")
		}
		return newConfirmTracker(timeout, share)
	}
}

// walkFrom - see walkRoots: with no share to measure there is nothing to bias a
// walk with.
func (c *ConfirmationCounter) walkFrom(from *Node) (bool, int, []*Node, []int) {
	if from == nil {
		return false, 0, nil, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.tips[from.id.id]
	return ok, 0, nil, nil
}

// walkBack - the legacy rule keeps no region, so there is no depth to throw a
// particle into. Selection falls back to a uniform pick.
func (c *ConfirmationCounter) walkBack(from *Node) []*Node { return nil }
