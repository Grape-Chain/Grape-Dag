package dag

import (
	"testing"
	"time"

	sm "github.com/Grape-Chain/Grape-Dag/statemachine"
	"github.com/google/uuid"
)

// waitForSM used to hold the registry mutex for the whole wait, which blocked
// changeToSM (needing the same mutex) from recording the transition being waited
// for - so every wait timed out unless the state already matched.
func TestWaitForSMObservesTransitionFromAnotherGoroutine(t *testing.T) {
	id := uuid.New()
	syncsm.resetSM(id)
	defer syncsm.deleteSM(id)
	syncsm.changeToSM(id, sm.SYNC_DISPATCH_BEGIN)

	go func() {
		time.Sleep(20 * time.Millisecond)
		syncsm.changeToSM(id, sm.SYNC_DISPATCH_END)
	}()

	state, err := syncsm.waitForSM(id, sm.SYNC_DISPATCH_END, 500)
	if err != nil {
		t.Fatalf("wait did not observe the state change: %s (state=%s)", err.Error(), state)
	}
	if state != sm.SYNC_DISPATCH_END {
		t.Fatalf("state = %s, want %s", state, sm.SYNC_DISPATCH_END)
	}
}

func TestWaitForSMOnUnknownIdReturnsError(t *testing.T) {
	if _, err := syncsm.waitForSM(uuid.New(), sm.SYNC_DISPATCH_END, 10); err == nil {
		t.Fatalf("expected an error for an unknown state machine id")
	}
}

// Other state machines must remain usable while one of them is being waited on.
func TestWaitForSMDoesNotBlockOtherMachines(t *testing.T) {
	waiting := uuid.New()
	other := uuid.New()
	syncsm.resetSM(waiting)
	syncsm.resetSM(other)
	defer syncsm.deleteSM(waiting)
	defer syncsm.deleteSM(other)

	done := make(chan struct{})
	go func() {
		defer close(done)
		syncsm.waitForSM(waiting, sm.SYNC_DISPATCH_END, 400)
	}()

	// give the waiter time to enter the wait
	time.Sleep(20 * time.Millisecond)
	progressed := make(chan struct{})
	go func() {
		defer close(progressed)
		syncsm.changeToSM(other, sm.SYNC_DISPATCH_BEGIN)
	}()

	select {
	case <-progressed:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("a wait on one state machine blocked progress on another")
	}
	<-done
}

// syncPublish calls resetSM after a successful publish, which replaces the
// registry entry with a fresh machine. A waiter that cached the old pointer
// would never see the transition, so waitForSM must re-resolve as it polls.
func TestWaitForSMSurvivesMachineReplacement(t *testing.T) {
	id := uuid.New()
	syncsm.resetSM(id)
	defer syncsm.deleteSM(id)

	go func() {
		time.Sleep(20 * time.Millisecond)
		// this is what the publisher does: replace, then drive the transitions
		syncsm.resetSM(id)
		syncsm.changeToSM(id, sm.SYNC_DISPATCH_BEGIN)
		syncsm.changeToSM(id, sm.SYNC_DISPATCH_END)
	}()

	state, err := syncsm.waitForSM(id, sm.SYNC_DISPATCH_END, 800)
	if err != nil {
		t.Fatalf("wait missed the transition on the replacement machine: %s (state=%s)", err.Error(), state)
	}
}

// A wait must return promptly once the state matches, rather than sleeping out
// its whole budget.
func TestWaitForSMReturnsAsSoonAsTheStateMatches(t *testing.T) {
	id := uuid.New()
	syncsm.resetSM(id)
	defer syncsm.deleteSM(id)
	syncsm.changeToSM(id, sm.SYNC_DISPATCH_BEGIN)
	go func() {
		time.Sleep(30 * time.Millisecond)
		syncsm.changeToSM(id, sm.SYNC_DISPATCH_END)
	}()

	start := time.Now()
	if _, err := syncsm.waitForSM(id, sm.SYNC_DISPATCH_END, 3000); err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("wait took %s, expected to return shortly after the transition", elapsed)
	}
}
