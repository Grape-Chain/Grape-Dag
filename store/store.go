// Package store persists the ledger so that a node can restart without
// rebuilding its state from the network.
//
// What is persisted is the commit-transaction chain, and only that. A commit
// transaction already carries everything needed to reconstruct the rest: the
// sites it settled, the account balances as of that point, the executed
// contract transactions and the state diffs. Balances, the slice archive and
// the query indexes are therefore derived on startup rather than stored
// separately, which removes any chance of them disagreeing with the chain.
//
// Unconfirmed sites are deliberately not persisted. By definition no commit
// transaction has settled them, so they are re-acquired from the network, the
// same way a node that has been offline for a moment catches up.
package store

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/Grape-Chain/Grape-Dag/tx/pb"
)

// SchemaVersion - the on-disk layout. A store written by a newer version is
// refused rather than misread.
const SchemaVersion = 1

// ErrEmpty - the store has never been written to.
var ErrEmpty = errors.New("store is empty")

// Head - what the store knows about the chain it holds. Kept small: it is
// rewritten with every commit transaction.
type Head struct {
	SchemaVersion int   `json:"schemaVersion"`
	LastPinNumber int64 `json:"lastPinNumber"`
	PinCount      int64 `json:"pinCount"`
	// BalancePinNumber - the commit transaction the stored balance snapshot was
	// taken at, or -1 when there is none. Recovery seeds from the snapshot and
	// replays only the commit transactions after it.
	BalancePinNumber int64     `json:"balancePinNumber"`
	GenesisSign      []byte    `json:"genesisSign"`
	Network          int       `json:"network"`
	Updated          time.Time `json:"updated"`
}

func (h *Head) marshal() ([]byte, error) { return json.Marshal(h) }

func unmarshalHead(raw []byte) (*Head, error) {
	h := &Head{}
	if err := json.Unmarshal(raw, h); err != nil {
		return nil, err
	}
	return h, nil
}

// Store - the ledger's durable form.
type Store interface {
	// Head - what the store holds, or ErrEmpty if it holds nothing.
	Head() (*Head, error)
	// AppendPin - durably record a commit transaction. Returns only once the
	// pin and the head that names it are both on disk, so a chain read back
	// after a crash never names a pin that is not there.
	AppendPin(pin *pb.TxPin, network int) error
	// LoadPins - hand every stored commit transaction to fn, oldest first.
	LoadPins(fn func(*pb.TxPin) error) error
	// Pin - one commit transaction by number, or ErrEmpty if the store does not
	// hold it.
	//
	// A point read rather than a scan, because the callers are answering a
	// question about one pin: a peer catching up from a height the node no
	// longer keeps in memory, and a wallet asking what happened to a
	// transaction settled long ago. The node holds only a recent window of
	// commit-transaction bodies in RAM - the whole chain would grow without
	// bound - so this is what makes the older ones still answerable rather than
	// simply missing.
	Pin(number int64) (*pb.TxPin, error)
	// PutBalances - record the settled balances as of a commit transaction.
	// Only the most recent snapshot is kept: it is a starting point, not
	// history, and the chain after it is replayed.
	PutBalances(pinNumber int64, balances map[string][]byte) error
	// Balances - the stored snapshot and the commit transaction it was taken
	// at. Reports (-1, nil, nil) when there is none.
	Balances() (int64, map[string][]byte, error)
	// Close - release the store.
	Close() error
}

// NoopStore - persistence turned off. Every read reports an empty store, so a
// node behaves exactly as it did before there was one.
type NoopStore struct{}

func (NoopStore) Head() (*Head, error)                   { return nil, ErrEmpty }
func (NoopStore) AppendPin(_ *pb.TxPin, _ int) error     { return nil }
func (NoopStore) LoadPins(_ func(*pb.TxPin) error) error { return nil }
func (NoopStore) Pin(_ int64) (*pb.TxPin, error)         { return nil, ErrEmpty }
func (NoopStore) Close() error                           { return nil }

func (NoopStore) PutBalances(_ int64, _ map[string][]byte) error { return nil }
func (NoopStore) Balances() (int64, map[string][]byte, error)    { return -1, nil, nil }
