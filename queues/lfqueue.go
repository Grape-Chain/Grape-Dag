/**
 * Lock free implementation of unbounded FIFO queue
 * Based on this paper: https://www.cs.rochester.edu/u/scott/papers/1996_PODC_queues.pdf
 */
package txqueue

import (
	"os"
	"strconv"
	"sync/atomic"
	"time"
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

// DefaultSyncCapacity - how many items a synchronised queue holds before an
// Enqueue has to wait for a consumer.
//
// This used to be an unnamed 1<<15 inside the constructor, and it was the
// node's real inbound ceiling: at 32768 queued items the token channel fills
// and Enqueue blocks. The node logged "DAG Insert queue size is critically
// high: 32767" and then stopped, which read as a mysterious stall rather than
// as a queue reaching a limit somebody chose.
//
// The number is unchanged on purpose. Renaming the ceiling and moving it in the
// same step would make the before-and-after throughput figures unreadable, and
// 32768 is what has been running. It is now named, reported through
// Capacity(), and overridable per process by GRAPE_QUEUE_CAPACITY.
const DefaultSyncCapacity = 1 << 15

// syncCapacity - the capacity NewLockFreeQueue gives a synchronised queue.
//
// Resolved once at package initialisation rather than per queue, because the
// queues are created during start-up and an operator changing the environment
// mid-run should not get two queues with different ceilings.
var syncCapacity = resolveSyncCapacity(os.Getenv("GRAPE_QUEUE_CAPACITY"))

// resolveSyncCapacity - read the operator's ceiling, or fall back to the
// default. A value that does not parse, or is not positive, is ignored rather
// than fatal: a queue of capacity zero would deadlock the node on its first
// transaction, and a typo in an environment variable is not worth that.
func resolveSyncCapacity(env string) int {
	if env == "" {
		return DefaultSyncCapacity
	}
	n, err := strconv.Atoi(env)
	if err != nil || n <= 0 {
		return DefaultSyncCapacity
	}
	return n
}

// Lock free queue structure
// Note: (we rely on unsafe pointers for atomic operation on holders)
//
// Generic in the element type so that a queue of a concrete type stores its
// items inline in the holder. With interface{} elements every Enqueue of a
// struct costs two allocations - one to box the value into the interface and
// one for the holder - and the boxing is invisible at the call site.
// LockFreeQueue below is an alias for the interface{} instantiation, so
// producers that genuinely queue heterogeneous values are unaffected.
type LockFreeQueueOf[T any] struct {
	_head_  unsafe.Pointer //queue head unsafe pointer
	_tail_  unsafe.Pointer //queue tail unsafe pointer
	_sz_    atomic.Int64   //queue current size
	_ch_    chan bool
	_synch_ bool
	_cap_   int64
	// _blocked_ / _waited_ - how often, and for how long in nanoseconds, an
	// Enqueue had to wait for room. This is the only evidence that separates
	// "the node absorbs this rate" from "the queue is absorbing the difference
	// and the producer is about to be held": a queue that is filling shows no
	// symptom until it is full. Counted with atomics and only on the slow path,
	// so the uncontended enqueue pays nothing for them.
	_blocked_ atomic.Uint64
	_waited_  atomic.Int64
}

// LockFreeQueue - the interface{} queue, kept as the package's original name so
// that producers outside this package (services, stats) are untouched.
type LockFreeQueue = LockFreeQueueOf[any]

// value holder and next holder link
type holder[T any] struct {
	_next_  unsafe.Pointer
	_value_ T
}

// NewLockFreeQeueue - create a new instance of LockFreeQueue
func NewLockFreeQueue(synchr bool) *LockFreeQueue {
	return NewLockFreeQueueOf[any](synchr)
}

// NewLockFreeQueueOf - create a queue of a concrete element type.
func NewLockFreeQueueOf[T any](synchr bool) *LockFreeQueueOf[T] {
	return NewLockFreeQueueCapOf[T](synchr, syncCapacity)
}

// NewLockFreeQueueCapOf - create a queue with an explicit ceiling. Capacity is
// meaningless for an unsynchronised queue, which never waits, and is recorded
// as zero there so that a reported capacity of zero means "no ceiling" rather
// than "a ceiling nobody set".
func NewLockFreeQueueCapOf[T any](synchr bool, capacity int) *LockFreeQueueOf[T] {
	h := unsafe.Pointer(&holder[T]{
		_next_: nil,
	})
	q := &LockFreeQueueOf[T]{_head_: h, _tail_: h, _synch_: synchr}
	if synchr {
		if capacity <= 0 {
			capacity = DefaultSyncCapacity
		}
		// make(chan bool, 1<<48-96) - max
		q._ch_ = make(chan bool, capacity)
		q._cap_ = int64(capacity)
	}
	return q
}

// Enqueue - lock free enqueue a new element
func (q *LockFreeQueueOf[T]) Enqueue(v T) {
	// new elements gets wrapped in a holder
	h := &holder[T]{_value_: v}
	for {
		// atomically get the current tail
		tail := __LOAD__[T](&q._tail_)
		// does the current tail holder point to anything?
		next := __LOAD__[T](&tail._next_)
		// check if tail and queue tail are consistent - they should!
		if tail == __LOAD__[T](&q._tail_) {
			// past the last holder - should be nil, otherwise the queue is broken
			if next == nil {
				// atomically compare and swap if needed: next if nil should now point to the new holder
				if __CAS__[T](&tail._next_, next, h) {
					// queue tail needs to be atomically swapped from old tail to new h (holder)
					__CAS__[T](&q._tail_, tail, h)
					q._sz_.Add(1)
					if q._synch_ {
						q.signal()
					}
					return
				}
				// if tmp tail swap fails, unfortunately, we have to try again
				continue
			}
			// tails' next is not nil? this is unexpected, but let's handle this case anyway
			// and repeat the effort in our FOR loop
			__CAS__[T](&q._tail_, tail, next)
		}
	}
}

// signal - hand a wake token to a waiting consumer.
//
// Tried without blocking first so that the common case is one channel send and
// nothing else, and so that reaching the ceiling is recorded. The blocking send
// that follows is the node's backpressure: the item is already linked into the
// queue by this point, so nothing is lost, but the producer is held until a
// consumer catches up. That is deliberate - the alternative, dropping, would
// lose a transaction that a peer has already accepted.
func (q *LockFreeQueueOf[T]) signal() {
	select {
	case q._ch_ <- true:
		return
	default:
	}
	q._blocked_.Add(1)
	start := time.Now()
	q._ch_ <- true
	q._waited_.Add(int64(time.Since(start)))
}

func (q *LockFreeQueueOf[T]) Len() int64 {
	return q._sz_.Load()
}

// Capacity - how many items this queue holds before Enqueue waits. Zero for an
// unsynchronised queue, which has no ceiling.
func (q *LockFreeQueueOf[T]) Capacity() int64 {
	return q._cap_
}

// EnqueueBlocked - enqueues that found the queue at its ceiling and had to wait
// for a consumer. Non-zero means the producer is being held, which is the point
// at which an accepted-transaction rate measured at ingress stops being the
// node's throughput and becomes the queue's drain rate.
func (q *LockFreeQueueOf[T]) EnqueueBlocked() uint64 {
	return q._blocked_.Load()
}

// EnqueueWaitSeconds - total time producers have spent waiting for room.
func (q *LockFreeQueueOf[T]) EnqueueWaitSeconds() float64 {
	return time.Duration(q._waited_.Load()).Seconds()
}

// Dequeue - remove and return an element at the head pointer to the holder
// return:
//
//	the element, the queue's remaining size. The zero value of T is returned
//	when the queue is empty, which for the interface{} instantiation is nil and
//	is what callers have always tested for. Use TryDequeue when the element
//	type has no usable "absent" value.
func (q *LockFreeQueueOf[T]) Dequeue() (T, int64) {
	v, sz, _ := q.TryDequeue()
	return v, sz
}

// TryDequeue - as Dequeue, with an explicit flag saying whether anything came
// out. A synchronised queue blocks until there is something to return, so false
// there means a spurious wakeup rather than an idle queue.
func (q *LockFreeQueueOf[T]) TryDequeue() (T, int64, bool) {
	var zero T
	if q._synch_ {
		// Exactly one token per successful dequeue, taken before the retry
		// loop rather than inside it. Taking it inside meant a lost CAS race
		// consumed a second token, so the token count drifted below the item
		// count and consumers eventually waited on items that were already
		// there - visible as a queue that stalled with work in it. Safe to take
		// it here because Enqueue links the holder before it sends the token:
		// having received the nth token, at least n items have been linked and
		// at most n-1 taken, so one is available.
		<-q._ch_
	}
	for {
		// head cotains the current element
		head := __LOAD__[T](&q._head_)
		tail := __LOAD__[T](&q._tail_)
		next := __LOAD__[T](&head._next_) // candidate for new head
		// still consistent as desired?
		if head == __LOAD__[T](&q._head_) {
			// is the queue empty?
			if head == tail {
				if next == nil {
					q.unsignal()
					return zero, 0, false // certainly empty
				}
				// head and tail are the same but next is not nil, hence tail is lagging
				// adjust the queue, so that next becomes the new tail
				__CAS__[T](&q._tail_, tail, next)
			} else {
				// the queue is certainly not empty
				v := next._value_ // get gueue current value and be ready to return it
				// atomically swap the current head with next as the new head
				if __CAS__[T](&q._head_, head, next) {
					// The value is deliberately left in the holder that has
					// just become the sentinel head. Clearing it would keep
					// one fewer item alive, but it is a plain write to a field
					// that consumers which lost the CAS race have already read
					// - a data race for the sake of one retained item.
					// return the current value
					return v, q._sz_.Add(-1), true
				}
			}
		}
	}
}

// unsignal - hand back a token taken for an item that turned out not to be
// there. Unreachable given the argument in TryDequeue, and here anyway because
// the failure it guards against is silent: a token lost is a consumer that will
// one day wait forever with work in the queue. Non-blocking, since producers
// may have refilled the channel and blocking here would be a deadlock in the
// code path that exists to prevent one.
func (q *LockFreeQueueOf[T]) unsignal() {
	if !q._synch_ {
		return
	}
	select {
	case q._ch_ <- true:
	default:
	}
}

// LOAD - load unsafe pointer and return holder
func __LOAD__[T any](p *unsafe.Pointer) (n *holder[T]) {
	return (*holder[T])(atomic.LoadPointer(p))
}

// CAS - compare and swap atomic pointer swap
func __CAS__[T any](p *unsafe.Pointer, old, new *holder[T]) (ok bool) {
	return atomic.CompareAndSwapPointer(
		p, unsafe.Pointer(old), unsafe.Pointer(new))
}
