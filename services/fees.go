// SPDX-License-Identifier: Apache-2.0

package services

import (
	"fmt"
	"math"
	"math/big"

	"github.com/Grape-Chain/Grape-Dag/config"
	"github.com/Grape-Chain/Grape-Dag/dag"
	"github.com/Grape-Chain/Grape-Dag/tx"
)

/*
What a payment has to pay on the ingress path. docs/economics.md is the
specification; this file is only its enforcement at the API boundary.

The fee rides on fuel_limit and fuel_price because a payment already carries both
and the funds check already includes them, so charging a fee needs no wire
change. The limit is fixed at 1, which makes the fee exactly the price: the
alternative, letting the sender express the same fee as 2 x 500 or 4 x 250, would
mean every reader of a transaction - client, peer, commit-transaction builder -
having to agree on how to normalise it before it could agree on the number.

Two decisions worth stating because they could have gone the other way:

  - The rule is judged at a commit-transaction height, not at "now". A fee is a
    consensus value, so what a payment owes has to be a function of the ledger
    every node can see rather than of the clock on one machine.

  - Nothing here is scoped to tx.PAYMENT. A service-typed transaction moves value
    as well - dag.Node.UpdateBalanceIfValid debits whatever amount the
    transaction carries and never looks at its type - so exempting a type here
    would be a way to send funds without paying for them.

The functions come in pairs. validateX reads the process configuration and the
live ledger, and is what the transaction paths call; checkX takes the settings
and the height as arguments, and is where the rule actually lives, so it can be
tested without a loaded configuration or a running node.
*/

// oneUnitOfFuel - the only fuel limit a payment may carry once fees are active.
var oneUnitOfFuel = big.NewInt(1)

// currentPinNumber - the commit-transaction height the fee rule is judged at.
//
// Nought when the ledger is not up yet, which matches nodeLedger.PinHeight and
// errs towards charging: a network that sets tx.feestartpin to 0 wants every
// payment priced, and answering -1 here would wave one through unpriced instead.
// The wallet application starts polling the node before the DAG exists, so this
// has to answer rather than panic.
func currentPinNumber() int64 {
	p := dag.GetPin()
	if p == nil {
		return 0
	}
	return int64(p.CurrentHeight())
}

// feeSettings - the fee parameters this process runs under.
//
// A process with no configuration loaded - a test binary, or a node before the
// file is read - gets fees off rather than the zero value of the struct, whose
// Feestartpin of 0 would read as "fees from the first commit transaction". The
// rule must not invent a charge nobody configured.
func feeSettings() config.TxConfiguration {
	c := config.GetConfig()
	if c == nil {
		return config.TxConfiguration{Feestartpin: -1}
	}
	return c.Tx
}

// fuelOffered - the limit and price the transaction carries, with a missing
// field read as nought.
//
// Both getters return nil for a nil transaction, and a rule that panicked on
// that would turn one malformed request into a dead API worker.
func fuelOffered(t tx.Transaction) (*big.Int, *big.Int) {
	if t == nil {
		return new(big.Int), new(big.Int)
	}
	limit, price := t.GetFuelLimit(), t.GetFuelPrice()
	if limit == nil {
		limit = new(big.Int)
	}
	if price == nil {
		price = new(big.Int)
	}
	return limit, price
}

// validatePaymentFee - whether a payment pays the fee the network requires at
// this commit-transaction height. See checkPaymentFee for the rule.
func validatePaymentFee(t tx.Transaction, pinNumber int64) error {
	return checkPaymentFee(feeSettings(), t, pinNumber)
}

// checkPaymentFee - the fee rule: once fees are active a payment must carry
// fuel_limit exactly 1 and fuel_price of at least tx.minpaymentfee, so that
// fee == fuel_limit x fuel_price == fuel_price.
//
// Before tx.feestartpin this has no opinion at all: the fee is nought and the
// pre-fee rule governs instead (see checkPaymentFuel). Asking
// config.TxConfiguration rather than comparing the height here is deliberate -
// the boundary is decided in one place, because three copies of the comparison
// is three chances to be off by one and disagree with the rest of the network.
func checkPaymentFee(cfg config.TxConfiguration, t tx.Transaction, pinNumber int64) error {
	if !cfg.FeesActive(pinNumber) {
		return nil
	}
	minimum := cfg.MinimumPaymentFee(pinNumber)
	limit, price := fuelOffered(t)
	if limit.Cmp(oneUnitOfFuel) != 0 {
		return fmt.Errorf("payment fuel limit must be exactly 1 so that the fee is the fuel price, got %s", limit.String())
	}
	if price.Cmp(new(big.Int).SetUint64(minimum)) < 0 {
		return fmt.Errorf("payment fee is below the minimum of %d neutrinos, offered %s", minimum, price.String())
	}
	return nil
}

// validatePaymentFuel - the whole fuel rule for a payment at this height. See
// checkPaymentFuel.
func validatePaymentFuel(t tx.Transaction, pinNumber int64) error {
	return checkPaymentFuel(feeSettings(), t, pinNumber)
}

// checkPaymentFuel - the fee rule once fees are active, and the rule that came
// before it while they are not.
//
// The two halves are one function because they are the same decision seen from
// either side of tx.feestartpin. Keeping the fees-off half is what makes this
// shippable ahead of the decision to switch fees on: while nothing collects a
// payment's fee, a payment offering one would be told it had paid something it
// had not, so it is still refused exactly as it is today.
func checkPaymentFuel(cfg config.TxConfiguration, t tx.Transaction, pinNumber int64) error {
	if cfg.FeesActive(pinNumber) {
		return checkPaymentFee(cfg, t, pinNumber)
	}
	if limit, price := fuelOffered(t); limit.Sign() != 0 || price.Sign() != 0 {
		return fmt.Errorf("payment transaction must have zero fuelLimit and fuelPrice, got %s and %s correspondingly", limit.String(), price.String())
	}
	return nil
}

// paymentFeeCharged - what this payment is charged at this height, for the
// receipt the sender gets back.
//
// Nought while fees are off, which is the truth then: no commit transaction
// collects anything from a payment before tx.feestartpin. Reporting the real
// figure rather than a hard-coded zero is the point - a receipt that always says
// nought stops being evidence of anything the day fees start.
func paymentFeeCharged(t tx.Transaction, pinNumber int64) int64 {
	return feeCharged(feeSettings(), t, pinNumber)
}

// feeCharged - the fee in neutrinos, as the int64 the receipt carries.
//
// Saturating rather than wrapping on a fee too large for an int64: the receipt
// is a report, the ledger keeps the real value in a big.Int, and a negative fee
// on a receipt would be read as a refund. A fee that big is refused for want of
// funds long before it is charged.
func feeCharged(cfg config.TxConfiguration, t tx.Transaction, pinNumber int64) int64 {
	if !cfg.FeesActive(pinNumber) {
		return 0
	}
	limit, price := fuelOffered(t)
	fee := new(big.Int).Mul(limit, price)
	if !fee.IsInt64() {
		return math.MaxInt64
	}
	return fee.Int64()
}
