package dag

import (
	"sync"
	"testing"
)

// Shutdown used to close the watcher channel while inserts could still be
// sending into it, and a send on a closed channel panics. It is a crash on the
// way out rather than a crash in service, but it is the last thing an operator
// sees and it is indistinguishable from data loss at a glance.
//
// The fix is an ordering, and an ordering is what this checks: the field is
// cleared under dag.mux first, so an insert arriving afterwards finds nil and
// returns, and only then is the channel closed. Both senders - insertMissing and
// publishSite - hold dag.mux, which is what makes taking it here sufficient.
func TestDetachingTheWatcherDuringInsertsDoesNotPanic(t *testing.T) {
	d := lockFixture(t, 4)
	d.txCh = make(chan TxVL, txNotifyBuffer)

	site := lockSite(1)
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				// The lock is what the real senders hold; taking it here is what
				// makes this test exercise the ordering rather than get lucky.
				d.mux.Lock()
				d.notifyDagModified(site, nil)
				d.mux.Unlock()
			}
		}()
	}
	// Drain, so the buffer does not simply fill and turn every send into a drop.
	// The channel is captured into a local first: detachNotify writes the field,
	// and reading it from this goroutine without the lock would be a race in the
	// test itself - which is exactly the bug being fixed in dag_watcher.
	drain := d.txCh
	go func() {
		for range drain { //nolint:revive // draining until closed
		}
	}()

	d.detachNotify()
	close(stop)
	wg.Wait()

	if d.txCh != nil {
		t.Error("the watcher channel is still attached after detachNotify, so a later insert would send into a closed channel")
	}
	// Detaching twice is what a second Terminate, or a Terminate racing a
	// shutdown path, would do. Closing an already-closed channel also panics.
	d.detachNotify()
}

// The graph mirror exists to be drawn, and drawing it writes a new
// ./dag.graph.N.gv file for every commit transaction. With peer.visualize off -
// the default - nothing should reach it at all, which means the channel is never
// created and the notification costs a nil check.
func TestWithVisualiseOffNothingIsSentToTheMirror(t *testing.T) {
	d := lockFixture(t, 4)
	d.txCh = nil

	before := txNotifyDrops.Load()
	d.mux.Lock()
	d.notifyDagModified(lockSite(1), nil)
	d.mux.Unlock()

	if after := txNotifyDrops.Load(); after != before {
		t.Errorf("a notification with the mirror off counted %d drop(s); it should not have been attempted at all", after-before)
	}
}
