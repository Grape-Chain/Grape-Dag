package txgen

import (
	"math"
	"math/rand"
)

// Pacing helpers for the timer-driven generator modes.
//
// GenConfiguration.Txrate is documented as transactions per second, so the gap
// between two transactions is 1000/rate milliseconds. All three functions used
// to compute 60000/rate instead, which made every configured rate 60 times
// slower than asked for: a config asking for 2000/s produced a 30ms tick, that
// is 33/s. TxuniformTime was worse than merely slow. It passed int32(t/2) to
// rand.Int31n, and t/2 truncates to 0 for any rate above 120, so every rate a
// load test would actually use crashed the generator on the first tick.
// TxnormalTime returned the rate itself as a millisecond count, which is not an
// interval at all.
const (
	// minPacingIntervalMs is the floor on every interval returned here. A Go
	// ticker panics on a non-positive period, and a timer shorter than a
	// millisecond is not honoured by the runtime anyway, so rates that need a
	// shorter gap cannot be served by a sleep at all - that is what the batched
	// token bucket in bench mode is for.
	minPacingIntervalMs = 1

	// maxPacingIntervalMs caps an interval at a minute. Nothing a sane rate
	// produces comes close; it exists so that a nonsense input cannot turn into
	// an overflowing int64 conversion.
	maxPacingIntervalMs = 60000

	// uniformPacingSpread is the fraction of the mean interval that
	// TxuniformTime jitters either side of the mean. Symmetric, so jitter does
	// not shift the mean offered rate away from Txrate.
	uniformPacingSpread = 0.25

	// normalPacingSigma is TxnormalTime's standard deviation as a fraction of
	// the mean interval, and normalPacingTrunc is where the draw is truncated.
	// Without truncation a single sample from the tail stalls the generator for
	// a multiple of the interval, which shows up as a dip in the offered rate
	// that has nothing to do with the node.
	normalPacingSigma = 0.125
	normalPacingTrunc = 3.0
)

// meanPacingIntervalMs - the gap in milliseconds needed to offer rate
// transactions every second.
func meanPacingIntervalMs(rate uint64) float64 {
	if rate == 0 {
		// A zero rate has no interval to compute. The timer-driven modes have no
		// way to express "unpaced", so they get the shortest interval a ticker
		// will honour; bench mode has an explicit unpaced path for that intent.
		return minPacingIntervalMs
	}
	return 1000.0 / float64(rate)
}

// clampPacingIntervalMs keeps an interval positive, finite and representable
// whatever the caller's arithmetic produced, so that no rate value can make a
// caller of these functions panic.
func clampPacingIntervalMs(ms float64) int64 {
	if math.IsNaN(ms) || ms < minPacingIntervalMs {
		return minPacingIntervalMs
	}
	if ms > maxPacingIntervalMs {
		return maxPacingIntervalMs
	}
	return int64(math.Round(ms))
}

// TxnormalTime - interval in milliseconds until the next transaction, drawn from
// a truncated normal distribution around the mean interval for rate.
func TxnormalTime(rate uint64) int64 {
	z := rand.NormFloat64()
	if z > normalPacingTrunc {
		z = normalPacingTrunc
	}
	if z < -normalPacingTrunc {
		z = -normalPacingTrunc
	}
	return clampPacingIntervalMs(meanPacingIntervalMs(rate) * (1 + normalPacingSigma*z))
}

// TxuniformTime - interval in milliseconds until the next transaction, drawn
// uniformly from a band around the mean interval for rate.
func TxuniformTime(rate uint64) int64 {
	// rand.Float64 rather than rand.Int31n: the integer version had to derive an
	// integer bound from the interval, and that bound is 0 for every rate above
	// 120, where rand.Int31n panics.
	jitter := (rand.Float64()*2 - 1) * uniformPacingSpread
	return clampPacingIntervalMs(meanPacingIntervalMs(rate) * (1 + jitter))
}

// TxdefaultTime - constant interval in milliseconds for rate transactions per
// second.
func TxdefaultTime(rate uint64) int64 {
	return clampPacingIntervalMs(meanPacingIntervalMs(rate))
}

func IsProcessCommand(m GenMode) bool {
	switch m {
	case GEN_MODE_COUNT:
		return true
	case GEN_MODE_WALLET:
		return true
	case GEN_MODE_WATCHDOG:
		return true
	default:
		return false
	}
}

func SpinMode(m GenMode) bool {
	switch m {
	case GEN_MODE_TRADER:
		return true
	case GEN_MODE_LOCAL:
		return true
	case GEN_MODE_GENESIS:
		return true
	case GEN_MODE_WATCHDOG:
		return true
	default:
		// Bench mode deliberately stays out: a spinner redrawing the line the
		// periodic report is writing to makes the report unreadable, and the
		// redraw itself is work inside the measurement window.
		return false
	}
}

func GenesisWalletMode(m GenMode) bool {
	switch m {
	case GEN_MODE_TRADER:
		return true
	case GEN_MODE_LOCAL:
		return true
	case GEN_MODE_GENESIS:
		return true
	case GEN_MODE_PAYMENT:
		return true
	case GEN_MODE_BENCH:
		// Bench mode needs the genesis wallet as the faucet that pre-funds the
		// pool of sender wallets.
		return true
	default:
		return false
	}

}
