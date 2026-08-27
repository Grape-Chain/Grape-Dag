package dag

import (
	"errors"
	"fmt"
	"sync"
	"time"

	sm "github.com/Grape-Chain/Grape-Dag/statemachine"
	"github.com/google/uuid"
)

func NewSyncStateMachine() *sm.StateMachine {
	sync_sc := sm.NewStateChanges()
	sync_sc.AddStateChangeFrom(sm.SYNC_ZERO_STATE).To(sm.SYNC_DISPATCH_BEGIN)
	sync_sc.AddStateChangeFrom(sm.SYNC_ZERO_STATE).To(sm.SYNC_QUERY_BEGIN)
	sync_sc.AddStateChangeFrom(sm.SYNC_DISPATCH_BEGIN).
		To(sm.SYNC_DISPATCH_END).
		ThenTo(sm.SYNC_DISPATCH_BEGIN)
	sync_sc.AddStateChangeFrom(sm.SYNC_DISPATCH_BEGIN).
		To(sm.SYNC_CANCEL_STATE)
	sync_sc.AddStateChangeFrom(sm.SYNC_QUERY_BEGIN).
		To(sm.SYNC_HANDLE_BEGIN).
		ThenTo(sm.SYNC_HANDLE_END).
		ThenTo(sm.SYNC_QUERY_END).
		ThenTo(sm.SYNC_QUERY_BEGIN)
	sync_sc.AddStateChangeFrom(sm.SYNC_QUERY_BEGIN).
		To(sm.SYNC_CANCEL_STATE)
	sync_sc.AddStateChangeFrom(sm.SYNC_HANDLE_BEGIN).
		To(sm.SYNC_HANDLE_END).
		ThenTo(sm.SYNC_HANDLE_BEGIN)
	sync_sc.AddStateChangeFrom(sm.SYNC_HANDLE_BEGIN).
		To(sm.SYNC_CANCEL_STATE)
	return sm.NewStateMachine(sync_sc, sm.SYNC_ZERO_STATE)

}

type SyncSM struct {
	machines map[uuid.UUID]*sm.StateMachine
	mx       sync.Mutex
}

var syncsm SyncSM = SyncSM{
	machines: make(map[uuid.UUID]*sm.StateMachine),
	mx:       sync.Mutex{},
}

func (sm *SyncSM) resetSM(id uuid.UUID) {
	sm.mx.Lock()
	defer sm.mx.Unlock()
	delete(sm.machines, id)
	sm.machines[id] = NewSyncStateMachine()
}

func (sm *SyncSM) deleteSM(id uuid.UUID) {
	sm.mx.Lock()
	defer sm.mx.Unlock()
	delete(sm.machines, id)
}

func (s *SyncSM) changeToSM(id uuid.UUID, sc sm.State) {
	s.mx.Lock()
	defer s.mx.Unlock()
	if _, ok := s.machines[id]; ok {
		s.machines[id].SyncChangeTo(sc, false)
	} else {
		logger.Warnf("No state machine for id: %s", id)
	}
}

func (s *SyncSM) currentSM(id uuid.UUID) sm.State {
	s.mx.Lock()
	defer s.mx.Unlock()
	if v, ok := s.machines[id]; ok {
		return v.Current()
	}
	return sm.SYNC_ZERO_STATE
}

func (sm *SyncSM) existsSM(id uuid.UUID) bool {
	sm.mx.Lock()
	defer sm.mx.Unlock()
	_, ok := sm.machines[id]
	return ok
}

// waitForSMPollInterval - how often waitForSM re-reads the registry while
// waiting. Short enough that a transition is picked up promptly, long enough not
// to spin.
const waitForSMPollInterval = 20 * time.Millisecond

func (s *SyncSM) waitForSM(id uuid.UUID, state sm.State, t int64) (sm.State, error) {
	// Poll, re-resolving the machine from the registry each time, and never hold
	// s.mx while sleeping. Both matter:
	//   - holding s.mx across the wait would block changeToSM (same lock) from
	//     recording the very transition being waited for;
	//   - caching the *StateMachine would miss the transition entirely, because
	//     syncPublish calls resetSM after a successful publish, which replaces
	//     the map entry with a fresh machine.
	deadline := time.Now().Add(time.Millisecond * time.Duration(t))
	for {
		s.mx.Lock()
		v, ok := s.machines[id]
		s.mx.Unlock()
		if !ok {
			return sm.SYNC_ZERO_STATE, fmt.Errorf("Failed to find a statemachine with id %s", id.String())
		}
		current := v.Current()
		if current == state {
			return current, nil
		}
		if !time.Now().Before(deadline) {
			return current, fmt.Errorf("no change to the desired state past the wait time")
		}
		remaining := time.Until(deadline)
		if remaining > waitForSMPollInterval {
			remaining = waitForSMPollInterval
		}
		time.Sleep(remaining)
	}
}

func (s *SyncSM) waitForSMInLoop(id uuid.UUID, state sm.State, t time.Duration) (sm.State, error) {
	ticker := time.NewTicker(3 * time.Second)
	timer := time.NewTimer(t)
	for {
		select {
		case <-ticker.C:
			state, err := s.waitForSM(id, state, 100)
			if err == nil {
				return state, err
			}
		case <-timer.C:
			return sm.SYNC_ZERO_STATE, errors.New("state of the state machine isn't equal to required")
		}
	}
	
}
