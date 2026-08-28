package node

import (
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeLedger - a Ledger whose answers a test sets directly.
type fakeLedger struct {
	pinHeight int64
	peers     int
	syncing   bool
	wallet    string
	lifetime  *big.Int
	pending   *big.Int
	recent    []Credit
	err       error

	askedFor []string // wallets EarningsFor was called with
}

func (f *fakeLedger) PinHeight() int64      { return f.pinHeight }
func (f *fakeLedger) PeerCount() int        { return f.peers }
func (f *fakeLedger) Syncing() bool         { return f.syncing }
func (f *fakeLedger) WalletAddress() string { return f.wallet }

func (f *fakeLedger) EarningsFor(wallet string) (*big.Int, *big.Int, []Credit, error) {
	f.askedFor = append(f.askedFor, wallet)
	return f.lifetime, f.pending, f.recent, f.err
}

func do(t *testing.T, h http.Handler, method, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, reader))
	var decoded map[string]any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
			t.Fatalf("%s %s answered a body that is not a JSON object: %s\n%s", method, path, err, rec.Body.String())
		}
	}
	return rec, decoded
}

func TestTheStatusEndpointReportsTheFiveFieldsAWalletNeeds(t *testing.T) {
	restoreGate(t)
	SetProcessing(true)

	svc := NewService(&fakeLedger{pinHeight: 4231, peers: 7, syncing: false, wallet: "0xabc"})
	rec, body := do(t, svc.Routes(), http.MethodGet, StatusPath, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", StatusPath, rec.Code)
	}
	for _, key := range []string{"state", "wallet", "pinHeight", "peers", "tpsContribution"} {
		if _, ok := body[key]; !ok {
			t.Errorf("the status response has no %q field: %v", key, body)
		}
	}
	if body["state"] != string(StateProcessing) {
		t.Errorf("state = %v, want %q", body["state"], StateProcessing)
	}
	if body["wallet"] != "0xabc" {
		t.Errorf("wallet = %v, want 0xabc", body["wallet"])
	}
	if body["pinHeight"] != float64(4231) {
		t.Errorf("pinHeight = %v, want 4231", body["pinHeight"])
	}
	if body["peers"] != float64(7) {
		t.Errorf("peers = %v, want 7", body["peers"])
	}
}

func TestStoppingProcessingLeavesTheNodeSyncing(t *testing.T) {
	restoreGate(t)
	SetProcessing(true)

	ledger := &fakeLedger{pinHeight: 100, peers: 5, syncing: false, wallet: "0xabc"}
	svc := NewService(ledger)
	routes := svc.Routes()

	_, before := do(t, routes, http.MethodGet, StatusPath, "")
	if before["state"] != string(StateProcessing) {
		t.Fatalf("state before stopping = %v, want %q", before["state"], StateProcessing)
	}

	_, stopped := do(t, routes, http.MethodPost, ProcessingPath, `{"enabled":false}`)
	if stopped["state"] != string(StateStopped) {
		t.Fatalf("state after stopping = %v, want %q", stopped["state"], StateStopped)
	}

	// The node carries on applying commit transactions while stopped, and the
	// endpoint keeps reading them: a paused node is a syncing node.
	ledger.pinHeight = 137
	_, after := do(t, routes, http.MethodGet, StatusPath, "")
	if after["state"] != string(StateStopped) {
		t.Fatalf("state = %v, want %q", after["state"], StateStopped)
	}
	if after["pinHeight"] != float64(137) {
		t.Fatalf("pinHeight while stopped = %v, want 137 - the node stopped syncing", after["pinHeight"])
	}
	if after["peers"] != float64(5) {
		t.Fatalf("peers while stopped = %v, want 5 - the node left the network", after["peers"])
	}
}

func TestStartingProcessingAgainReturnsTheNodeToProcessing(t *testing.T) {
	restoreGate(t)
	SetProcessing(true)

	svc := NewService(&fakeLedger{peers: 2})
	routes := svc.Routes()

	_, off := do(t, routes, http.MethodPost, ProcessingPath, `{"enabled":false}`)
	if off["state"] != string(StateStopped) {
		t.Fatalf("state = %v, want %q", off["state"], StateStopped)
	}
	_, on := do(t, routes, http.MethodPost, ProcessingPath, `{"enabled":true}`)
	if on["state"] != string(StateProcessing) {
		t.Fatalf("state = %v, want %q", on["state"], StateProcessing)
	}
}

func TestTheProcessingEndpointRejectsABodyWithNoEnabledField(t *testing.T) {
	restoreGate(t)
	SetProcessing(true)

	svc := NewService(&fakeLedger{})
	for _, body := range []string{`{}`, `{"Enable":false}`, `{"enabled":null}`} {
		rec, decoded := do(t, svc.Routes(), http.MethodPost, ProcessingPath, body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("POST %s with body %s = %d, want 400", ProcessingPath, body, rec.Code)
		}
		if _, ok := decoded["error"]; !ok {
			t.Errorf("POST %s with body %s gave no error message: %v", ProcessingPath, body, decoded)
		}
		if !ProcessingEnabled() {
			t.Fatalf("POST %s with body %s stopped the node", ProcessingPath, body)
		}
	}
}

func TestTheProcessingEndpointRejectsAMalformedBody(t *testing.T) {
	restoreGate(t)
	SetProcessing(true)

	svc := NewService(&fakeLedger{})
	rec, _ := do(t, svc.Routes(), http.MethodPost, ProcessingPath, `not json`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST %s with a malformed body = %d, want 400", ProcessingPath, rec.Code)
	}
	if !ProcessingEnabled() {
		t.Fatal("a malformed body stopped the node")
	}
}

func TestTheProcessingEndpointRefusesAGetRequest(t *testing.T) {
	svc := NewService(&fakeLedger{})
	rec := httptest.NewRecorder()
	svc.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, ProcessingPath, nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET %s = %d, want 405", ProcessingPath, rec.Code)
	}
}

func TestTheEarningsEndpointRendersAmountsAsDecimalStringsNotJsonNumbers(t *testing.T) {
	// 2^70 neutrinos: a JSON number would arrive as a float64 in a browser and
	// be displayed wrong, which is why every amount is a string.
	huge, ok := new(big.Int).SetString("1180591620717411303424", 10)
	if !ok {
		t.Fatal("cannot build the test amount")
	}
	at := time.Date(2026, 8, 28, 10, 30, 0, 0, time.UTC)
	ledger := &fakeLedger{
		wallet:   "0xfeed",
		lifetime: huge,
		pending:  big.NewInt(42),
		recent: []Credit{
			{Pin: 991, Site: "3f0c", Amount: huge, At: at},
			{Pin: 990, Amount: big.NewInt(7), At: at},
		},
	}
	svc := NewService(ledger)
	rec, body := do(t, svc.Routes(), http.MethodGet, EarningsPath, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", EarningsPath, rec.Code)
	}
	if body["wallet"] != "0xfeed" {
		t.Errorf("wallet = %v, want 0xfeed", body["wallet"])
	}
	if body["lifetime"] != huge.String() {
		t.Errorf("lifetime = %v (%T), want the string %q", body["lifetime"], body["lifetime"], huge.String())
	}
	if body["pending"] != "42" {
		t.Errorf("pending = %v (%T), want the string \"42\"", body["pending"], body["pending"])
	}
	recent, ok := body["recent"].([]any)
	if !ok {
		t.Fatalf("recent is %T, want an array", body["recent"])
	}
	if len(recent) != 2 {
		t.Fatalf("recent has %d entries, want 2", len(recent))
	}
	first, ok := recent[0].(map[string]any)
	if !ok {
		t.Fatalf("recent[0] is %T, want an object", recent[0])
	}
	if first["amount"] != huge.String() {
		t.Errorf("recent[0].amount = %v (%T), want the string %q", first["amount"], first["amount"], huge.String())
	}
	if first["pin"] != float64(991) {
		t.Errorf("recent[0].pin = %v, want 991", first["pin"])
	}
	if first["site"] != "3f0c" {
		t.Errorf("recent[0].site = %v, want 3f0c", first["site"])
	}
	if first["at"] != at.Format(time.RFC3339) {
		t.Errorf("recent[0].at = %v, want %s", first["at"], at.Format(time.RFC3339))
	}
	second := recent[1].(map[string]any)
	if _, present := second["site"]; present {
		t.Errorf("recent[1] carries a site field for a per-commit aggregate: %v", second)
	}

	if len(ledger.askedFor) != 1 || ledger.askedFor[0] != "0xfeed" {
		t.Errorf("EarningsFor was called with %v, want exactly [0xfeed]", ledger.askedFor)
	}
}

func TestTheEarningsEndpointReturnsAnEmptyArrayWhenNothingHasBeenEarned(t *testing.T) {
	svc := NewService(&fakeLedger{wallet: "0xfeed"})
	rec, body := do(t, svc.Routes(), http.MethodGet, EarningsPath, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", EarningsPath, rec.Code)
	}
	// A nil ledger slice, and nil totals, must not reach a wallet as null.
	if !strings.Contains(rec.Body.String(), `"recent":[]`) {
		t.Errorf("recent is not rendered as []: %s", rec.Body.String())
	}
	if body["lifetime"] != "0" || body["pending"] != "0" {
		t.Errorf("lifetime/pending = %v/%v, want \"0\"/\"0\"", body["lifetime"], body["pending"])
	}
}

func TestTheEarningsEndpointReportsALedgerFailureAsAServerError(t *testing.T) {
	svc := NewService(&fakeLedger{wallet: "0xfeed", err: errors.New("ledger store is closed")})
	rec, body := do(t, svc.Routes(), http.MethodGet, EarningsPath, "")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("GET %s with a failing ledger = %d, want 500", EarningsPath, rec.Code)
	}
	if got, _ := body["error"].(string); !strings.Contains(got, "ledger store is closed") {
		t.Errorf("error = %q, want it to carry the ledger's reason", got)
	}
}

func TestTheEndpointsAnswerJsonAndForbidCaching(t *testing.T) {
	restoreGate(t)
	SetProcessing(true)
	svc := NewService(&fakeLedger{})
	for _, path := range []string{StatusPath, EarningsPath} {
		rec, _ := do(t, svc.Routes(), http.MethodGet, path, "")
		if got := rec.Header().Get("Content-Type"); got != "application/json" {
			t.Errorf("GET %s Content-Type = %q, want application/json", path, got)
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("GET %s Cache-Control = %q, want no-store", path, got)
		}
	}
}

func TestAServiceBuiltWithNoLedgerAnswersInsteadOfPanicking(t *testing.T) {
	restoreGate(t)
	SetProcessing(true)
	svc := NewService(nil)
	if _, ok := svc.Ledger().(StubLedger); !ok {
		t.Fatalf("NewService(nil) bound a %T, want a StubLedger", svc.Ledger())
	}
	rec, body := do(t, svc.Routes(), http.MethodGet, StatusPath, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s on an unwired service = %d, want 200", StatusPath, rec.Code)
	}
	// Honest rather than optimistic: a node that cannot say it has caught up
	// reports that it has not.
	if body["state"] != string(StateSyncing) {
		t.Fatalf("state on an unwired service = %v, want %q", body["state"], StateSyncing)
	}
}
