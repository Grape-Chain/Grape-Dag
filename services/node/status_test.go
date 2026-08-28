package node

import "testing"

func TestTheNodeStateFollowsTheGateThenSyncThenPeerCount(t *testing.T) {
	cases := []struct {
		name       string
		processing bool
		syncing    bool
		peers      int
		want       State
	}{
		{"processing when the gate is on, caught up and connected", true, false, 3, StateProcessing},
		{"ready when the gate is on and caught up but alone", true, false, 0, StateReady},
		{"syncing when the gate is on but the node is behind", true, true, 3, StateSyncing},
		{"syncing takes precedence over having no peers", true, true, 0, StateSyncing},
		{"stopped when the gate is off, however well the node is doing", false, false, 3, StateStopped},
		{"stopped when the gate is off while the node is still syncing", false, true, 0, StateStopped},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DeriveState(c.processing, c.syncing, c.peers); got != c.want {
				t.Fatalf("DeriveState(%t, %t, %d) = %q, want %q", c.processing, c.syncing, c.peers, got, c.want)
			}
		})
	}
}

func TestEveryOneOfTheFourStatesIsReachable(t *testing.T) {
	seen := map[State]bool{}
	for _, processing := range []bool{true, false} {
		for _, syncing := range []bool{true, false} {
			for _, peers := range []int{0, 1} {
				seen[DeriveState(processing, syncing, peers)] = true
			}
		}
	}
	for _, want := range []State{StateSyncing, StateReady, StateProcessing, StateStopped} {
		if !seen[want] {
			t.Errorf("no combination of inputs produces state %q", want)
		}
	}
}

func TestANegativePeerCountIsTreatedAsNoPeers(t *testing.T) {
	// PeerCount comes from an implementation this package does not own. A
	// nonsense value must not read as "connected".
	if got := DeriveState(true, false, -1); got != StateReady {
		t.Fatalf("DeriveState(true, false, -1) = %q, want %q", got, StateReady)
	}
}
