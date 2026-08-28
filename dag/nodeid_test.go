package dag

import (
	"math/big"
	"sync"
	"testing"

	"github.com/Grape-Chain/Grape-Dag/tx"
)

// Two goroutines hand out site ids: the publisher, for a transaction this node
// accepted over the API, and the subscriber, for one a peer announced. NewDagNode
// took the major half of the id by incrementing Dag.prevMajor and then reading it
// back as a separate statement, which is two bugs in three lines - a data race on
// the field, and a window in which two overlapping inserts read the same value
// back and give two different sites the same major id.
//
// It is worth having a test for the duplicate rather than trusting -race to catch
// everything, because the race detector reports the racing access and not the
// consequence, and the consequence is two sites claiming one position in the
// order the ids are supposed to establish.
func TestConcurrentSitesNeverShareAMajorId(t *testing.T) {
	d := lockFixture(t, 4)
	d.prevMajor.Store(0)

	// Far more goroutines than cores, on purpose. The bug this catches needs two
	// of them to interleave between taking a number and reading it back, so the
	// test wants the scheduler preempting inside that window constantly rather
	// than a tidy one-goroutine-per-core run that could step around it.
	const goroutines, each = 64, 200
	ids := make([][]uint64, goroutines)
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		ids[g] = make([]uint64, each)
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < each; i++ {
				n := NewDagNode(newIdTestTx(), false)
				if n == nil {
					t.Errorf("goroutine %d: NewDagNode returned nothing for a well-formed payment", g)
					return
				}
				ids[g][i] = n.id.idMajor
			}
		}(g)
	}
	wg.Wait()

	seen := make(map[uint64]bool, goroutines*each)
	for g := range ids {
		for _, id := range ids[g] {
			if seen[id] {
				t.Fatalf("major id %d was handed to two sites - %d goroutines took ids that overlap", id, goroutines)
			}
			seen[id] = true
		}
	}
	if len(seen) != goroutines*each {
		t.Errorf("%d distinct major ids from %d sites", len(seen), goroutines*each)
	}
	if got := d.prevMajor.Load(); got != uint64(goroutines*each) {
		t.Errorf("prevMajor finished at %d after %d sites - increments were lost", got, goroutines*each)
	}
}

// A site cannot be made before there is a graph to put it in, and asking for one
// used to dereference the nil global rather than say so.
func TestMakingASiteWithNoGraphIsRefused(t *testing.T) {
	prev := _dag_
	_dag_ = nil
	t.Cleanup(func() { _dag_ = prev })

	if n := NewDagNode(newIdTestTx(), false); n != nil {
		t.Errorf("NewDagNode returned a site with no graph in place: %v", n.id.id)
	}
}

func newIdTestTx() tx.Transaction {
	t := tx.NewTxv1(tx.ChainType(peerConfig.Network))
	t.Tx_Type = tx.PAYMENT
	t.Sender = addr(0xaa)
	t.Recepient = addr(0xbb)
	t.Amount = big.NewInt(1).Bytes()
	return t
}
