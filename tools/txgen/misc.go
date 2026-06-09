package txgen

import (
	"math/rand"
)

func TxnormalTime(rate uint64) int64 {
	t := rand.NormFloat64()/1000 + float64(rate)
	return int64(t)
}

func TxuniformTime(rate uint64) int64 {
	t := 60.0 / rate
	b := rand.Int31n(int32(t / 2))
	x := float64(rate)*3/4 + float64(b)
	return int64(x)
}

func TxdefaultTime(rate uint64) int64 {
	zt := 60.0 / float64(rate)
	return int64(zt * 1000)
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
	default:
		return false
	}

}
