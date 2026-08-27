package store

import (
	"path/filepath"
	"testing"

	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func testPin(number int64, sign string) *pb.TxPin {
	pin := pb.NewTxPin([]byte{})
	pin.PinNumber = number
	pin.Sign = []byte(sign)
	pin.Ts = timestamppb.Now()
	pin.Balance.Balance["0xabc"] = []byte{0x01, 0x02}
	return pin
}

func openTemp(t *testing.T) (Store, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "ledger")
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("opening the store: %s", err.Error())
	}
	t.Cleanup(func() { s.Close() })
	return s, dir
}

func TestFreshStoreIsEmpty(t *testing.T) {
	s, _ := openTemp(t)
	if _, err := s.Head(); err != ErrEmpty {
		t.Fatalf("Head on a fresh store returned %v, want ErrEmpty", err)
	}
	count := 0
	if err := s.LoadPins(func(*pb.TxPin) error { count++; return nil }); err != nil {
		t.Fatalf("LoadPins on a fresh store: %s", err.Error())
	}
	if count != 0 {
		t.Fatalf("a fresh store returned %d pins", count)
	}
}

func TestPinsComeBackInChainOrder(t *testing.T) {
	s, _ := openTemp(t)
	// written out of order on purpose: the chain order must come from the key,
	// not from the order of the writes
	for _, n := range []int64{0, 3, 1, 4, 2} {
		if err := s.AppendPin(testPin(n, "sig"), 2); err != nil {
			t.Fatalf("appending pin %d: %s", n, err.Error())
		}
	}
	got := []int64{}
	if err := s.LoadPins(func(p *pb.TxPin) error { got = append(got, p.PinNumber); return nil }); err != nil {
		t.Fatalf("LoadPins: %s", err.Error())
	}
	want := []int64{0, 1, 2, 3, 4}
	if len(got) != len(want) {
		t.Fatalf("loaded %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("loaded %v, want %v", got, want)
		}
	}
}

func TestHeadTracksTheChain(t *testing.T) {
	s, _ := openTemp(t)
	if err := s.AppendPin(testPin(0, "genesis-sig"), 2); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendPin(testPin(1, "one"), 2); err != nil {
		t.Fatal(err)
	}
	head, err := s.Head()
	if err != nil {
		t.Fatalf("Head: %s", err.Error())
	}
	if head.LastPinNumber != 1 {
		t.Errorf("head names pin %d, want 1", head.LastPinNumber)
	}
	if head.PinCount != 2 {
		t.Errorf("head counts %d pins, want 2", head.PinCount)
	}
	if string(head.GenesisSign) != "genesis-sig" {
		t.Errorf("head genesis signature is %q, want the signature of pin 0", head.GenesisSign)
	}
	if head.Network != 2 {
		t.Errorf("head network is %d, want 2", head.Network)
	}
	if head.SchemaVersion != SchemaVersion {
		t.Errorf("head schema version is %d, want %d", head.SchemaVersion, SchemaVersion)
	}
}

// What is written has to survive the process that wrote it.
func TestChainSurvivesReopening(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ledger")
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for n := int64(0); n < 5; n++ {
		if err := s.AppendPin(testPin(n, "sig"), 2); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatalf("closing: %s", err.Error())
	}

	again, err := Open(dir)
	if err != nil {
		t.Fatalf("reopening: %s", err.Error())
	}
	defer again.Close()

	head, err := again.Head()
	if err != nil {
		t.Fatalf("Head after reopening: %s", err.Error())
	}
	if head.LastPinNumber != 4 || head.PinCount != 5 {
		t.Fatalf("after reopening the head names pin %d with %d pins, want 4 and 5", head.LastPinNumber, head.PinCount)
	}
	count := 0
	if err := again.LoadPins(func(p *pb.TxPin) error {
		if len(p.Balance.Balance) == 0 {
			t.Errorf("pin %d came back without its balances", p.PinNumber)
		}
		count++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if count != 5 {
		t.Fatalf("loaded %d pins after reopening, want 5", count)
	}
}

func TestLoadPinsStopsOnError(t *testing.T) {
	s, _ := openTemp(t)
	for n := int64(0); n < 4; n++ {
		if err := s.AppendPin(testPin(n, "sig"), 2); err != nil {
			t.Fatal(err)
		}
	}
	seen := 0
	err := s.LoadPins(func(p *pb.TxPin) error {
		seen++
		if p.PinNumber == 1 {
			return ErrEmpty // any error
		}
		return nil
	})
	if err == nil {
		t.Fatalf("LoadPins swallowed the callback's error")
	}
	if seen != 2 {
		t.Fatalf("LoadPins kept going after an error: saw %d pins", seen)
	}
}

func TestNoopStoreLooksEmpty(t *testing.T) {
	var s Store = NoopStore{}
	if _, err := s.Head(); err != ErrEmpty {
		t.Fatalf("NoopStore head returned %v, want ErrEmpty", err)
	}
	if err := s.AppendPin(testPin(0, "sig"), 2); err != nil {
		t.Fatalf("NoopStore append returned %v", err)
	}
	if err := s.LoadPins(func(*pb.TxPin) error { t.Fatal("NoopStore produced a pin"); return nil }); err != nil {
		t.Fatal(err)
	}
}
