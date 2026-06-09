package txqueue

import (
	"sync"

	"github.com/golang-collections/collections/queue"
)

type BasicQueue struct {
	mx sync.Mutex
	q  *queue.Queue
}

var basicq *BasicQueue

func GetBasicQueue() *BasicQueue {
	if basicq == nil {
		basicq = &BasicQueue{
			mx: sync.Mutex{},
			q:  queue.New(),
		}
	}
	return basicq
}

func NewBasicQueue() *BasicQueue {
	return &BasicQueue{
		mx: sync.Mutex{},
		q:  queue.New(),
	}
}

func (bq *BasicQueue) Enqueue(value interface{}) {
	bq.mx.Lock()
	defer bq.mx.Unlock()
	bq.q.Enqueue(value)
}

func (bq *BasicQueue) Len() int {
	bq.mx.Lock()
	defer bq.mx.Unlock()
	return bq.q.Len()
}

func (bq *BasicQueue) Dequeue() interface{} {
	bq.mx.Lock()
	defer bq.mx.Unlock()
	if bq.q.Len() > 0 {
		return bq.q.Dequeue()
	}
	return nil
}
