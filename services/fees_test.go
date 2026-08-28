// SPDX-License-Identifier: Apache-2.0

package services

import (
	"math"
	"math/big"
	"strings"
	"testing"

	"github.com/Grape-Chain/Grape-Dag/config"
	"github.com/Grape-Chain/Grape-Dag/tx"
)

/*
The fee rule from docs/economics.md, checked against explicit settings.

The rule is exercised through checkPaymentFee and checkPaymentFuel rather than
through their validate- counterparts because config.GetConfig has no seam a test
can write to: a services test binary has no configuration at all, so the only
state reachable through the process globals is "fees off". The cases that matter
are the ones no deployment has chosen yet - fees starting next commit
transaction, fees already started - and they are only reachable by passing the
settings in.
*/

// minimumFee - docs/economics.md's proposed tx.minpaymentfee, in neutrinos.
const minimumFee = 1000

// feesFrom - fee settings that start charging at this commit transaction.
// Negative means never, which is the shipped default.
func feesFrom(startPin int64) config.TxConfiguration {
	return config.TxConfiguration{
		Feemode:       "fixed",
		Minpaymentfee: minimumFee,
		Feestartpin:   startPin,
	}
}

// payment - a payment offering this much fuel. Nothing in the rule reads the
// amount, the sender or the signature, so they are only here to keep the
// transaction recognisable as a payment.
func payment(limit, price int64) tx.Transaction {
	return &tx.Txv1{
		Tx_Type:    tx.PAYMENT,
		Chain_Type: tx.PRIVATE_TESTNET,
		Amount:     big.NewInt(500000).Bytes(),
		Fuel_Limit: big.NewInt(limit).Bytes(),
		Fuel_Price: big.NewInt(price).Bytes(),
	}
}

// Fees off is the shipped default, and the property that makes this change
// shippable: whatever a payment offers and whatever height the node is at, the
// fee rule refuses nothing and charges nothing. The heights include a negative
// one because a node that has not applied a commit transaction yet answers 0,
// and callers below the ledger could answer worse.
func TestNoPaymentIsRefusedForItsFeeWhileFeesAreOff(t *testing.T) {
	offered := []struct {
		name         string
		limit, price int64
	}{
		{"nothing at all", 0, 0},
		{"a limit but no price", 1, 0},
		{"a price but no limit", 0, minimumFee},
		{"the fee the network will one day want", 1, minimumFee},
		{"less than that", 1, 1},
		{"a limit fees would refuse", 21000, minimumFee},
	}
	heights := []int64{-1, 0, 1, 99, 1000000}
	for _, o := range offered {
		for _, h := range heights {
			p := payment(o.limit, o.price)
			if err := checkPaymentFee(feesFrom(-1), p, h); err != nil {
				t.Errorf("a payment offering %s at pin %d was refused with fees off: %s", o.name, h, err)
			}
			if got := feeCharged(feesFrom(-1), p, h); got != 0 {
				t.Errorf("a payment offering %s at pin %d was charged %d with fees off, want 0", o.name, h, got)
			}
		}
	}
}

// The accepted side of the rule: the minimum is a floor, not a price, so paying
// over it is allowed. It has to be, or a sender who wants their transaction
// processed sooner under a later fee market would have no way to say so.
func TestAPaymentPayingTheMinimumOrMoreIsAccepted(t *testing.T) {
	cases := []struct {
		name  string
		price int64
	}{
		{"exactly the minimum", minimumFee},
		{"a neutrino over the minimum", minimumFee + 1},
		{"a hundred times the minimum", minimumFee * 100},
		{"as much as a receipt can carry", math.MaxInt64},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := payment(1, c.price)
			if err := checkPaymentFee(feesFrom(0), p, 0); err != nil {
				t.Errorf("a payment paying %s was refused: %s", c.name, err)
			}
			if got := feeCharged(feesFrom(0), p, 0); got != c.price {
				t.Errorf("a payment paying %s was charged %d, want %d", c.name, got, c.price)
			}
		})
	}
}

// The refused side. Nought is in the table on its own account: a payment that
// pays nothing is the flood the fee exists to price.
func TestAPaymentPayingLessThanTheMinimumIsRefused(t *testing.T) {
	cases := []struct {
		name  string
		price int64
	}{
		{"nothing", 0},
		{"a single neutrino", 1},
		{"one neutrino short of the minimum", minimumFee - 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := checkPaymentFee(feesFrom(0), payment(1, c.price), 0); err == nil {
				t.Errorf("a payment paying %s was accepted", c.name)
			}
		})
	}
}

// The limit is pinned at 1 so that the fee has exactly one representation. The
// last case is the reason: 2 x 500 pays the same 1000 neutrinos as 1 x 1000, and
// is still refused, because otherwise every reader of a transaction would have
// to normalise a fee before it could agree on it.
func TestAPaymentWhoseFuelLimitIsNotOneIsRefused(t *testing.T) {
	cases := []struct {
		name         string
		limit, price int64
	}{
		{"no limit at all", 0, minimumFee},
		{"a limit of two", 2, minimumFee},
		{"a contract-sized limit", 21000, minimumFee},
		{"a limit of two paying the minimum between them", 2, minimumFee / 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := checkPaymentFee(feesFrom(0), payment(c.limit, c.price), 0); err == nil {
				t.Errorf("a payment with %s was accepted", c.name)
			}
		})
	}
}

// The boundary, stated as its own test because an off-by-one here is a fork: a
// node that starts charging a commit transaction early refuses payments the rest
// of the network accepts. Anything goes at feestartpin-1; at feestartpin the
// rule bites.
func TestTheFeeRuleBeginsAtTheActivationPinAndNotBefore(t *testing.T) {
	const startPin = 100
	cfg := feesFrom(startPin)
	underpaying := payment(0, 0)

	if err := checkPaymentFee(cfg, underpaying, startPin-1); err != nil {
		t.Errorf("a payment paying nothing at pin %d, one before fees start, was refused: %s", startPin-1, err)
	}
	if got := feeCharged(cfg, payment(1, minimumFee), startPin-1); got != 0 {
		t.Errorf("fee charged one pin before fees start is %d, want 0", got)
	}
	if err := checkPaymentFee(cfg, underpaying, startPin); err == nil {
		t.Errorf("a payment paying nothing at pin %d, the pin fees start, was accepted", startPin)
	}
	if err := checkPaymentFee(cfg, payment(1, minimumFee), startPin); err != nil {
		t.Errorf("a payment paying the minimum at pin %d, the pin fees start, was refused: %s", startPin, err)
	}
	// Fees do not lapse afterwards.
	if err := checkPaymentFee(cfg, underpaying, startPin+50000); err == nil {
		t.Errorf("a payment paying nothing at pin %d was accepted", startPin+50000)
	}
}

// A sender who is refused has to be able to act on the refusal without reading
// the source, which means the numbers have to be in it.
func TestTheRefusalNamesTheMinimumAndWhatWasOffered(t *testing.T) {
	err := checkPaymentFee(feesFrom(0), payment(1, 7), 0)
	if err == nil {
		t.Fatal("a payment paying 7 neutrinos was accepted")
	}
	for _, want := range []string{"1000", "7"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal %q does not name %s", err.Error(), want)
		}
	}

	err = checkPaymentFee(feesFrom(0), payment(3, minimumFee), 0)
	if err == nil {
		t.Fatal("a payment with a fuel limit of 3 was accepted")
	}
	if !strings.Contains(err.Error(), "3") {
		t.Errorf("refusal %q does not name the limit that was offered", err.Error())
	}
}

// The two halves of the ingress rule, either side of the switch. Before fees a
// payment must offer no fuel, because nothing would collect it and the sender
// would be told they had paid something they had not; after, the same payment is
// refused for paying nothing and the fee-carrying one is accepted.
func TestAPaymentCarryingFuelIsRefusedWhileFeesAreOffAndRequiredOnceTheyStart(t *testing.T) {
	off, on := feesFrom(-1), feesFrom(0)

	if err := checkPaymentFuel(off, payment(0, 0), 0); err != nil {
		t.Errorf("a payment offering no fuel was refused with fees off: %s", err)
	}
	if err := checkPaymentFuel(off, payment(1, minimumFee), 0); err == nil {
		t.Error("a payment carrying fuel was accepted with fees off, so its sender would be charged nothing and told otherwise")
	}
	if err := checkPaymentFuel(off, payment(0, minimumFee), 0); err == nil {
		t.Error("a payment carrying a fuel price and no limit was accepted with fees off")
	}
	if err := checkPaymentFuel(on, payment(0, 0), 0); err == nil {
		t.Error("a payment offering no fuel was accepted with fees on")
	}
	if err := checkPaymentFuel(on, payment(1, minimumFee), 0); err != nil {
		t.Errorf("a payment paying the minimum was refused with fees on: %s", err)
	}
}

// The receipt has to report what was charged rather than a constant, or it stops
// being evidence of anything the day fees start.
func TestTheFeeChargedIsTheFuelPriceOnceFeesStartAndNoughtBefore(t *testing.T) {
	cases := []struct {
		name         string
		cfg          config.TxConfiguration
		pin          int64
		limit, price int64
		want         int64
	}{
		{"fees off, no fuel offered", feesFrom(-1), 0, 0, 0, 0},
		{"fees off, fuel offered anyway", feesFrom(-1), 0, 1, minimumFee, 0},
		{"one pin before fees start", feesFrom(10), 9, 1, minimumFee, 0},
		{"the pin fees start", feesFrom(10), 10, 1, minimumFee, minimumFee},
		{"paying over the minimum", feesFrom(10), 11, 1, minimumFee * 3, minimumFee * 3},
		// Not a valid payment - the limit rule refuses it - but the receipt must
		// saturate rather than wrap, because a negative fee reads as a refund.
		{"a fee too large for the receipt", feesFrom(0), 0, 2, math.MaxInt64, math.MaxInt64},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := feeCharged(c.cfg, payment(c.limit, c.price), c.pin); got != c.want {
				t.Errorf("fee charged is %d, want %d", got, c.want)
			}
		})
	}
}

// A service-typed transaction moves value like any other - dag's balance update
// debits the amount it carries and never looks at its type - so the fee rule
// must not exempt it. If it did, relabelling a payment would be a way to send
// funds for nothing.
func TestATransactionThatIsNotLabelledAPaymentPaysTheSameFee(t *testing.T) {
	for _, txType := range []tx.TransactionType{tx.SERVICE, tx.SERVICE_GENESIS, tx.SERVICE_PIN} {
		underpaying := &tx.Txv1{
			Tx_Type:    txType,
			Chain_Type: tx.PRIVATE_TESTNET,
			Amount:     big.NewInt(500000).Bytes(),
			Fuel_Limit: big.NewInt(0).Bytes(),
			Fuel_Price: big.NewInt(0).Bytes(),
		}
		if err := checkPaymentFee(feesFrom(0), underpaying, 0); err == nil {
			t.Errorf("a %s transaction paying nothing was accepted once fees were on", txType.Name())
		}
	}
}

// Everything above judges the rule at a height that was handed to it. This is
// the path that finds the height for itself, and it runs in a test binary with
// no ledger and no configuration - which is also the state a node is in while it
// starts up, and while a wallet application is already polling it.
func TestTheFeeRuleAnswersWithNoLedgerAndNoConfiguration(t *testing.T) {
	if got := currentPinNumber(); got < 0 {
		t.Fatalf("pin number with no ledger is %d, which no fee setting expects", got)
	}
	if feeSettings().FeesActive(currentPinNumber()) {
		t.Fatal("fees are active in a process with no configuration, so an unconfigured node would charge")
	}
	if err := validatePaymentFee(payment(0, 0), currentPinNumber()); err != nil {
		t.Errorf("a payment was refused a fee by an unconfigured process: %s", err)
	}
	if err := validatePaymentFuel(payment(0, 0), currentPinNumber()); err != nil {
		t.Errorf("a payment offering no fuel was refused by an unconfigured process: %s", err)
	}
	if got := paymentFeeCharged(payment(0, 0), currentPinNumber()); got != 0 {
		t.Errorf("an unconfigured process charged %d, want 0", got)
	}
	// A nil transaction reaches the rule if a caller ever fails to check a parse
	// result. It must answer, because a panic here kills an API worker.
	if err := validatePaymentFee(nil, currentPinNumber()); err != nil {
		t.Errorf("a nil transaction was refused with fees off, want no opinion: %s", err)
	}
	if err := checkPaymentFee(feesFrom(0), nil, 0); err == nil {
		t.Error("a nil transaction was accepted with fees on")
	}
	if got := feeCharged(feesFrom(0), nil, 0); got != 0 {
		t.Errorf("a nil transaction was charged %d, want 0", got)
	}
}
