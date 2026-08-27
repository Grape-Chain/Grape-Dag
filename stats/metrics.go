package stats

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

/*
Metrics for the parts of the node whose cost is a design question rather than a
detail.

The immediate reason this exists: committing a commit transaction now writes to
disk with an fsync and replays the settled balances, both on the synchronous
path, and both were added without anything measuring them. "It still works" is
not the same as "it still works at a thousand transactions a second", and the
difference between those two statements is the only thing standing between the
throughput target and a guess.

What is deliberately measured separately rather than as one number: a commit is
a store append, a balance replay, an occasional balance snapshot and a slice of
the live graph. Timing only the total would say a commit got slower without
saying which of the four to look at, and they have very different fixes - the
append is an fsync and could be batched, the replay is arithmetic over the
sites in the pin, the snapshot is proportional to the number of accounts, and
the slice is proportional to the live graph.

Everything here is registered on this package's own registry rather than the
global default one, because libp2p registers collectors on the default registry
and a duplicate registration there panics at start-up.
*/

// latency - 50us to about 7s. A site insert should sit at the bottom of this
// range and a commit near the middle; the top exists so that a commit that has
// gone badly wrong is visible as a number rather than as an overflow.
var latency = prometheus.ExponentialBuckets(0.00005, 2.5, 14)

var (
	registry = prometheus.NewRegistry()

	// ---------------------------------------------------------- ingress path

	// TxIngress - accepting a transaction over the API, up to the point it is
	// queued. The offered-load side of the throughput measurement.
	TxIngress = newHistogram("tx_ingress_seconds", "Time to accept a submitted transaction.")
	// TxAccepted / TxRejected - the denominator and numerator behind
	// accepted-versus-offered. A benchmark that reports only accepted
	// throughput is reporting whatever the node felt like taking.
	TxAccepted = newCounter("tx_accepted_total", "Transactions accepted for inclusion.")
	TxRejected = newCounterVec("tx_rejected_total", "Transactions refused at ingress.", "reason")

	// ---------------------------------------------------------- insert path

	// SiteInsert - encapsulating a transaction into a site and linking it into
	// the graph, tip selection included.
	SiteInsert = newHistogram("site_insert_seconds", "Time to add a site to the live graph.")
	// TipSelection - the random walk on its own. Broken out because it is the
	// part whose cost depends on the shape of the graph rather than on the
	// transaction, so it is the part that degrades quietly.
	TipSelection = newHistogram("tip_selection_seconds", "Time to choose the sites a new site approves.")
	// WalkSteps - how far a walk travels. Rising step counts mean the
	// unconfirmed region is getting deeper, which is the early warning that
	// confirmation is falling behind insertion.
	// WalkSteps - the first boundary is below one on purpose. A walk that takes
	// no steps at all is the failure mode this metric exists to catch (it means
	// selection has degenerated to returning its own starting point), and an
	// exponential scale starting at 1 cannot tell that apart from a healthy
	// one-step walk.
	WalkSteps = newHistogram2("walk_steps", "Steps taken by one tip-selection walk that reached a site to approve.",
		append([]float64{0.5}, prometheus.ExponentialBuckets(1, 2, 13)...))
	// WalksAbandoned - walks that ended without anything to approve, by reason.
	// Observed nowhere in WalkSteps, so without this they are invisible.
	WalksAbandoned = newCounterVec("walks_abandoned_total", "Tip-selection walks that found nothing to approve.", "reason")
	// SelectionFallbacks - selections that fell back to a uniform pick because
	// the walk came back empty. A rising count means the bias is not being
	// applied, whatever the walk histogram says.
	SelectionFallbacks = newCounter("selection_fallbacks_total", "Selections that fell back to a uniform pick of tips.")
	// GaugeSkips - size samples skipped because dag.mux was busy.
	GaugeSkips = newCounter("gauge_samples_skipped_total", "Size gauge samples skipped to avoid waiting on the dag lock.")
	SitesAdded = newCounter("sites_added_total", "Sites added to the live graph.")

	// ---------------------------------------------------------- commit path

	PinBuild  = newHistogram("pin_build_seconds", "Time to build a commit transaction.")
	PinCommit = newHistogram("pin_commit_seconds", "Time to commit a commit transaction, end to end.")
	// PinStoreAppend - the durability boundary: one synchronous, fsynced batch.
	PinStoreAppend = newHistogram("pin_store_append_seconds", "Time to write a commit transaction to disk, including the fsync.")
	// PinSettledApply - replaying the pin's payments into the settled balances.
	PinSettledApply = newHistogram("pin_settled_apply_seconds", "Time to fold a commit transaction into the settled balances.")
	// PinBalanceSnapshot - written every snapshotEveryPins commits, so it shows
	// up as a periodic spike rather than as steady cost.
	PinBalanceSnapshot = newHistogram("pin_balance_snapshot_seconds", "Time to write the settled balance snapshot.")
	// PinSlice - taking the settled sites out of the live graph.
	PinSlice = newHistogram("pin_slice_seconds", "Time to slice settled sites out of the live graph.")

	PinSites       = newHistogram2("pin_sites", "Sites settled by one commit transaction.", prometheus.ExponentialBuckets(1, 2, 16))
	PinsCommitted  = newCounter("pins_committed_total", "Commit transactions committed.")
	SitesConfirmed = newCounter("sites_confirmed_total", "Sites confirmed and written into a commit transaction.")
	StoreErrors    = newCounterVec("store_errors_total", "Failed store operations.", "op")

	// ---------------------------------------------------------- size gauges
	//
	// These are what say whether memory is bounded. Slicing and confirmation
	// are both supposed to hold their working sets steady under sustained load;
	// without these that claim rests on watching the process size.

	LiveSites      = newGauge("live_sites", "Sites currently in the live graph.")
	LiveLinks      = newGauge("live_links", "Approval edges currently in the live graph.")
	ArchiveSites   = newGauge("archive_sites", "Settled sites held in the slice archive.")
	ConfirmActive  = newGauge("confirm_active_sites", "Sites in the confirmation frontier.")
	ConfirmTips    = newGauge("confirm_tips", "Sites counting towards the confirmation denominator.")
	ConfirmPending = newGauge("confirm_pending", "Confirmed sites waiting for a commit transaction.")
	WalkRoots      = newGauge("walk_roots", "Entry points for a tip-selection walk.")
	PinHeight      = newGauge("pin_height", "Number of the newest commit transaction.")
)

func newHistogram(name, help string) prometheus.Histogram {
	return newHistogram2(name, help, latency)
}

func newHistogram2(name, help string, buckets []float64) prometheus.Histogram {
	h := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "grape", Name: name, Help: help, Buckets: buckets,
	})
	registry.MustRegister(h)
	return h
}

func newCounter(name, help string) prometheus.Counter {
	c := prometheus.NewCounter(prometheus.CounterOpts{Namespace: "grape", Name: name, Help: help})
	registry.MustRegister(c)
	return c
}

func newCounterVec(name, help string, labels ...string) *prometheus.CounterVec {
	c := prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: "grape", Name: name, Help: help}, labels)
	registry.MustRegister(c)
	return c
}

func newGauge(name, help string) prometheus.Gauge {
	g := prometheus.NewGauge(prometheus.GaugeOpts{Namespace: "grape", Name: name, Help: help})
	registry.MustRegister(g)
	return g
}

func init() {
	// process_resident_memory_bytes comes from here, and it is the number the
	// memory-bound part of the throughput target is stated in.
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	registry.MustRegister(collectors.NewGoCollector())
}

// Time - start a timer, and observe it when the returned function is called:
//
//	defer stats.Time(stats.PinCommit)()
//
// Reads as one line at the top of the function it measures, which is the only
// form of instrumentation that survives someone editing the function later.
func Time(h prometheus.Observer) func() {
	start := time.Now()
	return func() { h.Observe(time.Since(start).Seconds()) }
}

// Since - for the cases where the timer cannot be a defer, because only part of
// the function is being measured.
func Since(h prometheus.Observer, start time.Time) {
	h.Observe(time.Since(start).Seconds())
}

// MetricsHandler - the /metrics endpoint.
func MetricsHandler() http.Handler {
	return promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
}

// Registry - exposed so a test can read the collected values back.
func Registry() *prometheus.Registry { return registry }

// StartDiagnosticsServer - serve pprof and /metrics on one address.
//
// pprof registers its handlers on http.DefaultServeMux when net/http/pprof is
// imported, so that mux is the base and /metrics is added to it. There used to
// be two servers started on the same hard-coded address, and the second one's
// ListenAndServe error was discarded, so it failed silently on every run.
func StartDiagnosticsServer(addr string) {
	if addr == "" {
		addr = "127.0.0.1:6060"
	}
	mux := http.DefaultServeMux
	mux.Handle("/metrics", MetricsHandler())
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			if logger != nil {
				logger.Errorf("[metrics] Diagnostics server on %s stopped: %s", addr, err.Error())
			}
		}
	}()
	if logger != nil {
		logger.Infof("[metrics] Diagnostics server listening on http://%s (/debug/pprof, /metrics)", addr)
	}
}
