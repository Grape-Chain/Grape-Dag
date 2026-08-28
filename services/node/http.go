package node

import (
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"time"
)

// Route paths. Exported so that whatever mounts them cannot drift from what the
// bundled page fetches.
const (
	StatusPath     = "/node/status"
	EarningsPath   = "/node/earnings"
	ProcessingPath = "/node/processing"
	// WebPath - the bundled reference page. Trailing slash: it is a subtree.
	WebPath = "/node/"
)

// maxRequestBody - the processing endpoint takes a two-field object. Anything
// larger is not a request this node needs to read into memory.
const maxRequestBody = 4 << 10

// Service - the node endpoints bound to one ledger.
type Service struct {
	ledger Ledger
}

// NewService - endpoints reading l. A nil ledger becomes a StubLedger, so a
// caller that mounts the routes before the real implementation exists gets empty
// answers rather than a panic on the first request.
func NewService(l Ledger) *Service {
	if l == nil {
		l = StubLedger{}
	}
	return &Service{ledger: l}
}

// Ledger - the ledger these endpoints read.
func (s *Service) Ledger() Ledger {
	return s.ledger
}

type statusResponse struct {
	State           State   `json:"state"`
	Wallet          string  `json:"wallet"`
	PinHeight       int64   `json:"pinHeight"`
	Peers           int     `json:"peers"`
	TpsContribution float64 `json:"tpsContribution"`
}

// creditView - a Credit on the wire.
//
// Amounts are decimal strings, not JSON numbers, because a JSON number is a
// float64 in every browser and in most JSON libraries: a fee total large enough
// to matter would arrive rounded, and a wallet must not display a rounded
// balance.
type creditView struct {
	Pin    int64     `json:"pin"`
	Site   string    `json:"site,omitempty"`
	Amount string    `json:"amount"`
	At     time.Time `json:"at"`
}

type earningsResponse struct {
	Wallet   string       `json:"wallet"`
	Lifetime string       `json:"lifetime"`
	Pending  string       `json:"pending"`
	Recent   []creditView `json:"recent"`
}

type processingRequest struct {
	// A pointer so that a body with no "enabled" field is rejected rather than
	// read as false. Stopping a node by accident is the failure this prevents.
	Enabled *bool `json:"enabled"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// StatusHandler - GET /node/status.
func (s *Service) StatusHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		writeJSON(w, http.StatusOK, s.status())
	})
}

// EarningsHandler - GET /node/earnings.
func (s *Service) EarningsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		wallet := s.ledger.WalletAddress()
		lifetime, pending, recent, err := s.ledger.EarningsFor(wallet)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, earningsResponse{
			Wallet:   wallet,
			Lifetime: amount(lifetime),
			Pending:  amount(pending),
			Recent:   creditViews(recent),
		})
	})
}

// ProcessingHandler - POST /node/processing, body {"enabled": bool}.
//
// It answers with the whole status rather than an acknowledgement so that a UI
// showing the toggle can update from the same round trip that flipped it.
func (s *Service) ProcessingHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		var req processingRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody))
		if err := dec.Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "cannot read the request body as JSON: " + err.Error()})
			return
		}
		if req.Enabled == nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "field \"enabled\" is required and must be true or false"})
			return
		}
		SetProcessing(*req.Enabled)
		writeJSON(w, http.StatusOK, s.status())
	})
}

// Routes - the three endpoints and the bundled page on one mux, at their
// absolute paths.
//
// Absolute rather than prefix-relative so that mounting is a single line and
// cannot silently rewrite the paths the page fetches: mount it on a router that
// passes the request path through unchanged, for instance chi's
// mux.Handle("/node/*", svc.Routes()).
func (s *Service) Routes() http.Handler {
	mux := http.NewServeMux()
	// Registered without a method in the pattern, because each handler checks
	// the method itself: they are meant to be mounted individually too, and a
	// handler that is only correct behind a particular mux is a trap.
	mux.Handle(StatusPath, s.StatusHandler())
	mux.Handle(EarningsPath, s.EarningsHandler())
	mux.Handle(ProcessingPath, s.ProcessingHandler())
	mux.Handle(WebPath, WebHandler())
	return mux
}

// requireMethod - answer 405 unless the request uses method, and say which
// method it should have used.
func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	w.Header().Set("Allow", method)
	writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "this endpoint takes " + method})
	return false
}

func (s *Service) status() statusResponse {
	return statusResponse{
		State:           DeriveState(ProcessingEnabled(), s.ledger.Syncing(), s.ledger.PeerCount()),
		Wallet:          s.ledger.WalletAddress(),
		PinHeight:       s.ledger.PinHeight(),
		Peers:           s.ledger.PeerCount(),
		TpsContribution: TpsContribution(),
	}
}

func creditViews(credits []Credit) []creditView {
	// Never nil: the endpoint has to render [] rather than null, or every caller
	// needs a null check before it can iterate.
	out := make([]creditView, 0, len(credits))
	for _, c := range credits {
		out = append(out, creditView{
			Pin:    c.Pin,
			Site:   c.Site,
			Amount: amount(c.Amount),
			At:     c.At,
		})
	}
	return out
}

// amount - a fee total as a decimal string, treating an absent value as zero. A
// ledger that has nothing to report should not make a wallet parse "null".
func amount(v *big.Int) string {
	if v == nil {
		return "0"
	}
	return v.String()
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	// These endpoints report a value that changes every second and drive a
	// control. A cached answer would show somebody a toggle in the wrong
	// position.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

// ErrNotWired - what a Ledger implementation returns for a figure the node
// cannot produce yet. Handed out here so a partial implementation has one
// recognisable error to use rather than inventing its own.
var ErrNotWired = errors.New("not available on this node yet")
