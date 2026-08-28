package node

import (
	"sync"
	"sync/atomic"
	"time"
)

// processing - the gate that decides whether this node encapsulates transactions
// into sites and so earns fees. It is package-level and atomic because the
// publish loop reads it once per transaction on its own goroutine while an HTTP
// handler may be writing it.
//
// The gate governs encapsulation and nothing else. It does not stop the node
// syncing, validating, gossiping or applying commit transactions - that is the
// whole point of it, and it is why the check belongs in the publish loop rather
// than anywhere that would take the node off the network.
//
// Set during package variable initialisation rather than in an init function,
// which runs later: the gate has to hold its intended value before any other
// package variable in the process can read it.
var processing = enabledGate()

// enabledGate - a gate that starts enabled, so that adding the check to the
// publish loop does not by itself change how any existing node behaves.
// Stopping is a decision somebody makes.
func enabledGate() *atomic.Bool {
	var gate atomic.Bool
	gate.Store(true)
	return &gate
}

// ProcessingEnabled - whether this node should encapsulate transactions into
// sites. This is the call the publish loop makes.
func ProcessingEnabled() bool {
	return processing.Load()
}

// SetProcessing - start or stop encapsulation, returning what the gate was set
// to before. Safe to call from any goroutine.
func SetProcessing(enabled bool) (previous bool) {
	return processing.Swap(enabled)
}

// tpsWindow - the trailing window over which this node's own contribution is
// averaged. Long enough that a single slow moment does not read as a stall,
// short enough that stopping shows up in the figure while the operator is still
// looking at the page.
const tpsWindow = 10 * time.Second

// tpsMinSample - the shortest partial window worth dividing by. Below it the
// previous window's figure is reported instead, because one transaction 20ms
// into a window is not 50 tps.
const tpsMinSample = time.Second

// rateMeter - a windowed counter of the transactions this node has itself
// encapsulated. Deliberately not a stats.Gauge: this counts what this node did,
// not what the network did, and it has to be readable from an HTTP handler
// without pulling in the metrics registry.
type rateMeter struct {
	mu          sync.Mutex
	windowStart time.Time
	count       uint64
	settled     float64 // the last completed window's rate
}

var contribution rateMeter

// RecordProcessed - note that this node has just encapsulated one transaction
// into a site. Safe to call from the publish loop on every transaction.
func RecordProcessed() {
	contribution.record(time.Now())
}

// TpsContribution - transactions per second this node has itself encapsulated,
// averaged over a trailing window. Zero on a node that is not processing.
func TpsContribution() float64 {
	return contribution.rate(time.Now())
}

func (m *rateMeter) record(now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.roll(now)
	m.count++
}

func (m *rateMeter) rate(now time.Time) float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Rolling on read as well as on write is what makes an idle node's figure
	// decay to zero instead of freezing at whatever it was when the last
	// transaction went through.
	m.roll(now)
	if elapsed := now.Sub(m.windowStart); elapsed >= tpsMinSample {
		return float64(m.count) / elapsed.Seconds()
	}
	return m.settled
}

// roll - close the current window once it has run its length. Caller holds m.mu.
func (m *rateMeter) roll(now time.Time) {
	if m.windowStart.IsZero() {
		m.windowStart = now
		return
	}
	elapsed := now.Sub(m.windowStart)
	if elapsed < tpsWindow {
		return
	}
	m.settled = float64(m.count) / elapsed.Seconds()
	m.count = 0
	m.windowStart = now
}

// reset - drop the accumulated window. For tests; a running node has no reason
// to forget what it has done.
func (m *rateMeter) reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.windowStart = time.Time{}
	m.count = 0
	m.settled = 0
}
