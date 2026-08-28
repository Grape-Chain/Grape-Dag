package dag

import (
	"math/big"
	"testing"

	grape1crypto "github.com/Grape-Chain/Grape-Dag/crypto"
	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/Grape-Chain/Grape-Dag/tx/pb"
)

// feeSite - a settled site whose transaction paid limit x price, processed by
// the given account.
func feeSite(t *testing.T, n int, processor []byte, limit, price int64) *Node {
	t.Helper()
	node := tnode(n)
	payment := tx.NewTxv1(tx.PRIVATE_TESTNET)
	payment.Tx_Type = tx.PAYMENT
	payment.Sender = addr(byte(n))
	payment.Recepient = addr(byte(n + 1))
	payment.Amount = big.NewInt(100).Bytes()
	payment.Fuel_Limit = big.NewInt(limit).Bytes()
	payment.Fuel_Price = big.NewInt(price).Bytes()
	node.tx = payment
	node.processorAddress = processor
	return node
}

func testRewardSettings() rewardSettings {
	return rewardSettingsFrom(txSettings{
		Minstake:      10000000000, // 100 Grape
		Stakecapmilli: 5000,
	}, "0x00000000000000000000000000000000000000ff")
}

// While fees are off the pool is empty, so a commit transaction carries no
// reward fields at all and encodes exactly as it did before they existed. This
// is what lets the mechanism ship ahead of the decision to switch fees on.
func TestACommitTransactionCarriesNoRewardFieldsWhileFeesAreOff(t *testing.T) {
	sites := []*Node{
		feeSite(t, 1, addr(0x41), 0, 0),
		feeSite(t, 2, addr(0x42), 0, 0),
	}
	pin := pb.NewTxPin([]byte{})

	recordRewards(pin, sites, func(string) *big.Int { return big.NewInt(0) }, testRewardSettings())

	if len(pin.FeePool) != 0 || len(pin.Rewards) != 0 || len(pin.FeeRemainder) != 0 || len(pin.Coinbase) != 0 {
		t.Fatalf("a commit transaction with no fees carries reward fields: pool=%v rewards=%d remainder=%v coinbase=%v",
			pin.FeePool, len(pin.Rewards), pin.FeeRemainder, pin.Coinbase)
	}
}

// The pool is the sum of what the settled payments paid.
func TestTheFeePoolIsTheSumOfWhatThePaymentsPaid(t *testing.T) {
	sites := []*Node{
		feeSite(t, 1, addr(0x41), 1, 1000),
		feeSite(t, 2, addr(0x41), 1, 2500),
		feeSite(t, 3, addr(0x42), 1, 500),
		feeSite(t, 4, addr(0x42), 0, 0), // paid nothing, contributes nothing
	}
	if got := feePoolFor(sites, nil); got.Cmp(big.NewInt(4000)) != 0 {
		t.Fatalf("pool is %s, want 4000", got.String())
	}
}

// A contract is charged for the fuel it used, not the limit it was willing to
// pay, so the pool reads the executed result rather than the transaction.
func TestTheFeePoolChargesContractsForFuelUsedNotFuelOffered(t *testing.T) {
	contract := tx.NewTxv1(tx.PRIVATE_TESTNET)
	contract.Fuel_Limit = big.NewInt(1000000).Bytes() // willing to pay a lot
	contract.Fuel_Price = big.NewInt(3).Bytes()
	executed := []*pb.ExecutedSmcTx{{
		Tx:      contract.MarshalBinary(),
		Receipt: &pb.TxReceipt{FuelUsed: 7}, // actually used a little
	}}
	if got := feePoolFor(nil, executed); got.Cmp(big.NewInt(21)) != 0 {
		t.Fatalf("pool is %s, want 7 x 3 = 21", got.String())
	}
}

// A site whose processor cannot be established counts for nobody - it predates
// attribution, or its claim was stripped - but its fee still enters the pool.
func TestUnattributedSitesPayIntoThePoolButEarnNothing(t *testing.T) {
	attributed := feeSite(t, 1, addr(0x41), 1, 1000)
	orphan := feeSite(t, 2, nil, 1, 1000)
	sites := []*Node{attributed, orphan}

	work := workByProcessor(sites)
	if len(work) != 1 {
		t.Fatalf("work credited to %d processors, want 1", len(work))
	}
	if got := feePoolFor(sites, nil); got.Cmp(big.NewInt(2000)) != 0 {
		t.Fatalf("pool is %s, want 2000 - the orphan's fee is still collected", got.String())
	}

	pin := pb.NewTxPin([]byte{})
	recordRewards(pin, sites, func(string) *big.Int { return big.NewInt(0) }, testRewardSettings())
	if !rewardsBalance(pin) {
		t.Fatal("the split does not add up when a site is unattributed")
	}
	// The whole pool is paid to the one processor that can be identified; the
	// orphan's share is not lost, it is part of what that processor and the
	// remainder divide.
	if len(pin.Rewards) != 1 {
		t.Fatalf("%d reward records, want 1", len(pin.Rewards))
	}
}

// Nobody attributable at all: the fees still exist and must come back as
// remainder rather than vanishing. Fees that vanish are supply that vanishes.
func TestWithNoProcessorAtAllTheWholePoolBecomesRemainder(t *testing.T) {
	sites := []*Node{feeSite(t, 1, nil, 1, 1000), feeSite(t, 2, nil, 1, 500)}
	pin := pb.NewTxPin([]byte{})

	recordRewards(pin, sites, func(string) *big.Int { return big.NewInt(0) }, testRewardSettings())

	if len(pin.Rewards) != 0 {
		t.Fatalf("%d reward records with no identifiable processor", len(pin.Rewards))
	}
	if got := new(big.Int).SetBytes(pin.FeeRemainder); got.Cmp(big.NewInt(1500)) != 0 {
		t.Fatalf("remainder is %s, want the whole pool of 1500", got.String())
	}
	if !rewardsBalance(pin) {
		t.Fatal("the split does not add up")
	}
}

// The invariant every node can check from the commit transaction alone, without
// recomputing the weights: pool == sum(rewards) + remainder.
func TestARecordedSplitAlwaysBalances(t *testing.T) {
	for _, price := range []int64{1, 3, 999983, 1000000} {
		sites := []*Node{
			feeSite(t, 1, addr(0x41), 1, price),
			feeSite(t, 2, addr(0x42), 1, price),
			feeSite(t, 3, addr(0x43), 1, price),
			feeSite(t, 4, addr(0x41), 1, price),
		}
		balances := map[string]*big.Int{
			grape1crypto.BytesToAddress(addr(0x41)): grape(0),
			grape1crypto.BytesToAddress(addr(0x42)): grape(100),
			grape1crypto.BytesToAddress(addr(0x43)): grape(50000),
		}
		pin := pb.NewTxPin([]byte{})
		recordRewards(pin, sites, func(a string) *big.Int { return balances[a] }, testRewardSettings())

		if !rewardsBalance(pin) {
			t.Fatalf("at fee %d the split does not add up", price)
		}
		if len(pin.Rewards) != 3 {
			t.Fatalf("at fee %d there are %d reward records, want 3", price, len(pin.Rewards))
		}
	}
}

// A commit transaction naming an account twice must not be able to hide a
// payment from a node checking the total.
func TestARepeatedAccountInTheSplitIsSummedNotReplaced(t *testing.T) {
	pin := pb.NewTxPin([]byte{})
	pin.Rewards = []*pb.RewardRecord{
		{Processor: []byte("0xaa"), Amount: big.NewInt(70).Bytes()},
		{Processor: []byte("0xaa"), Amount: big.NewInt(30).Bytes()},
	}
	paid := rewardsPaid(pin)
	if got := paid["0xaa"]; got == nil || got.Cmp(big.NewInt(100)) != 0 {
		t.Fatalf("a repeated account reports %v, want the sum 100", got)
	}
}

// The balance check has to reject a commit transaction that creates money, or it
// is not a check.
func TestASplitThatDoesNotAddUpIsRejected(t *testing.T) {
	pin := pb.NewTxPin([]byte{})
	pin.FeePool = big.NewInt(1000).Bytes()
	pin.FeeRemainder = big.NewInt(0).Bytes()
	pin.Rewards = []*pb.RewardRecord{
		{Processor: []byte("0xaa"), Amount: big.NewInt(1001).Bytes()}, // one too many
	}
	if rewardsBalance(pin) {
		t.Fatal("a split paying out more than the pool was accepted")
	}
	pin.Rewards[0].Amount = big.NewInt(999).Bytes() // one too few
	if rewardsBalance(pin) {
		t.Fatal("a split paying out less than the pool, with no remainder, was accepted")
	}
}

// Earnings are read from the commit-transaction chain, which is the only
// authority: a running total would have to be rebuilt on recovery and kept in
// step, and would be a second place for the number to be wrong.
func TestEarningsAreReadFromTheCommitTransactionChain(t *testing.T) {
	recoveryFixture(t, t.TempDir())
	const alice = "0x00000000000000000000000000000000000000a1"
	const bob = "0x00000000000000000000000000000000000000b2"

	for i, amounts := range []map[string]int64{
		{alice: 100, bob: 50},
		{alice: 30},
		{bob: 7},
	} {
		pin := pb.NewTxPin([]byte{})
		pin.PinNumber = int64(i)
		total := int64(0)
		for account, amount := range amounts {
			pin.Rewards = append(pin.Rewards, &pb.RewardRecord{
				Processor: []byte(account),
				Amount:    big.NewInt(amount).Bytes(),
			})
			total += amount
		}
		pin.FeePool = big.NewInt(total).Bytes()
		_pins_.unsafe_appendPin(pin)
	}

	lifetime, pending, credits := EarningsFor(alice, 50)
	if lifetime.Cmp(big.NewInt(130)) != 0 {
		t.Fatalf("alice's lifetime earnings are %s, want 130", lifetime.String())
	}
	if pending.Sign() != 0 {
		t.Fatalf("pending is %s; a reward exists only once a commit transaction carries it", pending)
	}
	if len(credits) != 2 {
		t.Fatalf("alice has %d credits, want 2", len(credits))
	}
	// Newest first, so a wallet application shows the most recent payment at the
	// top without having to sort.
	if credits[0].Pin <= credits[1].Pin {
		t.Fatalf("credits are ordered %d then %d; want newest first", credits[0].Pin, credits[1].Pin)
	}

	if bobTotal, _, bobCredits := EarningsFor(bob, 50); bobTotal.Cmp(big.NewInt(57)) != 0 || len(bobCredits) != 2 {
		t.Fatalf("bob earned %s over %d credits, want 57 over 2", bobTotal.String(), len(bobCredits))
	}
	// An account the chain never paid earns nothing, and gets a list rather
	// than nil.
	if total, _, none := EarningsFor("0xdead", 50); total.Sign() != 0 || none == nil || len(none) != 0 {
		t.Fatalf("an unpaid account reports %s over %v", total.String(), none)
	}
}

// The credit list is capped so an endpoint cannot be asked to render the whole
// chain, but the lifetime total still covers all of it.
func TestTheEarningsCreditListIsCappedButTheTotalIsNot(t *testing.T) {
	recoveryFixture(t, t.TempDir())
	const alice = "0x00000000000000000000000000000000000000a1"
	for i := 0; i < 20; i++ {
		pin := pb.NewTxPin([]byte{})
		pin.PinNumber = int64(i)
		pin.FeePool = big.NewInt(10).Bytes()
		pin.Rewards = []*pb.RewardRecord{{Processor: []byte(alice), Amount: big.NewInt(10).Bytes()}}
		_pins_.unsafe_appendPin(pin)
	}

	lifetime, _, credits := EarningsFor(alice, 5)
	if lifetime.Cmp(big.NewInt(200)) != 0 {
		t.Fatalf("lifetime is %s, want 200 - the cap must not shorten the total", lifetime.String())
	}
	if len(credits) != 5 {
		t.Fatalf("%d credits returned for a limit of 5", len(credits))
	}
	if credits[0].Pin != 19 {
		t.Fatalf("the newest credit is pin %d, want 19", credits[0].Pin)
	}
}

// The fee has to be the fuel price, which means the limit has to be exactly
// one. Ingress refuses anything else; this is the recomputed-rather-than-
// trusted path, so a site that slipped past carrying a limit of two must not
// contribute twice its price to the pool that gets paid out.
func TestASiteWhoseFuelLimitIsNotOnePaysNothingIntoThePool(t *testing.T) {
	cases := []struct {
		name  string
		limit int64
		price int64
		want  int64
	}{
		{"the only valid shape", 1, 1000, 1000},
		{"a limit of two", 2, 1000, 0},
		{"a limit of 21000, as Ethereum tooling sends", 21000, 1, 0},
		{"no fuel at all", 0, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			site := feeSite(t, 1, addr(0x41), c.limit, c.price)
			if got := paymentFee(site); got.Cmp(big.NewInt(c.want)) != 0 {
				t.Fatalf("fee is %s, want %d", got.String(), c.want)
			}
		})
	}
}

// Escrow: the sender pays the amount and the fee, the recipient receives the
// amount. Debiting only the amount would pay rewards out of money nobody had
// paid in, so the supply would grow by the fee on every payment.
func TestTheFeeIsNoughtWhileFeesAreOffSoEscrowIsUnchanged(t *testing.T) {
	recoveryFixture(t, t.TempDir())
	// The fixture's config has no fee settings, so fees are off.
	site := feeSite(t, 1, addr(0x41), 1, 1000)
	if got := settledFee(site); got.Sign() != 0 {
		t.Fatalf("a fee of %s is charged while fees are off", got.String())
	}
}

// Once fees are active the settled fee is what the site's transaction paid, and
// it is the same figure the commit transaction divides out - one function, so
// the balances and the rewards cannot disagree.
func TestTheSettledFeeMatchesWhatTheCommitTransactionDividesOut(t *testing.T) {
	recoveryFixture(t, t.TempDir())
	prev := txConfig
	txConfig.Feestartpin = 0
	txConfig.Minpaymentfee = 1000
	t.Cleanup(func() { txConfig = prev })

	site := feeSite(t, 1, addr(0x41), 1, 2500)
	charged := settledFee(site)
	if charged.Cmp(big.NewInt(2500)) != 0 {
		t.Fatalf("the sender is charged %s, want 2500", charged.String())
	}
	pooled := feePoolFor([]*Node{site}, nil)
	if pooled.Cmp(charged) != 0 {
		t.Fatalf("the pool collects %s but the sender was charged %s: the two must agree",
			pooled.String(), charged.String())
	}
}

// The hazard the feesOff constant exists for: TxConfiguration's zero value has
// Feestartpin 0, which means "charge from the genesis commit transaction". A
// node built without going through configuration would therefore charge fees
// that no payment builder in the tree pays, and refuse every payment on the
// network - the chain would stop, on a default.
func TestAnUnconfiguredNodeDoesNotChargeFees(t *testing.T) {
	prev := txConfig
	t.Cleanup(func() { txConfig = prev })

	// Exactly what dag.Init falls back to when there is no configuration.
	txConfig = configTxFallback()
	if txConfig.FeesActive(0) {
		t.Fatal("a node with no configuration charges fees from the genesis commit transaction")
	}
	for _, h := range []int64{0, 1, 1000, 1 << 40} {
		if txConfig.FeesActive(h) {
			t.Fatalf("a node with no configuration charges fees at pin %d", h)
		}
	}
}
