package dag

import (
	"math/big"
	"sort"
)

/*
Who gets paid what, out of one commit transaction's fees.

Every node recomputes this from the commit transaction's own contents and must
reach the same number to the last neutrino, because a reward is a ledger entry:
two nodes that disagree about a balance have forked. That single requirement
decides most of what this file looks like.

There is no floating point anywhere in it. A float64 result depends on the order
of operations and, across architectures, on the hardware; a consensus value
cannot. Stake weight is therefore an integer in permille, and the share is a
big.Int division with an explicit floor rather than a rounded product.

The design and the parameters are in docs/economics.md, which is the document to
read before changing any of this.
*/

// weightBaseMilli - what a processor's work is worth before any stake bonus:
// 1.0x, expressed in permille.
//
// Not zero. An earlier draft gave nought weight below the minimum stake, as a
// sybil gate, which would have meant that somebody who had just installed the
// wallet could run a node, do real work and be paid nothing. Rewards are
// proportional to work, and work is bounded by what a machine can process, so
// creating a hundred wallets does not create a hundred machines: the work term
// already carries the sybil resistance the gate was there for.
const weightBaseMilli = 1000

// weightPerDecade - permille added per ten-fold increase in stake.
//
// log10(2) x 1000, because the step below counts doublings: 301 per doubling
// makes each decade worth 1.0x, which is a rate a person can hold in their head.
const weightPerDecade = 301

// stakeWeightMilli - a processor's reward weight in permille.
//
//	balance < minStake          -> weightBaseMilli
//	otherwise                   -> base + 301 x floor(log2(balance/minStake)),
//	                               capped at capMilli
//
// The logarithm is a bit length, so this is shifts and adds and cannot disagree
// with itself between two builds.
//
// The curve is concave, which is what makes splitting a balance across wallets
// to farm the bonus unprofitable - but the property is about work x weight, not
// about weight alone. Two wallets each earn the base, yet each also does half
// the work, so the base cancels; what is left is the bonus per unit of work,
// and that falls. A stake of 1024x the minimum doing 1024 units of work is
// worth 4,106,240; the same work split two ways is worth 3,798,016, and eight
// ways 3,181,568. A whale's best move is one wallet, which is what we want.
func stakeWeightMilli(balance, minStake *big.Int, capMilli uint32) uint32 {
	weight := uint32(weightBaseMilli)
	if capMilli < weightBaseMilli {
		// A cap below the base would mean work counted for less than nothing.
		// Treat it as "stake is worth nothing", which is what an operator
		// setting the cap to its floor is asking for.
		capMilli = weightBaseMilli
	}
	if balance == nil || minStake == nil || minStake.Sign() <= 0 || balance.Cmp(minStake) < 0 {
		return weight
	}
	ratio := new(big.Int).Quo(balance, minStake)
	if ratio.Sign() <= 0 {
		return weight
	}
	// floor(log2(ratio)): 1 -> 0, 2..3 -> 1, 4..7 -> 2, and so on.
	doublings := uint32(ratio.BitLen() - 1)
	bonus := uint64(weightPerDecade) * uint64(doublings)
	total := uint64(weight) + bonus
	if total > uint64(capMilli) {
		return capMilli
	}
	return uint32(total)
}

// RewardShare - one processor's cut of a commit transaction's fees.
type RewardShare struct {
	// Processor - the account the fee is paid to.
	Processor string
	// Work - sites in this commit transaction attributed to this processor.
	Work int
	// WeightMilli - the stake weight applied, in permille. Recorded so that a
	// node checking the split does not have to re-derive it from a balance that
	// may since have moved.
	WeightMilli uint32
	// Amount - the reward, in the ledger's smallest unit.
	Amount *big.Int
}

// splitFeePool - divide pool between processors by work and stake weight.
//
// Returns the shares in a stable order and the remainder that floor division
// left over. The caller credits the remainder to the coinbase account, which
// makes pool == sum(shares) + remainder exact by construction and checkable on
// every node.
//
// balanceOf reports the settled balance of an account; a nil return is treated
// as nought, so an account nobody has ever paid gets the base weight rather
// than an error.
func splitFeePool(
	pool *big.Int,
	work map[string]int,
	balanceOf func(account string) *big.Int,
	minStake *big.Int,
	capMilli uint32,
) ([]RewardShare, *big.Int) {
	if pool == nil || pool.Sign() <= 0 {
		return nil, big.NewInt(0)
	}

	// Sorted, so that every node forms the same list from the same commit
	// transaction. Map iteration order in Go is deliberately random, and a
	// reward list in a different order is a different set of ledger entries.
	accounts := make([]string, 0, len(work))
	for account := range work {
		if work[account] > 0 {
			accounts = append(accounts, account)
		}
	}
	sort.Strings(accounts)
	if len(accounts) == 0 {
		// A commit transaction can settle sites whose processor is unknown - an
		// older node built them, before sites carried an identity - so there is
		// fee income and nobody attributable to pay it to. The pool comes back
		// whole as remainder rather than being dropped: fees that vanish are
		// supply that vanishes, and every node checks
		// pool == sum(shares) + remainder.
		return nil, new(big.Int).Set(pool)
	}

	shares := make([]RewardShare, 0, len(accounts))
	denominator := new(big.Int)
	for _, account := range accounts {
		var balance *big.Int
		if balanceOf != nil {
			balance = balanceOf(account)
		}
		weight := stakeWeightMilli(balance, minStake, capMilli)
		contribution := new(big.Int).Mul(big.NewInt(int64(work[account])), big.NewInt(int64(weight)))
		denominator.Add(denominator, contribution)
		shares = append(shares, RewardShare{
			Processor:   account,
			Work:        work[account],
			WeightMilli: weight,
		})
	}
	if denominator.Sign() <= 0 {
		// Nobody has any weighted work, so there is nothing to divide by. The
		// whole pool is remainder.
		return nil, new(big.Int).Set(pool)
	}

	distributed := new(big.Int)
	for i := range shares {
		numerator := new(big.Int).Mul(pool, big.NewInt(int64(shares[i].Work)))
		numerator.Mul(numerator, big.NewInt(int64(shares[i].WeightMilli)))
		// Quo, not Div: floor for the non-negative values this only ever sees,
		// and no rounding to argue about.
		amount := numerator.Quo(numerator, denominator)
		shares[i].Amount = amount
		distributed.Add(distributed, amount)
	}

	remainder := new(big.Int).Sub(pool, distributed)
	if remainder.Sign() < 0 {
		// Cannot happen: the shares are floors of fractions summing to one. Kept
		// because "cannot happen" in a consensus calculation is worth a guard
		// rather than a comment.
		remainder = big.NewInt(0)
	}
	return shares, remainder
}
