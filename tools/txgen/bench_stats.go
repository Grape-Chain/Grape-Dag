package txgen

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc/status"
)

// Latency is recorded into exponentially spaced buckets rather than kept as one
// sample per transaction. A saturation run offers hundreds of thousands of
// transactions, and both alternatives cost more than they are worth: a slice per
// worker has to be copied under a lock every time the periodic report wants a
// percentile, and a shared slice puts a lock on the hot path. Buckets are atomic
// counters, so recording is a single add and a percentile can be taken at any
// moment without disturbing the workers.
//
// A reported percentile is the upper bound of the bucket the true value falls in,
// so it overstates by at most latencyBucketGrowth - that is 15%, which is finer
// than the run-to-run variation this tool is used to compare.
const (
	latencyBucketBaseUs = 50.0 // upper bound of the first bucket, microseconds
	latencyBucketGrowth = 1.15
	latencyBucketCount  = 112 // reaches about 275 seconds, past any useful deadline
)

type latencyHistogram struct {
	buckets [latencyBucketCount]atomic.Uint64
	count   atomic.Uint64
	sumUs   atomic.Uint64
	maxUs   atomic.Uint64
}

// latencyBucketUpperBound - the largest latency that lands in bucket i. Rounded
// up to the microsecond: a bound rounded down would sit below a sample that the
// index function, which truncates to whole microseconds, put in this bucket, and
// the reported percentile would then understate the sample it came from.
func latencyBucketUpperBound(i int) time.Duration {
	us := latencyBucketBaseUs * math.Pow(latencyBucketGrowth, float64(i))
	return time.Duration(math.Ceil(us)) * time.Microsecond
}

func latencyBucketIndex(d time.Duration) int {
	us := float64(d.Microseconds())
	if us <= latencyBucketBaseUs {
		return 0
	}
	i := int(math.Ceil(math.Log(us/latencyBucketBaseUs) / math.Log(latencyBucketGrowth)))
	if i < 0 {
		return 0
	}
	if i >= latencyBucketCount {
		return latencyBucketCount - 1
	}
	return i
}

func (h *latencyHistogram) record(d time.Duration) {
	if d < 0 {
		d = 0
	}
	us := uint64(d.Microseconds())
	h.buckets[latencyBucketIndex(d)].Add(1)
	h.count.Add(1)
	h.sumUs.Add(us)
	for {
		cur := h.maxUs.Load()
		if us <= cur || h.maxUs.CompareAndSwap(cur, us) {
			break
		}
	}
}

// quantile returns the upper bound of the bucket holding the q-th quantile, with
// q in (0,1]. It returns 0 when nothing has been recorded. Buckets are read one
// at a time while workers are still writing, so a percentile taken mid-run is a
// slightly smeared snapshot rather than a consistent one; the final report is
// taken after the workers have stopped and is exact to the bucket width.
func (h *latencyHistogram) quantile(q float64) time.Duration {
	total := h.count.Load()
	if total == 0 {
		return 0
	}
	want := uint64(math.Ceil(q * float64(total)))
	if want < 1 {
		want = 1
	}
	var seen uint64
	for i := 0; i < latencyBucketCount; i++ {
		seen += h.buckets[i].Load()
		if seen >= want {
			return latencyBucketUpperBound(i)
		}
	}
	return latencyBucketUpperBound(latencyBucketCount - 1)
}

func (h *latencyHistogram) mean() time.Duration {
	n := h.count.Load()
	if n == 0 {
		return 0
	}
	return time.Duration(h.sumUs.Load()/n) * time.Microsecond
}

// benchMaxErrorKinds bounds the error breakdown. gRPC codes are a short list, but
// a bug that put per-call detail into a key would otherwise grow the map once per
// failed transaction.
const benchMaxErrorKinds = 32

type benchStats struct {
	offered  atomic.Uint64
	accepted atomic.Uint64
	failed   atomic.Uint64
	// stalled counts the times a sender had nothing signed to send and had to
	// wait for its own signer. It is the generator's confession: while a sender
	// is waiting, the rate being measured is the generator's signing rate rather
	// than the node's capacity, so a run with a meaningful stall count has not
	// measured what it claims to.
	stalled atomic.Uint64
	latency latencyHistogram

	mu     sync.Mutex
	errors map[string]uint64
}

func newBenchStats() *benchStats {
	return &benchStats{errors: make(map[string]uint64)}
}

func (s *benchStats) recordAccepted(latency time.Duration) {
	s.accepted.Add(1)
	s.latency.record(latency)
}

func (s *benchStats) recordFailure(reason string) {
	s.failed.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, known := s.errors[reason]; !known && len(s.errors) >= benchMaxErrorKinds {
		reason = "other"
	}
	s.errors[reason]++
}

// classifyPublishError reduces an error to a short key so that the breakdown
// groups failures by cause instead of listing one line per transaction. Only the
// gRPC code is kept where there is one, because status messages carry per-call
// detail.
func classifyPublishError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline exceeded"
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	if s, ok := status.FromError(err); ok {
		return "grpc/" + strings.ToLower(s.Code().String())
	}
	return "other"
}

// benchReport is a snapshot of the counters. Taking a struct rather than reading
// the counters twice keeps the printed offered rate, accepted rate and ratio
// consistent with each other.
type benchReport struct {
	Elapsed  time.Duration
	Offered  uint64
	Accepted uint64
	Failed   uint64
	Stalled  uint64
	Mean     time.Duration
	P50      time.Duration
	P95      time.Duration
	P99      time.Duration
	Max      time.Duration
	Errors   map[string]uint64
}

func (s *benchStats) snapshot(elapsed time.Duration) benchReport {
	r := benchReport{
		Elapsed:  elapsed,
		Offered:  s.offered.Load(),
		Accepted: s.accepted.Load(),
		Failed:   s.failed.Load(),
		Stalled:  s.stalled.Load(),
		Mean:     s.latency.mean(),
		P50:      s.latency.quantile(0.50),
		P95:      s.latency.quantile(0.95),
		P99:      s.latency.quantile(0.99),
		Max:      time.Duration(s.latency.maxUs.Load()) * time.Microsecond,
		Errors:   make(map[string]uint64),
	}
	s.mu.Lock()
	for k, v := range s.errors {
		r.Errors[k] = v
	}
	s.mu.Unlock()
	return r
}

// formatLatency prints a latency at a precision worth reading: whole
// microseconds below a millisecond, tenths of a millisecond up to a second, and
// milliseconds above that. One fixed rounding step either hides a sub-millisecond
// result or fills the line with digits nobody uses.
func formatLatency(d time.Duration) string {
	switch {
	case d <= 0:
		return "0s"
	case d < time.Millisecond:
		return d.Round(time.Microsecond).String()
	case d < time.Second:
		return d.Round(100 * time.Microsecond).String()
	default:
		return d.Round(time.Millisecond).String()
	}
}

func perSecond(n uint64, d time.Duration) float64 {
	if d <= 0 {
		return 0
	}
	return float64(n) / d.Seconds()
}

// OfferedRate - transactions handed to the node per second, whether or not it
// took them.
func (r benchReport) OfferedRate() float64 { return perSecond(r.Offered, r.Elapsed) }

// AcceptedRate - transactions the node acknowledged per second. This is the
// number to read as the node's throughput.
func (r benchReport) AcceptedRate() float64 { return perSecond(r.Accepted, r.Elapsed) }

// AcceptedRatio - the share of offered transactions the node took. A ratio that
// falls away from 1 while the offered rate keeps climbing is the node refusing
// load, which is the signal that the ceiling has been found.
// StalledPercent - stalls as a share of what was offered. The number that says
// whether the run measured the node or the generator.
func (r benchReport) StalledPercent() float64 {
	if r.Offered == 0 {
		return 0
	}
	return 100 * float64(r.Stalled) / float64(r.Offered)
}

// benchStallWarnPercent - above this share of offered transactions, the senders
// spent enough time waiting on their own signers that the throughput figure is
// the generator's and not the node's. One percent is deliberately strict: the
// point of the reserve is that stalls should be rare, and a run that is only
// just under the line is a run to repeat with more workers or a bigger reserve.
const benchStallWarnPercent = 1.0

func (r benchReport) stallVerdict() string {
	switch {
	case r.Stalled == 0:
		return ""
	case r.StalledPercent() >= benchStallWarnPercent:
		return "  <- the generator could not keep up; this is not the node's ceiling"
	default:
		return "  (negligible)"
	}
}

func (r benchReport) AcceptedRatio() float64 {
	if r.Offered == 0 {
		return 0
	}
	return float64(r.Accepted) / float64(r.Offered)
}

// progress renders one periodic line, with rates over the interval since prev
// rather than since the start, so a change in the node's behaviour shows up in
// the line where it happened instead of being averaged away.
func (r benchReport) progress(prev benchReport) string {
	window := r.Elapsed - prev.Elapsed
	return fmt.Sprintf(
		"[bench] t=%5.1fs offered %8d (%9.1f/s) accepted %8d (%9.1f/s) failed %6d ratio %5.3f p50 %8s p95 %8s p99 %8s",
		r.Elapsed.Seconds(),
		r.Offered, perSecond(r.Offered-prev.Offered, window),
		r.Accepted, perSecond(r.Accepted-prev.Accepted, window),
		r.Failed,
		r.AcceptedRatio(),
		formatLatency(r.P50),
		formatLatency(r.P95),
		formatLatency(r.P99),
	)
}

func (r benchReport) String() string {
	var b strings.Builder
	b.WriteString("\n--- bench report ------------------------------------------------\n")
	fmt.Fprintf(&b, "  window            : %.2f sec\n", r.Elapsed.Seconds())
	fmt.Fprintf(&b, "  offered           : %d tx (%.1f tx/s)\n", r.Offered, r.OfferedRate())
	fmt.Fprintf(&b, "  accepted          : %d tx (%.1f tx/s)\n", r.Accepted, r.AcceptedRate())
	fmt.Fprintf(&b, "  failed            : %d tx\n", r.Failed)
	fmt.Fprintf(&b, "  accepted/offered  : %.4f\n", r.AcceptedRatio())
	fmt.Fprintf(&b, "  generator stalls  : %d (%.2f%% of offered)%s\n",
		r.Stalled, r.StalledPercent(), r.stallVerdict())
	fmt.Fprintf(&b, "  publish latency   : p50 %s  p95 %s  p99 %s  max %s  mean %s\n",
		formatLatency(r.P50),
		formatLatency(r.P95),
		formatLatency(r.P99),
		formatLatency(r.Max),
		formatLatency(r.Mean),
	)
	if len(r.Errors) == 0 {
		b.WriteString("  errors            : none\n")
	} else {
		b.WriteString("  errors            :\n")
		for _, k := range sortedErrorKinds(r.Errors) {
			fmt.Fprintf(&b, "      %-28s %d\n", k, r.Errors[k])
		}
	}
	b.WriteString("-----------------------------------------------------------------\n")
	return b.String()
}

// sortedErrorKinds orders the breakdown by count so the dominant failure is the
// first line, with the name as the tie-break so repeated runs print alike.
func sortedErrorKinds(errs map[string]uint64) []string {
	keys := make([]string, 0, len(errs))
	for k := range errs {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if errs[keys[i]] != errs[keys[j]] {
			return errs[keys[i]] > errs[keys[j]]
		}
		return keys[i] < keys[j]
	})
	return keys
}
