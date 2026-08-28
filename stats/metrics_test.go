package stats

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The endpoint has to actually serve, and it has to serve the series the
// throughput target is stated in. A metrics endpoint that returns an empty body
// looks healthy to everything except the person reading it.
func TestMetricsEndpointServesTheSeriesWeMeasureOn(t *testing.T) {
	PinCommit.Observe(0.01)
	PinStoreAppend.Observe(0.005)
	TxAccepted.Inc()
	TxRejected.WithLabelValues("rejected").Inc()
	LiveSites.Set(42)

	rec := httptest.NewRecorder()
	MetricsHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from /metrics, got %d", rec.Code)
	}
	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("reading the response: %s", err.Error())
	}
	got := string(body)

	for _, want := range []string{
		"grape_pin_commit_seconds",
		"grape_pin_store_append_seconds",
		"grape_pin_settled_apply_seconds",
		"grape_tx_accepted_total",
		`grape_tx_rejected_total{reason="rejected"}`,
		"grape_live_sites 42",
		// The memory bound in the throughput target is stated in this one, and
		// it comes from a collector rather than from this file - so it is the
		// one that silently disappears if the registry is rebuilt.
		"process_resident_memory_bytes",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("/metrics does not expose %s", want)
		}
	}
}

// testMetricSeq gives each registration in this file a name of its own.
//
// The registry rejects a duplicate name by panicking, which is the right
// behaviour for a process - two metrics under one name is a bug that should stop
// start-up - but it meant this file could not survive go test -count=2, and
// -count is how a flaky test gets found. The sequence is the smallest thing that
// fixes the test without weakening the guard in the code being tested.
var testMetricSeq atomic.Uint64

func TestTimeRecordsTheElapsedTime(t *testing.T) {
	name := fmt.Sprintf("test_elapsed_%d_seconds", testMetricSeq.Add(1))
	h := newHistogram(name, "Test only.")
	done := Time(h)
	time.Sleep(2 * time.Millisecond)
	done()

	rec := httptest.NewRecorder()
	MetricsHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), "grape_"+name+"_count 1") {
		t.Fatal("Time() did not record an observation")
	}
	// The 2ms sleep must land above the smallest bucket, or the timer is not
	// measuring anything.
	if strings.Contains(string(body), fmt.Sprintf("grape_%s_bucket{le=\"5e-05\"} 1", name)) {
		t.Fatal("a 2ms interval was recorded in the 50us bucket")
	}
}
