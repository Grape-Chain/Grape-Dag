package dag

import (
	"github.com/Grape-Chain/Grape-Dag/stats"
)

// refreshSizeGauges - sample the sizes that are supposed to stay bounded.
//
// Sampled once per commit transaction rather than on every insert. A commit is
// the natural interval: it is when the live graph is sliced and the confirmation
// frontier is drained, so it is the point where "bounded" either holds or does
// not. Sampling per insert would put a lock acquisition on the hot path to
// measure something that only changes at commit time.
//
// Never waits for dag.mux. Two goroutines take dag.mux and the pin mutex in
// opposite orders - the subscriber's insert path takes dag.mux then the pin lock
// to resolve an approval target, while a follower applying a commit transaction
// holds the pin lock and then takes dag.mux to slice - so any new blocking
// acquisition here would widen a deadlock rather than measure one. A gauge is a
// sample; a skipped sample costs a point on a graph, and the next commit takes
// another one.
func refreshSizeGauges() {
	if _dag_ != nil {
		if _dag_.mux.TryLock() {
			liveSites, liveLinks := len(_dag_._dag_), len(_dag_._links_)
			_dag_.mux.Unlock()
			stats.LiveSites.Set(float64(liveSites))
			stats.LiveLinks.Set(float64(liveLinks))
		} else {
			stats.GaugeSkips.Inc()
		}
	}
	stats.ArchiveSites.Set(float64(archiveLen()))

	// Only the share-of-tips rule keeps a frontier to measure; the legacy
	// counter has no equivalent, and reporting zero for it would read as a
	// healthy frontier rather than as an absent one.
	if tr, ok := confirmationCounter.(*ConfirmTracker); ok {
		active, tips, pending := tr.stats()
		stats.ConfirmActive.Set(float64(active))
		stats.ConfirmTips.Set(float64(tips))
		stats.ConfirmPending.Set(float64(pending))
	}
}
