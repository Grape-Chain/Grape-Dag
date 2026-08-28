package dag

import (
	"math/big"
	"testing"
)

// grape - n Grape in neutrinos, at the 1e-8 unit the config uses.
func grape(n int64) *big.Int {
	return new(big.Int).Mul(big.NewInt(n), big.NewInt(100000000))
}

const testCapMilli = 5000

var testMinStake = grape(100)

func TestStakeWeightStartsAtOneTimesForEveryone(t *testing.T) {
	cases := []struct {
		name    string
		balance *big.Int
	}{
		{"nothing at all", big.NewInt(0)},
		{"an account nobody has paid", nil},
		{"half the minimum stake", grape(50)},
		{"one neutrino short of the minimum", new(big.Int).Sub(testMinStake, big.NewInt(1))},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// The point of the base weight: a user who has just installed the
			// wallet holds no Grape, does real work, and must be paid for it.
			if got := stakeWeightMilli(c.balance, testMinStake, testCapMilli); got != weightBaseMilli {
				t.Fatalf("weight is %d, want the base %d", got, weightBaseMilli)
			}
		})
	}
}

// Each ten-fold increase in stake is worth 1.0x, which is what makes 301 per
// doubling the right constant.
func TestStakeWeightAddsOneTimesPerDecade(t *testing.T) {
	cases := []struct {
		multiple int64
		want     uint32
	}{
		{1, 1000},    // at the minimum: base only
		{2, 1301},    // one doubling
		{10, 1903},   // three doublings (8 <= 10 < 16)
		{100, 2806},  // six doublings
		{1000, 3709}, // nine doublings
	}
	for _, c := range cases {
		balance := new(big.Int).Mul(testMinStake, big.NewInt(c.multiple))
		if got := stakeWeightMilli(balance, testMinStake, testCapMilli); got != c.want {
			t.Fatalf("%dx the minimum stake weighs %d, want %d", c.multiple, got, c.want)
		}
	}
}

// The cap is what stops the largest holder on the network collecting nearly
// everything.
func TestStakeWeightIsCapped(t *testing.T) {
	huge := new(big.Int).Mul(testMinStake, big.NewInt(1e15))
	if got := stakeWeightMilli(huge, testMinStake, testCapMilli); got != testCapMilli {
		t.Fatalf("an enormous balance weighs %d, want the cap %d", got, testCapMilli)
	}
	// A cap at the base means stake is worth nothing and rewards are purely
	// work-based - the setting an operator uses to switch stake weighting off.
	if got := stakeWeightMilli(huge, testMinStake, weightBaseMilli); got != weightBaseMilli {
		t.Fatalf("with the cap at the base, weight is %d, want %d", got, weightBaseMilli)
	}
	// And a cap below the base cannot make work worth less than nothing.
	if got := stakeWeightMilli(huge, testMinStake, 1); got != weightBaseMilli {
		t.Fatalf("with a cap under the base, weight is %d, want %d", got, weightBaseMilli)
	}
}

// Splitting a balance to farm the stake bonus must not pay.
//
// The property is about work x weight, not weight alone: a wallet that holds
// half the stake also does half the work, so comparing weights on their own
// says splitting is profitable when it is not. Same total work throughout,
// divided evenly between however many wallets the stake is split across - which
// is the best a splitter can do, since work is bounded by the machine.
func TestSplittingAStakeAcrossWalletsEarnsLess(t *testing.T) {
	const totalWork = 1024
	whole := new(big.Int).Mul(testMinStake, big.NewInt(1024))

	weighted := func(parts int64) uint64 {
		stake := new(big.Int).Quo(whole, big.NewInt(parts))
		w := uint64(stakeWeightMilli(stake, testMinStake, testCapMilli))
		return uint64(parts) * (uint64(totalWork) / uint64(parts)) * w
	}

	undivided := weighted(1)
	for _, parts := range []int64{2, 4, 8, 16} {
		split := weighted(parts)
		if split >= undivided {
			t.Fatalf("the same work split %d ways is worth %d against %d undivided: splitting pays",
				parts, split, undivided)
		}
	}
}

// The worked example in docs/economics.md, to the neutrino. If this changes, the
// document is wrong and somebody has to decide which of the two is right.
func TestTheWorkedExampleFromTheEconomicsDocument(t *testing.T) {
	pool := big.NewInt(5000000) // 5000 sites at the 1000-neutrino minimum fee
	work := map[string]int{"A": 2500, "B": 2000, "C": 500}
	balances := map[string]*big.Int{
		"A": grape(100),   // exactly the minimum stake
		"B": grape(10000), // a hundred times it
		"C": grape(50),    // below it, and still paid
	}

	shares, remainder := splitFeePool(pool, work,
		func(a string) *big.Int { return balances[a] }, testMinStake, testCapMilli)

	want := map[string]struct {
		weight uint32
		amount int64
	}{
		"A": {1000, 1451463},
		"B": {2806, 3258244},
		"C": {1000, 290292},
	}
	if len(shares) != len(want) {
		t.Fatalf("got %d shares, want %d", len(shares), len(want))
	}
	for _, s := range shares {
		w, ok := want[s.Processor]
		if !ok {
			t.Fatalf("unexpected processor %s", s.Processor)
		}
		if s.WeightMilli != w.weight {
			t.Fatalf("%s weight %d, want %d", s.Processor, s.WeightMilli, w.weight)
		}
		if s.Amount.Cmp(big.NewInt(w.amount)) != 0 {
			t.Fatalf("%s reward %s, want %d", s.Processor, s.Amount.String(), w.amount)
		}
	}
	if remainder.Cmp(big.NewInt(1)) != 0 {
		t.Fatalf("remainder %s, want 1", remainder.String())
	}
}

// The invariant every node can check: nothing is created and nothing vanishes.
func TestThePoolIsConservedExactly(t *testing.T) {
	seeds := []int64{1, 7, 1000, 999983, 5000000, 123456789}
	for _, pool := range seeds {
		work := map[string]int{"a": 3, "b": 11, "c": 1, "d": 97}
		balances := map[string]*big.Int{
			"a": grape(0), "b": grape(100), "c": grape(31337), "d": grape(1),
		}
		shares, remainder := splitFeePool(big.NewInt(pool), work,
			func(x string) *big.Int { return balances[x] }, testMinStake, testCapMilli)

		total := new(big.Int).Set(remainder)
		for _, s := range shares {
			if s.Amount.Sign() < 0 {
				t.Fatalf("pool %d gave %s a negative reward %s", pool, s.Processor, s.Amount)
			}
			total.Add(total, s.Amount)
		}
		if total.Cmp(big.NewInt(pool)) != 0 {
			t.Fatalf("pool %d distributed to %s in total: fees were created or lost", pool, total.String())
		}
	}
}

// Map iteration order in Go is deliberately random. A reward list in a different
// order is a different set of ledger entries, so the split has to be ordered by
// something every node agrees on.
func TestTheSplitIsInAStableOrder(t *testing.T) {
	work := map[string]int{"zeta": 5, "alpha": 5, "mu": 5, "beta": 5, "omega": 5}
	balanceOf := func(string) *big.Int { return grape(100) }

	var first []string
	for run := 0; run < 25; run++ {
		shares, _ := splitFeePool(big.NewInt(999), work, balanceOf, testMinStake, testCapMilli)
		order := make([]string, 0, len(shares))
		for _, s := range shares {
			order = append(order, s.Processor)
		}
		if first == nil {
			first = order
			continue
		}
		for i := range order {
			if order[i] != first[i] {
				t.Fatalf("run %d ordered the split %v, the first run ordered it %v", run, order, first)
			}
		}
	}
}

// Degenerate inputs must not produce a reward, and must not lose the pool
// either: whatever is not paid out is remainder.
func TestNothingToDivideLeavesThePoolAsRemainder(t *testing.T) {
	balanceOf := func(string) *big.Int { return grape(100) }
	cases := []struct {
		name string
		pool *big.Int
		work map[string]int
	}{
		{"no processors", big.NewInt(1000), map[string]int{}},
		{"no work done", big.NewInt(1000), map[string]int{"a": 0, "b": 0}},
		{"an empty pool", big.NewInt(0), map[string]int{"a": 5}},
		{"a nil pool", nil, map[string]int{"a": 5}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			shares, remainder := splitFeePool(c.pool, c.work, balanceOf, testMinStake, testCapMilli)
			for _, s := range shares {
				if s.Amount != nil && s.Amount.Sign() > 0 {
					t.Fatalf("%s was paid %s", s.Processor, s.Amount.String())
				}
			}
			// Whatever cannot be paid out is remainder. Fees that vanish are
			// supply that vanishes, so a pool with nobody to pay must still
			// come back whole.
			expect := big.NewInt(0)
			if c.pool != nil && c.pool.Sign() > 0 {
				expect = c.pool
			}
			if remainder.Cmp(expect) != 0 {
				t.Fatalf("remainder %s, want %s", remainder.String(), expect.String())
			}
		})
	}
}

// A processor that did no work is not in the split at all, however much it
// holds. Stake multiplies work; it does not substitute for it.
func TestStakeWithoutWorkEarnsNothing(t *testing.T) {
	work := map[string]int{"worker": 10, "idle-whale": 0}
	balances := map[string]*big.Int{"worker": grape(0), "idle-whale": grape(1000000)}

	shares, _ := splitFeePool(big.NewInt(1000), work,
		func(a string) *big.Int { return balances[a] }, testMinStake, testCapMilli)

	for _, s := range shares {
		if s.Processor == "idle-whale" {
			t.Fatalf("a processor that did no work was paid %s", s.Amount.String())
		}
	}
	if len(shares) != 1 || shares[0].Processor != "worker" {
		t.Fatalf("expected only the worker in the split, got %+v", shares)
	}
}
