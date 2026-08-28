package txgen

import (
	"math"
	"testing"
)

// pacingFunctions - the three exported pacing functions, so that a property that
// has to hold for all of them is not written out three times.
var pacingFunctions = []struct {
	name string
	fn   func(uint64) int64
}{
	{"TxdefaultTime", TxdefaultTime},
	{"TxuniformTime", TxuniformTime},
	{"TxnormalTime", TxnormalTime},
}

func TestTxrateIsTransactionsPerSecondForEveryPacingFunction(t *testing.T) {
	cases := []struct {
		rate   uint64
		meanMs float64
	}{
		{0, 1}, // no interval to compute, so the shortest a ticker will honour
		{1, 1000},
		{2, 500},
		{60, 1000.0 / 60},
		{100, 10},
		{120, 1000.0 / 120},
		{121, 1000.0 / 121},
		{1000, 1},
		{10000, 1},
		{100000, 1},
	}
	for _, c := range cases {
		wantDefault := int64(math.Round(c.meanMs))
		if wantDefault < minPacingIntervalMs {
			wantDefault = minPacingIntervalMs
		}
		if got := TxdefaultTime(c.rate); got != wantDefault {
			t.Errorf("TxdefaultTime(%d) = %d ms, want %d ms", c.rate, got, wantDefault)
		}
		// The distributions jitter around the same mean, so they have to stay
		// inside their documented band; otherwise the mean offered rate is not
		// the configured rate.
		checkWithinBand(t, "TxuniformTime", c.rate, c.meanMs, uniformPacingSpread, TxuniformTime)
		checkWithinBand(t, "TxnormalTime", c.rate, c.meanMs, normalPacingSigma*normalPacingTrunc, TxnormalTime)
	}
}

func checkWithinBand(t *testing.T, name string, rate uint64, meanMs, spread float64, fn func(uint64) int64) {
	t.Helper()
	lo := int64(math.Floor(meanMs * (1 - spread)))
	if lo < minPacingIntervalMs {
		lo = minPacingIntervalMs
	}
	hi := int64(math.Ceil(meanMs * (1 + spread)))
	if hi < minPacingIntervalMs {
		hi = minPacingIntervalMs
	}
	for i := 0; i < 2000; i++ {
		got := fn(rate)
		if got < lo || got > hi {
			t.Fatalf("%s(%d) = %d ms, want within [%d, %d] ms of a %.3f ms mean", name, rate, got, lo, hi, meanMs)
		}
	}
}

// TestTxdefaultTimeIsNoLongerSixtyTimesTooSlow pins the arithmetic that was
// wrong. Every pacing function used to divide 60000 by the rate, so a config
// asking for 60/s was paced at 1/s.
func TestTxdefaultTimeIsNoLongerSixtyTimesTooSlow(t *testing.T) {
	// Rates that divide 1000, so the comparison is not blurred by rounding the
	// interval to whole milliseconds.
	for _, rate := range []uint64{1, 2, 5, 10, 25, 100, 250, 500, 1000} {
		wasMs := int64((60.0 / float64(rate)) * 1000)
		nowMs := TxdefaultTime(rate)
		if nowMs*60 != wasMs {
			t.Errorf("TxdefaultTime(%d) = %d ms; the old formula gave %d ms, which should be exactly 60x more",
				rate, nowMs, wasMs)
		}
	}
}

// TestTxuniformTimeNoLongerPanicsAboveOneHundredAndTwentyPerSecond pins the
// crash. TxuniformTime used to pass int32(mean/2) to rand.Int31n, and that bound
// is 0 for every rate above 120, where rand.Int31n panics - so every rate a load
// test would use killed the generator on its first tick.
func TestTxuniformTimeNoLongerPanicsAboveOneHundredAndTwentyPerSecond(t *testing.T) {
	for rate := uint64(121); rate <= 5000; rate += 7 {
		for i := 0; i < 20; i++ {
			if got := TxuniformTime(rate); got < minPacingIntervalMs {
				t.Fatalf("TxuniformTime(%d) = %d ms, want at least %d ms", rate, got, minPacingIntervalMs)
			}
		}
	}
}

func TestNoPacingFunctionPanicsAtAnyRate(t *testing.T) {
	rates := []uint64{0, 1, 2, 119, 120, 121, 500, 1000, 10000, 100000, 1000000, math.MaxUint32, math.MaxUint64}
	for rate := uint64(0); rate <= 2000; rate += 13 {
		rates = append(rates, rate)
	}
	for _, f := range pacingFunctions {
		for _, rate := range rates {
			for i := 0; i < 25; i++ {
				got := f.fn(rate)
				if got < minPacingIntervalMs || got > maxPacingIntervalMs {
					t.Fatalf("%s(%d) = %d ms, want within [%d, %d] ms",
						f.name, rate, got, minPacingIntervalMs, maxPacingIntervalMs)
				}
			}
		}
	}
}

func TestBenchModeUsesTheGenesisWalletButNotTheSpinner(t *testing.T) {
	if !GenesisWalletMode(GEN_MODE_BENCH) {
		t.Error("GenesisWalletMode(GEN_MODE_BENCH) = false, want true: the faucet is the genesis wallet")
	}
	if SpinMode(GEN_MODE_BENCH) {
		t.Error("SpinMode(GEN_MODE_BENCH) = true, want false: the spinner would overwrite the progress reports")
	}
	if IsProcessCommand(GEN_MODE_BENCH) {
		t.Error("IsProcessCommand(GEN_MODE_BENCH) = true, want false: bench is not a one-shot command")
	}
}

func TestEveryGenModeStillMapsToItsOwnName(t *testing.T) {
	// GenMode.Type indexes a slice by the enum value, so adding bench anywhere
	// but the end would silently rename the modes after it.
	for _, name := range []string{"genesis", "trader", "local", "balance", "payment", "wallet", "count", "check", "watchdog", "bench"} {
		mode, ok := ModeMapper(name)
		if !ok {
			t.Fatalf("ModeMapper(%q) reported unknown mode", name)
		}
		if mode.Type() != name {
			t.Errorf("ModeMapper(%q).Type() = %q, want %q", name, mode.Type(), name)
		}
	}
}
