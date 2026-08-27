package dag

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"github.com/google/uuid"
)

/*
Benchmarks for the two places where a design decision has a cost attached.

Tip selection walks the unconfirmed region, so its cost depends on the shape of
the graph rather than on the transaction - which means it can degrade quietly as
throughput rises and the frontier grows. Committing a commit transaction writes
to disk with an fsync and replays the pin's payments into the settled balances,
both synchronously, and both were added on the strength of "it still works".
These say what they cost, per part, at frontier and pin sizes matching the
thousand-transactions-a-second target: at a five second commit interval that is
about five thousand sites per commit.

Run: go test ./dag/ -run XXX -bench . -benchtime 200x
*/

// growFrontier - a graph grown by real tip selection, so the shape the walk sees
// is the shape the walk produces rather than one chosen to be convenient.
func growFrontier(b testing.TB, sites int) (*Dag, *ConfirmTracker) {
	b.Helper()
	tr := newConfirmTracker(2, 0)
	prev := confirmationCounter
	confirmationCounter = tr
	b.Cleanup(func() { confirmationCounter = prev })

	genesis := tnode(0)
	d := &Dag{genesis: genesis}
	tr.add(genesis)
	for i := 1; i <= sites; i++ {
		n := tnode(i)
		for _, target := range d.selectTips(0.5) {
			tlink(n, target)
		}
		tr.add(n)
		// Left undrained on purpose: confirmed sites leaving the region is what
		// keeps it small, so draining here would benchmark the easy case. The
		// reported frontier size below says which case this actually is.
	}
	return d, tr
}

func BenchmarkTipSelection(b *testing.B) {
	for _, sites := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("sites=%d", sites), func(b *testing.B) {
			d, tr := growFrontier(b, sites)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if got := d.selectTips(0.5); len(got) == 0 {
					b.Fatal("selection offered nothing to approve")
				}
			}
			b.StopTimer()
			// ResetTimer discards user metrics, so these are reported at the
			// end. They are the reason the timings do not grow with the graph:
			// what selection walks is the unconfirmed frontier, not the ledger.
			active, tips, _ := tr.stats()
			b.ReportMetric(float64(active), "frontier")
			b.ReportMetric(float64(tips), "tips")
			b.ReportMetric(float64(tr.rootCount()), "roots")
		})
	}
}

// BenchmarkGraphGrowth - selection and confirmation together, which is the pair
// that runs once per transaction.
func BenchmarkGraphGrowth(b *testing.B) {
	for _, sites := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("sites=%d", sites), func(b *testing.B) {
			d, tr := growFrontier(b, sites)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				n := tnode(sites + i + 1)
				for _, target := range d.selectTips(0.5) {
					tlink(n, target)
				}
				tr.add(n)
				tr.pop()
			}
		})
	}
}

func BenchmarkConfirmTrackerAdd(b *testing.B) {
	tr := newConfirmTracker(2, 0)
	genesis := tnode(0)
	tr.add(genesis)
	nodes := []*Node{genesis}
	b.ResetTimer()
	for i := 1; i <= b.N; i++ {
		n := tnode(i)
		tips := tipsOf(nodes, 2)
		tlink(n, tips[len(tips)-1])
		if len(tips) > 1 {
			tlink(n, tips[len(tips)-2])
		}
		nodes = append(nodes, n)
		tr.add(n)
	}
}

// benchPin - a commit transaction of the given size, with every payment moving
// value between funded accounts so the settled replay does real arithmetic.
func benchPin(number int64, sites int) *pb.TxPin {
	pin := storedPin(number, []byte{byte(number)}, nil)
	for i := 0; i < sites; i++ {
		s := paymentSite(int(number)*1000000+i, byte(i%64), byte((i+1)%64), 1)
		pin.Sites = append(pin.Sites, &pb.SiteID{Id: s.id.id[:]})
		pin.Nodes = append(pin.Nodes, s.ToPbNode())
	}
	return pin
}

// benchOpeningPin - the chain's first commit transaction, which states the balances
// outright. Funds every account the later pins move value between.
func benchOpeningPin(accounts int) *pb.TxPin {
	balances := map[string]string{}
	for i := 0; i < accounts; i++ {
		balances[addrStr(byte(i%256))] = "1000000000000"
	}
	return storedPin(0, nil, balances)
}

// openingPinWide - an opening statement with an arbitrary number of distinct
// accounts, for measuring work that scales with the account set rather than
// with the transaction rate.
func openingPinWide(accounts int) *pb.TxPin {
	balances := make(map[string]string, accounts)
	for i := 0; i < accounts; i++ {
		balances[fmt.Sprintf("0x%040x", i)] = "1000000000000"
	}
	return storedPin(0, nil, balances)
}

// BenchmarkPinCommit - the whole synchronous commit path: the fsynced store
// append, the settled replay, the periodic balance snapshot, and the slice.
func BenchmarkPinCommit(b *testing.B) {
	for _, sites := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("sites=%d", sites), func(b *testing.B) {
			recoveryFixture(b, filepath.Join(b.TempDir(), "ledger"))
			_dag_ = &Dag{
				mapped_vertices: map[uuid.UUID]*Node{},
				mapped_edges:    map[uuid.UUID][]uuid.UUID{},
			}
			b.Cleanup(func() { _dag_ = nil })
			chainStartCommitted(benchOpeningPin(64))
			pin := benchPin(1, sites)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				pin.PinNumber = int64(i + 1)
				pinCommitted(pin)
			}
		})
	}
}

// BenchmarkPinStoreAppend - the durability boundary on its own. This is the
// fsync, and it is the number that decides whether commits have to be batched.
func BenchmarkPinStoreAppend(b *testing.B) {
	for _, sites := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("sites=%d", sites), func(b *testing.B) {
			s := recoveryFixture(b, filepath.Join(b.TempDir(), "ledger"))
			// One pin, renumbered per iteration. Building b.N pins of 5000 sites
			// up front holds half a million nodes live, and collecting them
			// inside the timed loop is what made an earlier version of this
			// benchmark look superlinear in the number of sites.
			pin := benchPin(1, sites)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				pin.PinNumber = int64(i + 1)
				if err := s.AppendPin(pin, 2); err != nil {
					b.Fatalf("appending: %s", err.Error())
				}
			}
		})
	}
}

// BenchmarkSettledApplyPin - replaying a commit transaction's payments into the
// settled balances, which every node does on every commit regardless of role.
func BenchmarkSettledApplyPin(b *testing.B) {
	for _, sites := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("sites=%d", sites), func(b *testing.B) {
			recoveryFixture(b, filepath.Join(b.TempDir(), "ledger"))
			settled.applyPin(benchOpeningPin(64))
			// See BenchmarkPinStoreAppend: one pin, renumbered, so the timed
			// loop is not competing with the collector over the fixture.
			pin := benchPin(1, sites)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				pin.PinNumber = int64(i + 1)
				settled.applyPin(pin)
			}
		})
	}
}

// BenchmarkBalanceSnapshot - written every snapshotEveryPins commits and
// proportional to the number of accounts, so it is a periodic spike whose size
// grows with adoption rather than with load.
func BenchmarkBalanceSnapshot(b *testing.B) {
	for _, accounts := range []int{100, 10000, 100000} {
		b.Run(fmt.Sprintf("accounts=%d", accounts), func(b *testing.B) {
			recoveryFixture(b, filepath.Join(b.TempDir(), "ledger"))
			settled.applyPin(openingPinWide(accounts))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				snapshotSettledBalances()
			}
		})
	}
}
