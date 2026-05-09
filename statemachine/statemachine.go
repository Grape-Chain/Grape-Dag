package statemachine

import (
	"fmt"
	"sync"
	"time"

	"github.com/VG-Grape/luna/config"
	golog "github.com/ipfs/go-log/v2"
)

var logger golog.EventLogger

func init() {
	logger = golog.Logger("STATE")
}

type StateMachine struct {
	stateChanges StateChanges
	currentState State
	mu           sync.Mutex
	ch           chan State
}

func NewStateMachine(scs StateChanges, is State) *StateMachine {
	return &StateMachine{
		stateChanges: scs,
		currentState: is,
		mu:           sync.Mutex{},
		ch:           make(chan State, 1),
	}
}

func (sm *StateMachine) Current() State {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.currentState
}

func (sm *StateMachine) ResetTo(state State) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.currentState = state
}

func (sm *StateMachine) canChangeTo(newState State) bool {
	return sm.stateChanges.CanChangeState(sm.currentState, newState)
}

func (sm *StateMachine) SyncChangeTo(newState State, signal bool) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	current_sm := sm.currentState
	// logger.Infof("%s  ~ STATE CHANGE FROM %s to %s", emoji.ControlKnobs, current_sm, newState)
	flag := sm.canChangeTo(newState)
	if !flag {
		logger.Errorf("Cannot change state %s -> %s :%s", sm.currentState, newState, ErrStateChange.Error())
		return ErrStateChange
	}
	sm.currentState = newState
	if signal {
		// logger.Infof("[*] Signaling state change: %s", newState)
		sm.ch <- newState
	}
	if config.GetConfig().Host.Verbose > 3 {
		logger.Infof("State Change: %s -> %s", current_sm, sm.currentState)
	}
	// logger.Infof("%s  ~ %s  STATE CHANGED FROM %s to %s", emoji.ControlKnobs, emoji.CheckBoxWithCheck, current_sm, newState)
	return nil
}

func (sm *StateMachine) WaitForChange() State {
	return <-sm.ch
}

func (sm *StateMachine) WaitForState(waitState State, waitFor time.Duration) (State, error) {
	sm.mu.Lock()
	if sm.currentState == waitState {
		sm.mu.Unlock()
		return sm.currentState, nil
	}
	sm.mu.Unlock()
	t := time.NewTimer(waitFor)
	<-t.C
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.currentState != waitState {
		return sm.currentState, fmt.Errorf("no change to the desired state past the wait time")
	}
	return sm.currentState, nil
}
