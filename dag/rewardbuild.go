package dag

import (
	"math/big"
	"time"

	grape1crypto "github.com/Grape-Chain/Grape-Dag/crypto"
	"github.com/Grape-Chain/Grape-Dag/tx/pb"
)

/*
Turning a commit transaction's settled sites into a reward split.

This is the bridge between the sites - which carry who processed them and what
their transactions paid - and the arithmetic in dag/rewards.go, which knows
nothing about either. Kept separate from the pin builder because the builder is
already long and because the interesting property here is testability: the
functions below take sites and a balance lookup and return numbers, so the whole
split can be checked without a ledger.

While fees are off, which is the default, the pool is nought, the split is
empty, and the commit transaction encodes exactly as it did before any of these
fields existed. See docs/economics.md.
*/

// RewardCredit - one payment the commit-transaction chain made to an account.
type RewardCredit struct {
	Pin    int64
	Amount *big.Int
	At     time.Time
}

// paymentFee - what one site's transaction paid, in the ledger's smallest unit.
//
// fuel_limit x fuel_price, the fields a payment already carries. Zero for a site
// with no transaction, and zero when either field is unset, so an old
// transaction that predates fees contributes nothing rather than being refused
// here: refusing belongs on the ingress path, where the sender can be told.
func paymentFee(n *Node) *big.Int {
	if n == nil || n.tx == nil {
		return big.NewInt(0)
	}
	limit, price := n.tx.GetFuelLimit(), n.tx.GetFuelPrice()
	if limit == nil || price == nil || limit.Sign() <= 0 || price.Sign() <= 0 {
		return big.NewInt(0)
	}
	return new(big.Int).Mul(limit, price)
}

// feePoolFor - the fees a commit transaction collects from the sites it settles,
// plus what its smart-contract transactions burned.
//
// The smart-contract half is read from the executed results rather than from the
// transactions, because what a contract is charged is the fuel it actually used,
// not the limit it was willing to pay.
func feePoolFor(sites []*Node, executed []*pb.ExecutedSmcTx) *big.Int {
	pool := new(big.Int)
	for _, s := range sites {
		pool.Add(pool, paymentFee(s))
	}
	for _, e := range executed {
		if e == nil || e.Receipt == nil || e.Tx == nil {
			continue
		}
		used := new(big.Int).SetUint64(uint64(e.Receipt.GetFuelUsed()))
		price := new(big.Int).SetBytes(e.Tx.GetFuelPrice())
		pool.Add(pool, used.Mul(used, price))
	}
	return pool
}

// workByProcessor - how many of these sites each processor is credited with.
//
// A site whose processor cannot be established counts for nobody: it was built
// before attribution existed, or its claim did not verify and was stripped. Its
// fee still enters the pool, and comes back out as remainder if there is nobody
// at all to pay - fees that vanish are supply that vanishes.
func workByProcessor(sites []*Node) map[string]int {
	work := map[string]int{}
	for _, s := range sites {
		if s == nil || len(s.processorAddress) == 0 {
			continue
		}
		work[grape1crypto.BytesToAddress(s.processorAddress)]++
	}
	return work
}

// recordRewards - compute the split for these sites and write it into the pin.
//
// A no-op while the pool is empty, which is the case whenever fees are off: the
// fields stay absent and the commit transaction is byte-identical to one built
// before rewards existed. That is what lets this ship ahead of the decision to
// switch fees on.
func recordRewards(pin *pb.TxPin, sites []*Node, balanceOf func(string) *big.Int, cfg rewardSettings) {
	if pin == nil {
		return
	}
	pool := feePoolFor(sites, pin.SmcTxs)
	if pool.Sign() <= 0 {
		return
	}

	shares, remainder := splitFeePool(pool, workByProcessor(sites), balanceOf, cfg.minStake, cfg.capMilli)

	pin.FeePool = pool.Bytes()
	pin.Coinbase = []byte(cfg.coinbase)
	pin.FeeRemainder = remainder.Bytes()
	pin.Rewards = make([]*pb.RewardRecord, 0, len(shares))
	for _, s := range shares {
		pin.Rewards = append(pin.Rewards, &pb.RewardRecord{
			Processor:   []byte(s.Processor),
			Work:        uint32(s.Work),
			WeightMilli: s.WeightMilli,
			Amount:      s.Amount.Bytes(),
		})
	}
}

// rewardSettings - the parameters the split needs, gathered so the builder does
// not have to reach into configuration from inside a locked section.
type rewardSettings struct {
	minStake *big.Int
	capMilli uint32
	coinbase string
}

// rewardSettingsFrom - the settings as configured.
func rewardSettingsFrom(tx txSettings, coinbase string) rewardSettings {
	return rewardSettings{
		minStake: new(big.Int).SetUint64(tx.Minstake),
		capMilli: tx.Stakecapmilli,
		coinbase: coinbase,
	}
}

// txSettings - the part of the transaction configuration this file reads. An
// interface-free struct copy, so a test can set it without a config file.
type txSettings struct {
	Minstake      uint64
	Stakecapmilli uint32
}

// rewardsPaid - the split a commit transaction carries, as amounts by account.
// What a node applying the commit transaction credits, and what the earnings
// endpoint reports.
func rewardsPaid(pin *pb.TxPin) map[string]*big.Int {
	out := map[string]*big.Int{}
	if pin == nil {
		return out
	}
	for _, r := range pin.GetRewards() {
		if r == nil || len(r.Processor) == 0 {
			continue
		}
		account := string(r.Processor)
		amount := new(big.Int).SetBytes(r.Amount)
		if existing, ok := out[account]; ok {
			// A well-formed commit transaction names an account once. Summing
			// rather than replacing means a malformed one cannot use a repeat
			// to hide a payment from a node checking the total.
			existing.Add(existing, amount)
			continue
		}
		out[account] = amount
	}
	return out
}

// rewardsBalance - whether a commit transaction's split adds up.
//
// pool == sum(rewards) + remainder. Checkable by every node from the commit
// transaction alone, without recomputing the split, which is the point: a node
// that cannot reproduce the weights (because balances have moved) can still
// refuse a commit transaction that creates or destroys money.
func rewardsBalance(pin *pb.TxPin) bool {
	if pin == nil {
		return true
	}
	pool := new(big.Int).SetBytes(pin.GetFeePool())
	total := new(big.Int).SetBytes(pin.GetFeeRemainder())
	for _, r := range pin.GetRewards() {
		if r == nil {
			continue
		}
		total.Add(total, new(big.Int).SetBytes(r.Amount))
	}
	return pool.Cmp(total) == 0
}

// EarningsFor - what the commit-transaction chain says this account has been
// paid, newest first, capped at limit records.
//
// Read from the chain rather than from a running total, because the chain is the
// only authority: a running total would have to be rebuilt on recovery and kept
// in step with reorganisation, and would be a second place for the number to be
// wrong. The cost is a scan of the pins held in memory, which is bounded by the
// retain window and is paid only when somebody asks.
//
// lifetime is everything the chain has settled to this account. pending is
// always nought here: a reward exists only once a commit transaction carries it,
// and at that point it is settled, so there is no window in which a reward is
// earned but not yet paid. The distinction is kept in the shape because fees
// charged per site before the commit transaction that settles them would
// reintroduce one.
func EarningsFor(account string, limit int) (lifetime, pending *big.Int, credits []RewardCredit) {
	lifetime, pending = new(big.Int), new(big.Int)
	credits = []RewardCredit{}
	if _pins_ == nil || account == "" {
		return lifetime, pending, credits
	}
	chain := _pins_.snapshotPins()
	for _, pin := range chain {
		amount, ok := rewardsPaid(pin)[account]
		if !ok || amount == nil || amount.Sign() <= 0 {
			continue
		}
		lifetime.Add(lifetime, amount)
		credits = append(credits, RewardCredit{
			Pin:    pin.PinNumber,
			Amount: new(big.Int).Set(amount),
			At:     pin.GetTs().AsTime(),
		})
	}
	// Newest first, and only as many as were asked for. Reversed after
	// accumulating rather than scanning backwards, because lifetime has to
	// cover the whole chain however few records the caller wants.
	for i, j := 0, len(credits)-1; i < j; i, j = i+1, j-1 {
		credits[i], credits[j] = credits[j], credits[i]
	}
	if limit > 0 && len(credits) > limit {
		credits = credits[:limit]
	}
	return lifetime, pending, credits
}
