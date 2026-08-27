package dag

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/Grape-Chain/Grape-Dag/tx/pb"
)

func openingPin(number int64, wallet string, amount string) *pb.TxPin {
	pin := pb.NewTxPin(nil)
	pin.PinNumber = number
	v, _ := new(big.Int).SetString(amount, 10)
	pin.Balance.Balance[wallet] = v.Bytes()
	return pin
}

func paymentPin(number int64, sites ...*Node) *pb.TxPin {
	pin := pb.NewTxPin([]byte{byte(number)})
	pin.PinNumber = number
	for _, s := range sites {
		pin.Sites = append(pin.Sites, &pb.SiteID{Id: s.id.id[:]})
		pin.Nodes = append(pin.Nodes, s.ToPbNode())
	}
	return pin
}

// Money must not appear or vanish: every payment moves value between accounts.
func TestSettledLedgerConservesValue(t *testing.T) {
	l := newSettledLedger()
	l.applyPin(openingPin(0, addrStr(0xaa), "1000"))
	if got := l.total().String(); got != "1000" {
		t.Fatalf("opening total is %s, want 1000", got)
	}

	l.applyPin(paymentPin(1, paymentSite(1, 0xaa, 0xbb, 250), paymentSite(2, 0xaa, 0xcc, 100)))
	l.applyPin(paymentPin(2, paymentSite(3, 0xbb, 0xcc, 50)))

	if got := l.total().String(); got != "1000" {
		t.Fatalf("total after payments is %s, want 1000", got)
	}
	for wallet, want := range map[string]string{
		addrStr(0xaa): "650", // 1000 - 250 - 100
		addrStr(0xbb): "200", // 250 - 50
		addrStr(0xcc): "150", // 100 + 50
	} {
		got, ok := l.get(wallet)
		if !ok {
			t.Errorf("no balance for %s", wallet)
			continue
		}
		if got.String() != want {
			t.Errorf("balance for %s is %s, want %s", wallet, got.String(), want)
		}
	}
}

// Applying the same commit transaction twice must not move money twice - a
// replay after a snapshot would otherwise double-count.
func TestSettledLedgerIgnoresPinsAlreadyApplied(t *testing.T) {
	l := newSettledLedger()
	l.applyPin(openingPin(0, addrStr(0xaa), "500"))
	pin := paymentPin(1, paymentSite(1, 0xaa, 0xbb, 200))

	l.applyPin(pin)
	l.applyPin(pin)
	l.applyPin(pin)

	if got, _ := l.get(addrStr(0xbb)); got.String() != "200" {
		t.Fatalf("recipient holds %s after three applications of one pin, want 200", got.String())
	}
	if got := l.total().String(); got != "500" {
		t.Fatalf("total is %s, want 500", got)
	}
}

// A snapshot is a starting point: the chain after it is replayed onto it, and
// the chain before it is not applied again.
func TestSettledLedgerResumesFromASnapshot(t *testing.T) {
	l := newSettledLedger()
	l.applyPin(openingPin(0, addrStr(0xaa), "1000"))
	l.applyPin(paymentPin(1, paymentSite(1, 0xaa, 0xbb, 100)))
	at, snapshot := l.snapshot()
	if at != 1 {
		t.Fatalf("snapshot taken at pin %d, want 1", at)
	}

	// a fresh ledger seeded from that snapshot, then handed the whole chain
	again := newSettledLedger()
	again.seed(at, snapshot)
	again.applyPin(openingPin(0, addrStr(0xaa), "1000"))           // already covered
	again.applyPin(paymentPin(1, paymentSite(1, 0xaa, 0xbb, 100))) // already covered
	again.applyPin(paymentPin(2, paymentSite(2, 0xaa, 0xbb, 50)))  // new

	for wallet, want := range map[string]string{addrStr(0xaa): "850", addrStr(0xbb): "150"} {
		got, ok := again.get(wallet)
		if !ok {
			t.Errorf("no balance for %s after resuming", wallet)
			continue
		}
		if got.String() != want {
			t.Errorf("balance for %s is %s after resuming, want %s", wallet, got.String(), want)
		}
	}
	if got := again.total().String(); got != "1000" {
		t.Fatalf("total after resuming is %s, want 1000", got)
	}
}

// Nothing read off the chain may crash the node, however malformed.
func TestSettledLedgerToleratesUnusableSites(t *testing.T) {
	l := newSettledLedger()
	l.applyPin(openingPin(0, addrStr(0xaa), "10"))

	// a site whose transaction has no addresses at all
	bad := siteWithTx(9)
	l.applyPin(paymentPin(1, bad))

	// a site with a short sender
	short := tnode(10)
	short.tx = paymentSite(11, 0xaa, 0xbb, 1).tx
	pin := paymentPin(2, short)
	pin.Nodes[0].Tx.Sender = []byte{0x01}
	l.applyPin(pin)

	if got := l.total().String(); got != "10" {
		t.Fatalf("total is %s after unusable sites, want 10", got)
	}
}

// Contract execution states balances outright rather than as transfers.
func TestSettledLedgerTakesContractDiffsAsGiven(t *testing.T) {
	l := newSettledLedger()
	l.applyPin(openingPin(0, addrStr(0xaa), "100"))

	pin := paymentPin(1)
	pin.Diffs = append(pin.Diffs, &pb.Diff{
		MappingOrAccount: &pb.Diff_AccountDiff{AccountDiff: &pb.AccountDiff{
			NewValue: &pb.AccountData{Address: addr(0xaa), Balance: big.NewInt(4242).Bytes()},
		}},
	})
	l.applyPin(pin)

	got, _ := l.get(addrStr(0xaa))
	if got.String() != "4242" {
		t.Fatalf("balance after a contract diff is %s, want 4242", got.String())
	}
}

func TestAddressOfRejectsWrongLengths(t *testing.T) {
	if _, ok := addressOf(nil); ok {
		t.Errorf("a nil address was accepted")
	}
	if _, ok := addressOf([]byte{1, 2, 3}); ok {
		t.Errorf("a short address was accepted")
	}
	if _, ok := addressOf(make([]byte, 21)); ok {
		t.Errorf("a long address was accepted")
	}
	got, ok := addressOf(addr(0x7f))
	if !ok {
		t.Fatalf("a 20-byte address was rejected")
	}
	if got != "0x"+hex.EncodeToString(addr(0x7f)) {
		t.Fatalf("address rendered as %s", got)
	}
}
