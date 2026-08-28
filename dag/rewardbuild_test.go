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
