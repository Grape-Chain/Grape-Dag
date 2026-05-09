package statemachine

import (
	"errors"
)

type State uint16

const (
	SYNC_ZERO_STATE State = iota
	SYNC_DISPATCH_BEGIN
	SYNC_DISPATCH_END
	SYNC_QUERY_BEGIN
	SYNC_QUERY_END
	SYNC_HANDLE_BEGIN
	SYNC_HANDLE_END
	SYNC_CANCEL_STATE
)

func (s State) String() string {
	return []string{
		"SYNC_ZERO_STATE",
		"SYNC_DISPATCH_BEGIN",
		"SYNC_DISPATCH_END",
		"SYNC_QUERY_BEGIN",
		"SYNC_QUERY_END",
		"SYNC_HANDLE_BEGIN",
		"SYNC_HANDLE_END",
		"SYNC_CANCEL_STATE"}[s]
}

type StateChanges map[State]map[State]struct{} // FromState -> ToState

var ErrStateChange = errors.New("invalid state change")

func NewStateChanges() StateChanges {
	return make(map[State]map[State]struct{})
}

func (sc StateChanges) AddStateChange(fromState, toState State) {
	newState, ok := sc[fromState]
	if !ok {
		newState = make(map[State]struct{})
		sc[fromState] = newState
	}
	newState[toState] = struct{}{}
}

type StateChangeFrom struct {
	sc        StateChanges
	fromState State
}

type StateChangeTo struct {
	sc        StateChanges
	fromState State
}

func (sc StateChanges) AddStateChangeFrom(state State) StateChangeFrom {
	return StateChangeFrom{
		sc:        sc,
		fromState: state,
	}
}

func (f StateChangeFrom) To(state State) StateChangeTo {
	f.sc.AddStateChange(f.fromState, state)
	return StateChangeTo{
		sc:        f.sc,
		fromState: state,
	}
}

func (i StateChangeTo) ThenTo(state State) StateChangeTo {
	i.sc.AddStateChange(i.fromState, state)
	return StateChangeTo{
		sc:        i.sc,
		fromState: state,
	}
}

func (sc StateChanges) CanChangeState(fromState, toState State) bool {
	tsc, ok := sc[fromState]
	if !ok {
		return false
	}
	_, ok = tsc[toState]
	return ok
}
