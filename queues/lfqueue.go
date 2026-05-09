/**
 * Lock free implementation of unbounded FIFO queue
 * Based on this paper: https://www.cs.rochester.edu/u/scott/papers/1996_PODC_queues.pdf
 */
package txqueue

import (
	"sync/atomic"
	"unsafe"
)

/**
Pointer represents a pointer to an arbitrary type.
There are four special operations available for type Pointer that are not available for other types:

    A pointer value of any type can be converted to a Pointer.
    A Pointer can be converted to a pointer value of any type.
    A uintptr can be converted to a Pointer.
    A Pointer can be converted to a uintptr.

Pointer therefore allows a program to defeat the type system and read and write arbitrary memory.
It should be used with extreme care.

The following patterns involving Pointer are valid.
Code not using these patterns is likely to be invalid today or to become invalid in the future.
Even the valid patterns below come with important caveats.
*/

// Lock free queue structure
// Note: (we rely on unsafe pointers for atomic operation on holders)
type LockFreeQueue struct {
	_head_  unsafe.Pointer //queue head unsafe pointer
	_tail_  unsafe.Pointer //queue tail unsafe pointer
	_sz_    atomic.Int64   //queue current size
	_ch_    chan bool
	_synch_ bool
}

// value holder and next holder link
type holder struct {
	_next_  unsafe.Pointer
	_value_ interface{}
}

// NewLockFreeQeueue - create a new instance of LockFreeQueue
func NewLockFreeQueue(synchr bool) *LockFreeQueue {
	h := unsafe.Pointer(&holder{
		_next_:  nil,
		_value_: nil,
	})
	if synchr {
		// make(chan bool, 1<<48-96) - max
		return &LockFreeQueue{_head_: h, _tail_: h, _sz_: atomic.Int64{}, _ch_: make(chan bool, 1<<15), _synch_: true}
	}
	return &LockFreeQueue{_head_: h, _tail_: h, _sz_: atomic.Int64{}, _synch_: false}
}

// Enqueue - lock free enqueue a new element
func (q *LockFreeQueue) Enqueue(v interface{}) {
	// new elements gets wrapped in a holder
	h := &holder{_value_: v}
	for {
		// atomically get the current tail
		tail := __LOAD__(&q._tail_)
		// does the current tail holder point to anything?
		next := __LOAD__(&tail._next_)
		// check if tail and queue tail are consistent - they should!
		if tail == __LOAD__(&q._tail_) {
			// past the last holder - should be nil, otherwise the queue is broken
			if next == nil {
				// atomically compare and swap if needed: next if nil should now point to the new holder
				if __CAS__(&tail._next_, next, h) {
					// queue tail needs to be atomically swapped from old tail to new h (holder)
					__CAS__(&q._tail_, tail, h)
					q._sz_.Add(1)
					if q._synch_ {
						q._ch_ <- true
					}
					return
				}
				// if tmp tail swap fails, unfortunately, we have to try again
				continue
			}
			// tails' next is not nil? this is unexpected, but let's handle this case anyway
			// and repeat the effort in our FOR loop
			__CAS__(&q._tail_, tail, next)
		}
	}
}

func (q *LockFreeQueue) Len() int64 {
	return q._sz_.Load()
}

// Dequeue - remove and return an element at the head pointer to the holder
// return:
//
//	an interface to the element, nil if queue is empty
func (q *LockFreeQueue) Dequeue() (interface{}, int64) {
	for {
		if q._synch_ {
			<-q._ch_
		}
		// head cotains the current element
		head := __LOAD__(&q._head_)
		tail := __LOAD__(&q._tail_)
		next := __LOAD__(&head._next_) // candidate for new head
		// still consistent as desired?
		if head == __LOAD__(&q._head_) {
			// is the queue empty?
			if head == tail {
				if next == nil {
					return nil, 0 // certainly empty
				}
				// head and tail are the same but next is not nil, hence tail is lagging
				// adjust the queue, so that next becomes the new tail
				__CAS__(&q._tail_, tail, next)
			} else {
				// the queue is certainly not empty
				v := next._value_ // get gueue current value and be ready to return it
				// atomically swap the current head with next as the new head
				if __CAS__(&q._head_, head, next) {
					// return the current value
					return v, q._sz_.Add(-1)
				}
			}
		}
	}
}

// LOAD - load unsafe pointer and return holder
func __LOAD__(p *unsafe.Pointer) (n *holder) {
	return (*holder)(atomic.LoadPointer(p))
}

// CAS - compare and swap atomic pointer swap
func __CAS__(p *unsafe.Pointer, old, new *holder) (ok bool) {
	return atomic.CompareAndSwapPointer(
		p, unsafe.Pointer(old), unsafe.Pointer(new))
}
