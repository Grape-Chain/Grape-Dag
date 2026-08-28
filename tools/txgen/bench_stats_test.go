package txgen

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestLatencyHistogramQuantilesBracketTheRecordedSamples(t *testing.T) {
	// A percentile is reported as the upper bound of the bucket it falls in, so
	// it may overstate by up to one bucket width and must never understate.
	for _, d := range []time.Duration{
		60 * time.Microsecond,
		400 * time.Microsecond,
		time.Millisecond,
		7 * time.Millisecond,
		120 * time.Millisecond,
		3 * time.Second,
	} {
		h := &latencyHistogram{}
		for i := 0; i < 1000; i++ {
			h.record(d)
		}
		for _, q := range []float64{0.5, 0.95, 0.99, 1.0} {
			got := h.quantile(q)
			if got < d {
				t.Errorf("p%.0f of 1000 samples of %s = %s, which understates the sample", q*100, d, got)
			}
			if limit := time.Duration(float64(d) * latencyBucketGrowth * latencyBucketGrowth); got > limit {
				t.Errorf("p%.0f of 1000 samples of %s = %s, want no more than %s", q*100, d, got, limit)
			}
		}
	}
}

func TestLatencyHistogramSeparatesTheTailFromTheBody(t *testing.T) {
	h := &latencyHistogram{}
	for i := 0; i < 950; i++ {
		h.record(time.Millisecond)
	}
	for i := 0; i < 50; i++ {
		h.record(200 * time.Millisecond)
	}
	p50, p99 := h.quantile(0.50), h.quantile(0.99)
	if p50 > 2*time.Millisecond {
		t.Errorf("p50 = %s, want about 1ms: 95%% of the samples were 1ms", p50)
	}
	if p99 < 200*time.Millisecond {
		t.Errorf("p99 = %s, want at least 200ms: the slowest 5%% were 200ms", p99)
	}
	if h.maxUs.Load() != uint64((200 * time.Millisecond).Microseconds()) {
		t.Errorf("max = %d us, want %d us", h.maxUs.Load(), (200 * time.Millisecond).Microseconds())
	}
}

func TestLatencyHistogramOfNothingReportsZero(t *testing.T) {
	h := &latencyHistogram{}
	if got := h.quantile(0.99); got != 0 {
		t.Errorf("p99 of an empty histogram = %s, want 0", got)
	}
	if got := h.mean(); got != 0 {
		t.Errorf("mean of an empty histogram = %s, want 0", got)
	}
}

func TestLatencyBucketsRiseWithTheDurationAndNeverOverflow(t *testing.T) {
	last := -1
	for d := time.Microsecond; d < time.Hour; d = d * 3 / 2 {
		i := latencyBucketIndex(d)
		if i < 0 || i >= latencyBucketCount {
			t.Fatalf("latencyBucketIndex(%s) = %d, outside [0, %d)", d, i, latencyBucketCount)
		}
		if i < last {
			t.Fatalf("latencyBucketIndex(%s) = %d, lower than the %d of a shorter duration", d, i, last)
		}
		if bound := latencyBucketUpperBound(i); bound < d && i != latencyBucketCount-1 {
			t.Fatalf("bucket %d for %s has upper bound %s, below the sample", i, d, bound)
		}
		last = i
	}
}

func TestPublishErrorsAreGroupedByCause(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"nothing", nil, ""},
		{"node out of queue space", status.Error(codes.ResourceExhausted, "queue full"), "grpc/resourceexhausted"},
		{"node not listening", status.Error(codes.Unavailable, "connection refused"), "grpc/unavailable"},
		{"call deadline", status.Error(codes.DeadlineExceeded, "context deadline exceeded"), "grpc/deadlineexceeded"},
		{"bare context deadline", context.DeadlineExceeded, "deadline exceeded"},
		{"bare cancellation", context.Canceled, "cancelled"},
		{"wrapped context deadline", fmt.Errorf("publishing: %w", context.DeadlineExceeded), "deadline exceeded"},
		{"not a status", errors.New("something else"), "other"},
	}
	for _, c := range cases {
		if got := classifyPublishError(c.err); got != c.want {
			t.Errorf("%s: classifyPublishError(%v) = %q, want %q", c.name, c.err, got, c.want)
		}
	}
}

func TestTheErrorBreakdownCannotGrowWithoutBound(t *testing.T) {
	s := newBenchStats()
	for i := 0; i < benchMaxErrorKinds*4; i++ {
		s.recordFailure(fmt.Sprintf("reason-%d", i))
	}
	report := s.snapshot(time.Second)
	if len(report.Errors) > benchMaxErrorKinds+1 {
		t.Errorf("breakdown holds %d kinds, want no more than %d plus \"other\"", len(report.Errors), benchMaxErrorKinds)
	}
	if report.Errors["other"] == 0 {
		t.Error("failures past the cap should be collected under \"other\"")
	}
	if report.Failed != uint64(benchMaxErrorKinds*4) {
		t.Errorf("failed count = %d, want %d: capping the breakdown must not lose a count", report.Failed, benchMaxErrorKinds*4)
	}
}

func TestBenchReportShowsTheAcceptedToOfferedRatio(t *testing.T) {
	s := newBenchStats()
	for i := 0; i < 90; i++ {
		s.offered.Add(1)
		s.recordAccepted(time.Millisecond)
	}
	for i := 0; i < 10; i++ {
		s.offered.Add(1)
		s.recordFailure("grpc/resourceexhausted")
	}
	r := s.snapshot(2 * time.Second)

	if r.OfferedRate() != 50 {
		t.Errorf("offered rate = %.1f/s, want 50/s (100 tx in 2 sec)", r.OfferedRate())
	}
	if r.AcceptedRate() != 45 {
		t.Errorf("accepted rate = %.1f/s, want 45/s (90 tx in 2 sec)", r.AcceptedRate())
	}
	if r.AcceptedRatio() != 0.9 {
		t.Errorf("accepted/offered = %.4f, want 0.9", r.AcceptedRatio())
	}

	out := r.String()
	for _, want := range []string{"offered", "accepted", "failed", "accepted/offered", "publish latency", "grpc/resourceexhausted"} {
		if !strings.Contains(out, want) {
			t.Errorf("the report does not mention %q:\n%s", want, out)
		}
	}
}

func TestAnEmptyBenchReportDoesNotDivideByZero(t *testing.T) {
	r := newBenchStats().snapshot(0)
	if r.OfferedRate() != 0 || r.AcceptedRate() != 0 || r.AcceptedRatio() != 0 {
		t.Errorf("a report of nothing over no time = %+v, want zeroes", r)
	}
	if !strings.Contains(r.String(), "errors            : none") {
		t.Errorf("a report with no failures should say so:\n%s", r.String())
	}
}

func TestProgressLinesRateOverTheIntervalNotTheWholeRun(t *testing.T) {
	s := newBenchStats()
	for i := 0; i < 1000; i++ {
		s.offered.Add(1)
		s.recordAccepted(time.Millisecond)
	}
	first := s.snapshot(10 * time.Second) // 100/s so far
	for i := 0; i < 5000; i++ {
		s.offered.Add(1)
		s.recordAccepted(time.Millisecond)
	}
	second := s.snapshot(15 * time.Second) // 1000/s over the last five seconds

	line := second.progress(first)
	if !strings.Contains(line, "1000.0/s") {
		t.Errorf("progress line should rate the interval at 1000/s, not the run at 400/s:\n%s", line)
	}
}

func TestSortedErrorKindsPutsTheDominantFailureFirst(t *testing.T) {
	got := sortedErrorKinds(map[string]uint64{"rare": 1, "common": 100, "also-rare": 1})
	want := []string{"common", "also-rare", "rare"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sortedErrorKinds = %v, want %v", got, want)
		}
	}
}
