package config

import "testing"

// Fees are off unless a height is set, and the boundary is inclusive: the pin
// named by feestartpin is the first that charges. Asked in three places on
// three different paths, so it is one function rather than three copies of a
// comparison.
func TestFeesAreOffUntilTheConfiguredHeight(t *testing.T) {
	off := TxConfiguration{Feestartpin: -1, Minpaymentfee: 1000}
	for _, h := range []int64{0, 1, 1000, 1 << 40} {
		if off.FeesActive(h) {
			t.Fatalf("fees active at pin %d with feestartpin -1", h)
		}
		if got := off.MinimumPaymentFee(h); got != 0 {
			t.Fatalf("minimum fee at pin %d is %d with fees off, want 0", h, got)
		}
	}

	on := TxConfiguration{Feestartpin: 100, Minpaymentfee: 1000}
	cases := []struct {
		pin    int64
		active bool
	}{
		{0, false}, {99, false}, {100, true}, {101, true}, {1 << 40, true},
	}
	for _, c := range cases {
		if got := on.FeesActive(c.pin); got != c.active {
			t.Fatalf("pin %d: fees active = %v, want %v", c.pin, got, c.active)
		}
		want := uint64(0)
		if c.active {
			want = 1000
		}
		if got := on.MinimumPaymentFee(c.pin); got != want {
			t.Fatalf("pin %d: minimum fee %d, want %d", c.pin, got, want)
		}
	}
}

// Zero is a legitimate start height - a chain that has charged fees from its
// genesis - and must not be confused with "off".
func TestFeesCanStartAtTheGenesisPin(t *testing.T) {
	c := TxConfiguration{Feestartpin: 0, Minpaymentfee: 7}
	if !c.FeesActive(0) {
		t.Fatal("feestartpin 0 does not charge at pin 0")
	}
	if got := c.MinimumPaymentFee(0); got != 7 {
		t.Fatalf("minimum fee at pin 0 is %d, want 7", got)
	}
}
