package node

import (
	"sync"
	"testing"
	"time"
)

// restoreGate - put the package-level gate back as the rest of the process
// expects to find it. The gate is deliberately global, so a test that moves it
// has to put it back.
func restoreGate(t *testing.T) {
	t.Helper()
	previous := ProcessingEnabled()
	t.Cleanup(func() { SetProcessing(previous) })
}

// gateAtStartUp - the gate as the process found it, captured during package
// variable initialisation so that no test, in any file, can have moved it first.
var gateAtStartUp = processing.Load()

func TestTheProcessingGateStartsEnabledSoWiringItInChangesNothing(t *testing.T) {
	// A fresh process must publish exactly as it did before the check existed,
	// otherwise adding one line to the publish loop silently stops every node.
	if !gateAtStartUp {
		t.Fatal("the processing gate is not enabled at start-up")
	}
}

func TestSettingTheProcessingGateReportsWhatItWasBefore(t *testing.T) {
	restoreGate(t)

	SetProcessing(true)
	if previous := SetProcessing(false); previous != true {
		t.Fatalf("SetProcessing(false) reported previous = %t, want true", previous)
	}
	if previous := SetProcessing(true); previous != false {
		t.Fatalf("SetProcessing(true) reported previous = %t, want false", previous)
	}
}

func TestConcurrentFlipsOfTheProcessingGateLeaveItInADefinedState(t *testing.T) {
	restoreGate(t)

	const writers = 16
	const flips = 500
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < flips; i++ {
				SetProcessing(i%2 == 0)
			}
		}(w)
	}
	// Readers run alongside the writers because the publish loop reads the gate
	// on every transaction while an HTTP handler may be writing it. Under -race
	// this is the test that the two cannot tear.
	for r := 0; r < writers; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < flips; i++ {
				_ = ProcessingEnabled()
			}
		}()
	}
	wg.Wait()

	// Whatever the winner was, the gate reads as one of two values and reads the
	// same twice running.
	first := ProcessingEnabled()
	if second := ProcessingEnabled(); first != second {
		t.Fatalf("the gate read %t then %t with nothing writing it", first, second)
	}
}

func TestTheLastWriteToTheProcessingGateWins(t *testing.T) {
	restoreGate(t)

	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				SetProcessing(i%2 == 0)
			}
		}()
	}
	wg.Wait()
	SetProcessing(false)
	if ProcessingEnabled() {
		t.Fatal("the gate is enabled after a final SetProcessing(false)")
	}
	SetProcessing(true)
	if !ProcessingEnabled() {
		t.Fatal("the gate is disabled after a final SetProcessing(true)")
	}
}

func TestRecordingProcessedTransactionsConcurrentlyProducesARate(t *testing.T) {
	m := &rateMeter{}
	start := time.Now()
	// Open the window at a known instant before anything races on it: the meter
	// opens its window on whichever call reaches it first, and a test that let
	// that be a reader would be measuring an interval it had not chosen.
	_ = m.rate(start)

	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 250; i++ {
				m.record(start)
			}
		}()
	}
	// Reads race the writes: the status endpoint is read while the publish loop
	// is recording.
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 250; i++ {
				_ = m.rate(start)
			}
		}()
	}
	wg.Wait()

	// 2000 transactions over a two-second window.
	if got := m.rate(start.Add(2 * time.Second)); got != 1000 {
		t.Fatalf("rate over 2s after 2000 transactions = %v, want 1000", got)
	}
}

func TestAnIdleNodesContributionDecaysInsteadOfFreezing(t *testing.T) {
	m := &rateMeter{}
	start := time.Now()
	for i := 0; i < 100; i++ {
		m.record(start)
	}
	busy := m.rate(start.Add(time.Second))
	if busy != 100 {
		t.Fatalf("rate over 1s after 100 transactions = %v, want 100", busy)
	}
	// Nothing recorded for a further minute: the window rolls on read, so the
	// figure falls rather than sitting at 100 forever.
	if idle := m.rate(start.Add(time.Minute)); idle >= busy {
		t.Fatalf("rate after a minute of idleness = %v, want less than %v", idle, busy)
	}
}

func TestAContributionIsNotReportedFromTooShortASample(t *testing.T) {
	m := &rateMeter{}
	start := time.Now()
	// One transaction 20ms into a window is not 50 tx/s.
	m.record(start)
	if got := m.rate(start.Add(20 * time.Millisecond)); got != 0 {
		t.Fatalf("rate from a 20ms sample = %v, want 0", got)
	}
}
