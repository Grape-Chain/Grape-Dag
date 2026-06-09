package txqueue

import (
	"sync"

	"github.com/golang-collections/collections/queue"
)

type SyncQueue struct {
	mx sync.Mutex
	q  *queue.Queue
	c  chan bool
}

var syncq *SyncQueue

func GetSyncQueue() *SyncQueue {
	if syncq == nil {
		syncq = &SyncQueue{
			mx: sync.Mutex{},
			q:  queue.New(),
			c:  make(chan bool, 100),
		}
	}
	return syncq
}

func (pq *SyncQueue) Enqueue(value interface{}) {
	pq.c <- true
	pq.mx.Lock()
	defer pq.mx.Unlock()
	pq.q.Enqueue(value)
}

func (pq *SyncQueue) Dequeue() interface{} {
	<-pq.c
	pq.mx.Lock()
	defer pq.mx.Unlock()
	if pq.q.Len() > 0 {
		return pq.q.Dequeue()
	}
	return nil
}
