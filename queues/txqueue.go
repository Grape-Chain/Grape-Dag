package txqueue

import (
	"container/heap"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/Grape-Chain/Grape-Dag/tx"
)

type QueuePriority uint8

const (
	LOWEST_PRIORITY QueuePriority = iota
	LOWER_PRIORITY
	DEFAULT_PRIORITY
	HIGHER_PRIORITY
	HIGHEST_PRIORITY
)

const (
	PUB_QUEUE_CAPACITY = 999999
)

// An Item is something we manage in a priority queue.
type Item struct {
	value    any // The value of the item; arbitrary.
	priority int // The priority of the item in the queue.
	// The index is needed by update and is maintained by the heap.Interface methods.
	index int // The index of the item in the heap.
}

// A PriorityQueue implements heap.Interface and holds Items.
type PriorityQueue []*Item

type PublishQueue struct {
	mx sync.Mutex
	q  PriorityQueue
	c  chan bool
}

var pubq *PublishQueue

var lfq *LockFreeQueue

func GetPublishQueue() *LockFreeQueue {
	if lfq == nil {
		lfq = NewLockFreeQueue(true)
	}
	return lfq
}

func GetPublishQueueEx() *PublishQueue {
	if pubq == nil {
		pubq = &PublishQueue{
			mx: sync.Mutex{},
			q:  make(PriorityQueue, 0, PUB_QUEUE_CAPACITY),
			c:  make(chan bool, PUB_QUEUE_CAPACITY),
		}
		heap.Init(&pubq.q)
	}
	return pubq
}

func (pq *PublishQueue) Enqueue(value any, priority QueuePriority) {
	attempts := 0
	for {
		pq.mx.Lock()
		l := pq.Len()
		pq.mx.Unlock()
		if l < PUB_QUEUE_CAPACITY {
			item := &Item{
				value:    value,
				priority: int(priority),
			}
			pq.mx.Lock()
			heap.Push(&pq.q, item)
			pq.mx.Unlock()
			pq.c <- true
			break
		} else {
			attempts++
			// Offer queue relief
			if attempts%1000 == 0 {
				t := time.NewTimer(time.Second)
				<-t.C
				attempts = 0
			} else {
				runtime.Gosched()
			}
		}
	}
}

func (pq *PublishQueue) Dequeue() any {
	<-pq.c
	pq.mx.Lock()
	defer pq.mx.Unlock()
	// if pq.q.Len() > 0 {
	// item := heap.Pop(&pq.q).(*Item)
	// return item.value
	return heap.Pop(&pq.q).(*Item).value
	// }
	// return nil
}

func (pq *PublishQueue) Len() int { return len(pq.q) }

func (pq *PriorityQueue) Len() int { return len(*pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	// We want Pop to give us the highest, not lowest, priority so we use greater than here.
	return pq[i].priority > pq[j].priority
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *PriorityQueue) Push(x any) {
	n := len(*pq)
	item := x.(*Item)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *PriorityQueue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // avoid memory leak
	item.index = -1 // for safety
	*pq = old[0 : n-1]
	return item
}

// update modifies the priority and value of an Item in the queue.
func (pq *PriorityQueue) update(item *Item, value tx.Txv1, priority int) {
	item.value = value
	item.priority = priority
	heap.Fix(pq, item.index)
}

func test(items []Item) {
	// Create a priority queue, put the items in it, and
	// establish the priority queue (heap) invariants.
	pq := make(PriorityQueue, len(items))
	i := 0
	for _, item := range items {
		pq[i] = &Item{
			value:    item.value,
			priority: item.priority,
			index:    i,
		}
		i++
	}
	heap.Init(&pq)

	// Insert a new item and then modify its priority.
	item := &Item{
		value:    tx.Txv1{},
		priority: 1,
	}
	heap.Push(&pq, item)
	//pq.update(item, item.value, 5)

	// Take the items out; they arrive in decreasing priority order.
	for pq.Len() > 0 {
		item := heap.Pop(&pq).(*Item)
		fmt.Printf("%.2d:%s ", item.priority, item.value)
	}
}
