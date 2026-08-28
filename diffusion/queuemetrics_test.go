package diffusion

import (
	"net/http/httptest"
	"regexp"
	"strconv"
	"testing"
	"time"

	txqueue "github.com/Grape-Chain/Grape-Dag/queues"
	"github.com/Grape-Chain/Grape-Dag/stats"
)

/*
Ingress accepted 3,087 transactions a second while the graph inserted about
2,620, and the only thing between the two was a queue nothing measured. These
tests are about that gap being visible: the depth, the ceiling it is heading
for, and whether reaching it drops work or holds the producer.
*/

// scrape - the metrics as an operator would see them.
func scrape(t *testing.T) string {
	t.Helper()
	rec := httptest.NewRecorder()
	stats.MetricsHandler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if rec.Code != 200 {
		t.Fatalf("scraping /metrics returned %d", rec.Code)
	}
	return rec.Body.String()
}

// metricValue - one sample's value, by metric name and queue label.
func metricValue(t *testing.T, body, name, queue string) (float64, bool) {
	t.Helper()
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(name) + `\{queue="` + regexp.QuoteMeta(queue) + `"\} (\S+)$`)
	m := re.FindStringSubmatch(body)
	if m == nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		t.Fatalf("%s{queue=%q} is not a number: %q", name, queue, m[1])
	}
	return v, true
}

func TestTheInsertQueuesDepthAndCeilingAreExposedForScraping(t *testing.T) {
	q := txqueue.NewLockFreeQueueCapOf[SubInTx](true, 8)
	stats.RegisterQueue("test_insert", q)

	body := scrape(t)
	depth, ok := metricValue(t, body, "grape_queue_depth", "test_insert")
	if !ok {
		t.Fatalf("grape_queue_depth{queue=\"test_insert\"} is not exposed:\n%s", body)
	}
	if depth != 0 {
		t.Fatalf("expected an empty queue to report depth 0, got %v", depth)
	}
	capacity, ok := metricValue(t, body, "grape_queue_capacity", "test_insert")
	if !ok {
		t.Fatal("grape_queue_capacity is not exposed; the depth alone does not say what it is heading for")
	}
	if capacity != 8 {
		t.Fatalf("expected the ceiling of 8 to be reported, got %v", capacity)
	}

	// The gauge has to follow the queue, not a value captured at registration.
	for i := 0; i < 3; i++ {
		q.Enqueue(SubInTx{})
	}
	if depth, _ := metricValue(t, scrape(t), "grape_queue_depth", "test_insert"); depth != 3 {
		t.Fatalf("expected the depth to follow the queue to 3, got %v", depth)
	}
	q.TryDequeue()
	if depth, _ := metricValue(t, scrape(t), "grape_queue_depth", "test_insert"); depth != 2 {
		t.Fatalf("expected the depth to fall back to 2, got %v", depth)
	}
}

// TestBackpressureOnTheInsertQueueIsCountedRatherThanSilent - the queue holds
// the producer rather than dropping, so the way to tell "the node absorbs this
// rate" from "the queue is absorbing the difference" is this counter moving.
func TestBackpressureOnTheInsertQueueIsCountedRatherThanSilent(t *testing.T) {
	q := txqueue.NewLockFreeQueueCapOf[SubInTx](true, 2)
	stats.RegisterQueue("test_backpressure", q)

	if blocked, ok := metricValue(t, scrape(t), "grape_queue_enqueue_blocked_total", "test_backpressure"); !ok || blocked != 0 {
		t.Fatalf("expected a fresh queue to report no blocked enqueues, got %v (present=%t)", blocked, ok)
	}

	q.Enqueue(SubInTx{})
	q.Enqueue(SubInTx{})
	held := make(chan struct{})
	go func() {
		q.Enqueue(SubInTx{})
		close(held)
	}()
	// Wait until the producer is genuinely being held before draining. Draining
	// first would leave room, the enqueue would sail through, and the test would
	// fail for a reason that has nothing to do with what it is checking.
	deadline := time.Now().Add(10 * time.Second)
	for q.EnqueueBlocked() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("an enqueue past the ceiling never waited; the ceiling is not backpressure")
		}
		time.Sleep(time.Millisecond)
	}
	q.TryDequeue()
	<-held

	body := scrape(t)
	blocked, ok := metricValue(t, body, "grape_queue_enqueue_blocked_total", "test_backpressure")
	if !ok {
		t.Fatal("grape_queue_enqueue_blocked_total is not exposed")
	}
	if blocked < 1 {
		t.Fatalf("expected the held enqueue to be counted, got %v", blocked)
	}
	if _, ok := metricValue(t, body, "grape_queue_enqueue_wait_seconds_total", "test_backpressure"); !ok {
		t.Fatal("grape_queue_enqueue_wait_seconds_total is not exposed; the count alone does not say how hard the producer was held")
	}
}

// TestReRegisteringAQueueRepointsTheGaugeRatherThanPanicking - the subscriber
// builds a new insert queue every time it starts, and a collector still reading
// the old one would report a queue nothing is using.
func TestReRegisteringAQueueRepointsTheGaugeRatherThanPanicking(t *testing.T) {
	first := txqueue.NewLockFreeQueueCapOf[SubInTx](true, 4)
	stats.RegisterQueue("test_rebuild", first)
	first.Enqueue(SubInTx{})
	if depth, _ := metricValue(t, scrape(t), "grape_queue_depth", "test_rebuild"); depth != 1 {
		t.Fatalf("expected depth 1 from the first queue, got %v", depth)
	}

	second := txqueue.NewLockFreeQueueCapOf[SubInTx](true, 16)
	stats.RegisterQueue("test_rebuild", second)

	body := scrape(t)
	if depth, _ := metricValue(t, body, "grape_queue_depth", "test_rebuild"); depth != 0 {
		t.Fatalf("expected the gauge to follow the new queue and read 0, got %v", depth)
	}
	if capacity, _ := metricValue(t, body, "grape_queue_capacity", "test_rebuild"); capacity != 16 {
		t.Fatalf("expected the new queue's ceiling of 16, got %v", capacity)
	}
}

// labelledValue - one sample's value by metric name and a single label, and
// whether it was there at all.
//
// A counter that has never been touched has no sample, so "is it reported" and
// "what does it say" are different questions and callers need both. Absent
// counts as zero, which is what makes a before-and-after difference the right
// assertion: a test that only checks a sample exists passes on whatever an
// earlier test in the same process happened to leave behind.
func labelledValue(t *testing.T, body, name, label, value string) (float64, bool) {
	t.Helper()
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(name) + `\{` + regexp.QuoteMeta(label) + `="` + regexp.QuoteMeta(value) + `"\} (\S+)$`)
	m := re.FindStringSubmatch(body)
	if m == nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		t.Fatalf("%s{%s=%q} is not a number: %q", name, label, value, m[1])
	}
	return v, true
}
