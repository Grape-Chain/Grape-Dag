package store

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"github.com/cockroachdb/pebble"
	golog "github.com/ipfs/go-log/v2"
	"google.golang.org/protobuf/proto"
)

var logger golog.EventLogger = golog.Logger("store")

/*
Pebble-backed store.

Pebble is a log-structured merge tree, which suits the shape of this workload:
commit transactions only ever arrive in order and are never rewritten, so writes
are appends and reads are a point lookup or one ordered scan at startup. It is
pure Go, so the peer stays a single static binary with no external database to
run alongside it - which matters, because a node is meant to be something a
wallet can start with one command.

Keys are prefixed by one byte and numbers are big-endian, so a prefix scan
returns commit transactions in chain order without sorting.

	m + "head"        the Head record
	p + <8-byte num>  a marshalled TxPin
*/

const (
	prefixMeta = 'm'
	prefixPin  = 'p'
)

var (
	headKey     = []byte{prefixMeta, 'h', 'e', 'a', 'd'}
	balancesKey = []byte{prefixMeta, 'b', 'a', 'l'}
)

func pinKey(number int64) []byte {
	k := make([]byte, 9)
	k[0] = prefixPin
	// Offset so that a negative number - which should not occur, but would
	// otherwise sort after everything - cannot break the ordering.
	binary.BigEndian.PutUint64(k[1:], uint64(number)+1<<63)
	return k
}

type pebbleStore struct {
	db   *pebble.DB
	path string
}

// Open - open or create a store under dir.
func Open(dir string) (Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating the store directory %s: %w", dir, err)
	}
	db, err := pebble.Open(dir, &pebble.Options{
		// The peer already logs; Pebble's own event logging is noise here.
		Logger: pebbleSilencer{},
	})
	if err != nil {
		return nil, fmt.Errorf("opening the store at %s: %w", dir, err)
	}
	logger.Infof("Ledger store open at %s", dir)
	return &pebbleStore{db: db, path: dir}, nil
}

// pebbleSilencer - Pebble insists on a logger; route it to debug.
type pebbleSilencer struct{}

func (pebbleSilencer) Infof(format string, args ...interface{})  { logger.Debugf(format, args...) }
func (pebbleSilencer) Errorf(format string, args ...interface{}) { logger.Debugf(format, args...) }
func (pebbleSilencer) Fatalf(format string, args ...interface{}) { logger.Errorf(format, args...) }

func (s *pebbleStore) Head() (*Head, error) {
	raw, closer, err := s.db.Get(headKey)
	if err == pebble.ErrNotFound {
		return nil, ErrEmpty
	}
	if err != nil {
		return nil, fmt.Errorf("reading the store head: %w", err)
	}
	defer closer.Close()
	// The value belongs to Pebble until closer runs, so decode before returning.
	head, err := unmarshalHead(raw)
	if err != nil {
		return nil, fmt.Errorf("decoding the store head: %w", err)
	}
	return head, nil
}

func (s *pebbleStore) AppendPin(pin *pb.TxPin, network int) error {
	if pin == nil {
		return nil
	}
	raw, err := pin.MarshalBinary()
	if err != nil {
		return fmt.Errorf("marshalling pin %d: %w", pin.PinNumber, err)
	}

	head, err := s.Head()
	if err != nil && err != ErrEmpty {
		return err
	}
	if head == nil {
		head = &Head{SchemaVersion: SchemaVersion, Network: network, BalancePinNumber: -1}
	}
	if pin.PinNumber == 0 {
		head.GenesisSign = pin.Sign
	}
	head.LastPinNumber = pin.PinNumber
	head.PinCount++
	head.Updated = pin.Ts.AsTime()
	headRaw, err := head.marshal()
	if err != nil {
		return fmt.Errorf("encoding the store head: %w", err)
	}

	// One batch, one sync: either both the pin and the head naming it are on
	// disk, or neither is. A head that named a pin the store did not hold would
	// make the chain unreadable at the point it matters most.
	batch := s.db.NewBatch()
	defer batch.Close()
	if err := batch.Set(pinKey(pin.PinNumber), raw, nil); err != nil {
		return fmt.Errorf("staging pin %d: %w", pin.PinNumber, err)
	}
	if err := batch.Set(headKey, headRaw, nil); err != nil {
		return fmt.Errorf("staging the store head: %w", err)
	}
	if err := batch.Commit(pebble.Sync); err != nil {
		return fmt.Errorf("committing pin %d: %w", pin.PinNumber, err)
	}
	return nil
}

func (s *pebbleStore) LoadPins(fn func(*pb.TxPin) error) error {
	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte{prefixPin},
		UpperBound: []byte{prefixPin + 1},
	})
	if err != nil {
		return fmt.Errorf("opening a store iterator: %w", err)
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		pin := &pb.TxPin{}
		if err := pin.UnmarshalBinary(iter.Value()); err != nil {
			return fmt.Errorf("decoding a stored pin: %w", err)
		}
		if err := fn(pin); err != nil {
			return err
		}
	}
	return iter.Error()
}

// Pin - one commit transaction by number.
//
// ErrEmpty rather than an error for a number the store does not hold: "not
// here" is an ordinary answer to this question. A peer can legitimately ask for
// a height before this node's history begins, and a wallet can ask about a
// transaction that was never settled.
func (s *pebbleStore) Pin(number int64) (*pb.TxPin, error) {
	raw, closer, err := s.db.Get(pinKey(number))
	if err == pebble.ErrNotFound {
		return nil, ErrEmpty
	}
	if err != nil {
		return nil, fmt.Errorf("reading pin %d: %w", number, err)
	}
	defer closer.Close()

	// Unmarshalled from a copy. Pebble's value is only valid until closer is
	// called, and proto unmarshalling can retain sub-slices of its input for
	// bytes fields - so decoding straight from it would hand the caller a
	// message whose bytes are freed underneath it.
	buf := make([]byte, len(raw))
	copy(buf, raw)

	pin := &pb.TxPin{}
	if err := pin.UnmarshalBinary(buf); err != nil {
		return nil, fmt.Errorf("decoding pin %d: %w", number, err)
	}
	return pin, nil
}

func (s *pebbleStore) PutBalances(pinNumber int64, balances map[string][]byte) error {
	raw, err := proto.Marshal(&pb.Balance{Balance: balances})
	if err != nil {
		return fmt.Errorf("marshalling the balance snapshot: %w", err)
	}
	head, err := s.Head()
	if err == ErrEmpty {
		head = &Head{SchemaVersion: SchemaVersion, BalancePinNumber: -1}
	} else if err != nil {
		return err
	}
	head.BalancePinNumber = pinNumber
	headRaw, err := head.marshal()
	if err != nil {
		return fmt.Errorf("encoding the store head: %w", err)
	}

	batch := s.db.NewBatch()
	defer batch.Close()
	if err := batch.Set(balancesKey, raw, nil); err != nil {
		return fmt.Errorf("staging the balance snapshot: %w", err)
	}
	if err := batch.Set(headKey, headRaw, nil); err != nil {
		return fmt.Errorf("staging the store head: %w", err)
	}
	// Synced like the chain itself: a head naming a snapshot that is not there
	// would send recovery down a path it cannot complete.
	if err := batch.Commit(pebble.Sync); err != nil {
		return fmt.Errorf("committing the balance snapshot: %w", err)
	}
	return nil
}

func (s *pebbleStore) Balances() (int64, map[string][]byte, error) {
	head, err := s.Head()
	if err == ErrEmpty {
		return -1, nil, nil
	}
	if err != nil {
		return -1, nil, err
	}
	if head.BalancePinNumber < 0 {
		return -1, nil, nil
	}
	raw, closer, err := s.db.Get(balancesKey)
	if err == pebble.ErrNotFound {
		return -1, nil, nil
	}
	if err != nil {
		return -1, nil, fmt.Errorf("reading the balance snapshot: %w", err)
	}
	defer closer.Close()
	balances := &pb.Balance{}
	if err := proto.Unmarshal(raw, balances); err != nil {
		return -1, nil, fmt.Errorf("decoding the balance snapshot: %w", err)
	}
	return head.BalancePinNumber, balances.Balance, nil
}

func (s *pebbleStore) Close() error {
	if s.db == nil {
		return nil
	}
	logger.Infof("Closing the ledger store at %s", filepath.Base(s.path))
	err := s.db.Close()
	s.db = nil
	return err
}
