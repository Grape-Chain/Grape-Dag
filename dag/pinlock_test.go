package dag

import (
	"bytes"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Grape-Chain/Grape-Dag/vm"
)

/*
The pin lock: what may be done without it, and what must not.

A commit transaction is the ledger's only irrevocable statement, and forming one
is expensive - it reads every account the settled sites touch, executes the
contract transactions against the VM, and signs a payload that is megabytes at
load. The lock is held for the append and not for the build, so the tests here
are about the seam that creates: that the build really does run without the lock,
that a build the chain has moved under is thrown away rather than appended, and
that the chain a reader sees is gap-free and correctly linked throughout.
*/

// buildFixture - the globals a build reads, plus a state store to execute
// against and a wallet to sign with.
func buildFixture(t testing.TB) *NodeTxPin {
	t.Helper()
	recoveryFixture(t, t.TempDir())
	vm.AttachInMemoryStateStore()
	prevWallet := dagWallet
	dagWallet = testPinWallet
	t.Cleanup(func() { dagWallet = prevWallet })
	return _pins_
}

// fundedChainStart - an opening commit transaction that states balances for the
// accounts the test's payments move value between, appended as the chain's head.
func fundedChainStart(t testing.TB, p *NodeTxPin, accounts int) {
	t.Helper()
	balances := map[string]string{}
	for i := 0; i < accounts; i++ {
		balances[addrStr(byte(i))] = "1000000000000"
	}
	p.lock("test chain start")
	p.unsafe_appendPin(storedPin(0, nil, balances))
	p.unlock()
}

// paymentsFrom - n distinct settled sites, each a payment between two of the
// funded accounts. Distinct per caller, because building sorts the slice it is
// given in place.
func paymentsFrom(seed, n, accounts int) []*Node {
	sites := make([]*Node, 0, n)
	for i := 0; i < n; i++ {
		sites = append(sites, paymentSite(seed*1000000+i, byte(i%accounts), byte((i+1)%accounts), 1))
	}
	return sites
}

// A commit transaction built against a head that has since moved names a
// predecessor that is no longer the head and a number that is taken. It is not
// appendable at any price: the ledger would carry two statements for the same
// point in the chain.
func TestACommitTransactionBuiltAgainstAMovedHeadIsDiscarded(t *testing.T) {
	p := buildFixture(t)
	fundedChainStart(t, p, 8)

	head := p.headSnapshot()
	built, err := p.buildCheckpointed(head, paymentsFrom(1, 4, 8), nil)
	if err != nil {
		t.Fatalf("building: %s", err.Error())
	}

	// Somebody else's commit transaction lands first - an applied pin from the
	// network reaches the chain through exactly this call.
	p.lock("test competing append")
	p.unsafe_appendPin(storedPin(head.number, head.prev, map[string]string{addrStr(0): "1"}))
	p.unlock()

	if p.appendIfHeadUnmoved(head, built.pin) {
		t.Fatal("a commit transaction built against a head that had moved was appended")
	}
	built.discard()

	if got := p.CurrentHeight(); got != int(head.number) {
		t.Fatalf("chain head is %d, want %d: the discarded build reached the chain", got, head.number)
	}
	chain := p.snapshotPins()
	if len(chain) != 2 {
		t.Fatalf("chain holds %d commit transaction(s), want 2", len(chain))
	}
	if bytes.Equal(chain[1].Sign, built.pin.Sign) {
		t.Fatal("the discarded commit transaction is the chain head")
	}
}

// The other half of the gate: a build against the current head is appended, and
// what it changed stands.
func TestACommitTransactionBuiltAgainstTheCurrentHeadIsAppended(t *testing.T) {
	p := buildFixture(t)
	fundedChainStart(t, p, 8)
	before := p.GetLastPin()

	if err := p.add(paymentsFrom(1, 4, 8), nil); err != nil {
		t.Fatalf("adding: %s", err.Error())
	}

	head := p.GetLastPin()
	if head.PinNumber != before.PinNumber+1 {
		t.Fatalf("appended commit transaction %d after %d", head.PinNumber, before.PinNumber)
	}
	if !bytes.Equal(head.Prev, before.Sign) {
		t.Fatal("the appended commit transaction does not name its predecessor")
	}
	if len(head.Sites) != 4 {
		t.Fatalf("the appended commit transaction settled %d site(s), want 4", len(head.Sites))
	}
	// The balances it states supersede the cache, so the accounts it settled for
	// are no longer cached and the next lookup reads the chain.
	for wallet := range head.Balance.Balance {
		if _, err := walletCache.get(wallet); err == nil {
			t.Fatalf("wallet %s is still cached after the commit transaction that settled it", wallet)
		}
	}
}

// The point of the split: the build must not need the pin lock. Held here for
// the duration of a build, which would deadlock if the build reached for it.
func TestABuildDoesNotWaitForTheChainLock(t *testing.T) {
	p := buildFixture(t)
	fundedChainStart(t, p, 8)
	head := p.headSnapshot()

	p.lock("test holds the chain")
	done := make(chan *builtPin, 1)
	go func() {
		p.LockBuild()
		defer p.UnlockBuild()
		built, err := p.buildCheckpointed(head, paymentsFrom(2, 64, 8), nil)
		if err != nil {
			done <- nil
			return
		}
		done <- built
	}()

	select {
	case built := <-done:
		p.unlock()
		if built == nil {
			t.Fatal("building failed")
		}
		built.discard()
	case <-time.After(10 * time.Second):
		p.unlock()
		t.Fatal("the build waited for the pin lock; it is meant to run with the lock released")
	}
}

// Readers take no lock at all, so they must make progress while a build is in
// flight - including the balance lookups the insert path makes, which is where
// the contention showed up.
func TestChainReadersRunWhileACommitTransactionIsBeingBuilt(t *testing.T) {
	p := buildFixture(t)
	fundedChainStart(t, p, 8)

	stop := make(chan struct{})
	reads := make(chan int, 1)
	go func() {
		n := 0
		for {
			select {
			case <-stop:
				reads <- n
				return
			default:
			}
			p.CurrentHeight()
			if _, err := p.GetBalance(addr(0)); err != nil {
				panic("balance lookup failed: " + err.Error())
			}
			p.snapshotPins()
			n++
		}
	}()

	for i := 0; i < 8; i++ {
		if err := p.add(paymentsFrom(10+i, 32, 8), nil); err != nil {
			t.Fatalf("adding: %s", err.Error())
		}
	}
	close(stop)
	if n := <-reads; n == 0 {
		t.Fatal("no reader made progress while commit transactions were being built")
	}
}

// Whatever the interleaving, the chain must number its commit transactions
// consecutively, hold no duplicate, and link every one to the signature of the
// one before it. Builders and readers together, so the race detector sees the
// published view being read while it is being replaced.
func TestTheChainStaysGapFreeWhileBuildersAndReadersRunTogether(t *testing.T) {
	p := buildFixture(t)
	fundedChainStart(t, p, 16)

	const builders, rounds = 4, 6
	var building, reading sync.WaitGroup
	stop := make(chan struct{})

	for r := 0; r < 3; r++ {
		reading.Add(1)
		go func() {
			defer reading.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				chain := p.snapshotPins()
				for i := 1; i < len(chain); i++ {
					// Read, not asserted on, from a goroutine that must not call
					// t.Fatalf; the assertions are made once at the end.
					_ = chain[i].PinNumber
				}
				p.CurrentHeight()
				p.getAllFrom(0)
			}
		}()
	}

	var succeeded, discarded int64
	for b := 0; b < builders; b++ {
		building.Add(1)
		go func(b int) {
			defer building.Done()
			for r := 0; r < rounds; r++ {
				if err := p.add(paymentsFrom(100+b*100+r, 8, 16), nil); err != nil {
					// A build the head moved under is discarded, which is the
					// gate working; it must not have appended anything.
					atomic.AddInt64(&discarded, 1)
					continue
				}
				atomic.AddInt64(&succeeded, 1)
			}
		}(b)
	}

	// Builders first, then the readers that were watching them.
	building.Wait()
	close(stop)
	reading.Wait()

	chain := p.snapshotPins()
	if len(chain) != int(succeeded)+1 {
		t.Fatalf("chain holds %d commit transaction(s) after %d successful and %d discarded build(s)",
			len(chain), succeeded, discarded)
	}
	seen := map[int64]bool{}
	for i, pin := range chain {
		if seen[pin.PinNumber] {
			t.Fatalf("commit transaction %d appears twice in the chain", pin.PinNumber)
		}
		seen[pin.PinNumber] = true
		if i == 0 {
			continue
		}
		if pin.PinNumber != chain[i-1].PinNumber+1 {
			t.Fatalf("chain jumps from %d to %d", chain[i-1].PinNumber, pin.PinNumber)
		}
		if !bytes.Equal(pin.Prev, chain[i-1].Sign) {
			t.Fatalf("commit transaction %d does not name %d as its predecessor",
				pin.PinNumber, chain[i-1].PinNumber)
		}
	}
}

// ------------------------------------------------------- the retain window

// Past the window a commit transaction keeps everything the chain's readers ask
// of it - its place in the chain, the sites it settled, the balances it stated -
// and lets go of the transactions themselves.
func TestReleasingSettledTransactionsKeepsTheChainReadable(t *testing.T) {
	p := &NodeTxPin{}
	prevRetain := pinBodyRetainPins
	pinBodyRetainPins = 4
	defer func() { pinBodyRetainPins = prevRetain }()

	const pins = 12
	p.lock("test")
	p.unsafe_appendPin(storedPin(0, nil, map[string]string{addrStr(0xaa): "1000"},
		paymentSite(0, 0xaa, 0xbb, 1)))
	for n := int64(1); n < pins; n++ {
		prev := p.unsafe_getLastPin().Sign
		p.unsafe_appendPin(storedPin(n, prev,
			map[string]string{fmt.Sprintf("0x%040x", n): fmt.Sprint(n)},
			paymentSite(int(n), 0xaa, 0xbb, 1)))
	}
	p.unlock()

	chain := p.snapshotPins()
	if len(chain) != pins {
		t.Fatalf("chain holds %d commit transaction(s), want %d", len(chain), pins)
	}
	for i, pin := range chain {
		if pin.PinNumber != int64(i) {
			t.Fatalf("commit transaction at %d is numbered %d", i, pin.PinNumber)
		}
		if i > 0 && !bytes.Equal(pin.Prev, chain[i-1].Sign) {
			t.Fatalf("commit transaction %d lost its link to %d", i, i-1)
		}
		if len(pin.Sites) != 1 {
			t.Fatalf("commit transaction %d lost the sites it settled", i)
		}
		if len(pin.Balance.Balance) == 0 {
			t.Fatalf("commit transaction %d lost the balances it stated", i)
		}
	}
	// Index 0 is kept whole: dag.Init reads the genesis site back out of it.
	if len(chain[0].Nodes) != 1 {
		t.Fatal("the opening commit transaction lost the genesis site")
	}
	// Everything outside the window has let go of its transactions.
	for i := 1; i < pins-pinBodyRetainPins; i++ {
		if len(chain[i].Nodes) != 0 {
			t.Fatalf("commit transaction %d still holds its transactions past the retain window", i)
		}
	}
	for i := pins - pinBodyRetainPins; i < pins; i++ {
		if len(chain[i].Nodes) != 1 {
			t.Fatalf("commit transaction %d inside the retain window has no transactions", i)
		}
	}

	// The balance lookups walk the whole chain, and a wallet last mentioned
	// before the window must still be found.
	if got, err := p.unsafe_getLatestBalance(addrStr(0xaa)); err != nil || got.Int64() != 1000 {
		t.Fatalf("balance for a wallet last stated in the opening pin = %v, %v; want 1000", got, err)
	}
	if got, err := p.unsafe_getLatestBalance(fmt.Sprintf("0x%040x", 2)); err != nil || got.Int64() != 2 {
		t.Fatalf("balance for a wallet stated outside the window = %v, %v; want 2", got, err)
	}
	// And a pin is still addressable by number.
	if pin := p.GetPin(2); pin == nil || pin.PinNumber != 2 {
		t.Fatal("a commit transaction outside the window is no longer addressable by number")
	}
}

// A pin whose transactions have been released cannot be sent to a peer: the
// receiver derives balances from those transactions and checks a signature that
// covers them. Better to serve nothing and say so than to serve a pin that
// settles nothing.
func TestAPinWithoutItsTransactionsIsNotServedToAPeer(t *testing.T) {
	p := &NodeTxPin{}
	prevRetain := pinBodyRetainPins
	pinBodyRetainPins = 3
	defer func() { pinBodyRetainPins = prevRetain }()

	const pins = 10
	p.lock("test")
	p.unsafe_appendPin(storedPin(0, nil, map[string]string{addrStr(0xaa): "1"}))
	for n := int64(1); n < pins; n++ {
		prev := p.unsafe_getLastPin().Sign
		p.unsafe_appendPin(storedPin(n, prev, nil, paymentSite(int(n), 0xaa, 0xbb, 1)))
	}
	p.unlock()

	if got := p.getAllFrom(1); got != nil {
		t.Fatalf("served %d pin(s) from a height whose transactions were released", len(got))
	}
	served := p.getAllFrom(pins - pinBodyRetainPins)
	if len(served) != pinBodyRetainPins {
		t.Fatalf("served %d pin(s) from inside the window, want %d", len(served), pinBodyRetainPins)
	}
	for _, pin := range served {
		if len(pin.Sites) > 0 && len(pin.Nodes) == 0 {
			t.Fatalf("served pin %d without the transactions it settled", pin.PinNumber)
		}
	}
	// Selected by pin number, not by position: a node that joined from a
	// balance snapshot holds pins numbered from wherever the leader was.
	joined := &NodeTxPin{}
	joined.lock("test")
	joined.unsafe_appendPin(storedPin(500, nil, map[string]string{addrStr(0xaa): "1"}))
	joined.unsafe_appendPin(storedPin(501, joined.unsafe_getLastPin().Sign, nil,
		paymentSite(1, 0xaa, 0xbb, 1)))
	joined.unlock()
	if got := joined.getAllFrom(501); len(got) != 1 || got[0].PinNumber != 501 {
		t.Fatalf("a snapshot-synced chain served %d pin(s) from height 501", len(got))
	}
}

// ------------------------------------------------------------- benchmarks

// benchBuildFixture - a chain with an opening statement, ready to build on.
func benchBuildFixture(b *testing.B, accounts int) *NodeTxPin {
	b.Helper()
	p := buildFixture(b)
	balances := map[string]string{}
	for i := 0; i < accounts; i++ {
		balances[addrStr(byte(i))] = "1000000000000"
	}
	p.lock("bench chain start")
	p.unsafe_appendPin(storedPin(0, nil, balances))
	p.unlock()
	return p
}

// BenchmarkPinBuild - forming a commit transaction: the balance lookups, the
// protobuf copy of every settled transaction, the smart-contract stage and the
// signature. This is the work that used to run with the pin lock held.
//
// One site set, reused. Building b.N site sets of 5000 up front holds hundreds
// of thousands of nodes live and makes the collector compete with the timed
// loop, which is what made an earlier commit benchmark read 22ms instead of 9ms.
func BenchmarkPinBuild(b *testing.B) {
	for _, sites := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("sites=%d", sites), func(b *testing.B) {
			p := benchBuildFixture(b, 64)
			set := paymentsFrom(1, sites, 64)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				p.lock("bench")
				if _, err := p.unsafe_buildPin(set, nil); err != nil {
					p.unlock()
					b.Fatalf("building: %s", err.Error())
				}
				p.unlock()
			}
		})
	}
}

// BenchmarkPinAddUnderReaderLoad - the whole leader path with the readers that
// used to wait behind it: what a REST balance query, an eth-RPC height and the
// insert path's balance lookup cost while a commit transaction is being formed.
// Reported per commit, with the reads each commit made alongside it.
func BenchmarkPinAddUnderReaderLoad(b *testing.B) {
	for _, sites := range []int{100, 1000} {
		b.Run(fmt.Sprintf("sites=%d", sites), func(b *testing.B) {
			p := benchBuildFixture(b, 64)
			set := paymentsFrom(1, sites, 64)
			stop := make(chan struct{})
			reads := make(chan int64, 1)
			go func() {
				var n int64
				for {
					select {
					case <-stop:
						reads <- n
						return
					default:
					}
					p.CurrentHeight()
					p.GetBalance(addr(0))
					n++
				}
			}()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := p.add(set, nil); err != nil {
					b.Fatalf("adding: %s", err.Error())
				}
			}
			b.StopTimer()
			close(stop)
			b.ReportMetric(float64(<-reads)/float64(b.N), "reads/commit")
		})
	}
}
