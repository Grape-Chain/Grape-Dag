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
	tipNodes := []*Node{}
	for k, _ := range c.tips {
		if v, ok := c.cache[k]; ok {
			tipNodes = append(tipNodes, v)
		}
	}
	return tipNodes
}

func newConfirmationCounter() *ConfirmationCounter {
	return &ConfirmationCounter{
		mu:        sync.Mutex{},
		tips:      make(map[uuid.UUID][]uuid.UUID),
		cache:     make(map[uuid.UUID]*Node),
		confirmed: set.New(),
	}
}

func (c *ConfirmationCounter) add(vertex *Node) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[vertex.id.id] = vertex
	c.tips[vertex.id.id] = []uuid.UUID{vertex.id.id}
	for _, v := range vertex.targets {
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
		confirmed = append(confirmed, c.cache[id])
	})
	c.confirmed = set.New()
	return confirmed
}

func (c *ConfirmationCounter) tip() []*Node {
	c.mu.Lock()
	defer c.mu.Unlock()
	tips := []*Node{}
	for k, _ := range c.tips {
		tips = append(tips, c.cache[k])
	}
	return tips
}
