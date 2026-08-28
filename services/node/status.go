package node

// State - what the node is doing, in the terms a wallet application puts in
// front of somebody who has just clicked a button.
type State string

const (
	// StateSyncing - processing is on but the node has not caught up yet. It will
	// begin encapsulating by itself once it has; there is nothing to click.
	StateSyncing State = "syncing"
	// StateReady - processing is on and the node has caught up, but it has no
	// peers, so a site it built would reach nobody.
	StateReady State = "ready"
	// StateProcessing - processing is on, the node has caught up, and it has
	// peers. This is the state that earns fees.
	StateProcessing State = "processing"
	// StateStopped - processing is off. The node still syncs, still validates and
	// still applies commit transactions; it just does not encapsulate
	// transactions into sites and so does not earn.
	StateStopped State = "stopped"
)

// DeriveState - the node's state from the three facts that decide it.
//
// The gate is read first, and that ordering is the point rather than an
// accident: when somebody stops processing, the answer to "what is my node
// doing" must be "stopped", not a report on a part of the node they did not ask
// about. That a stopped node keeps syncing is shown by the pin height in the
// same response, which keeps rising.
func DeriveState(processingEnabled, syncing bool, peers int) State {
	switch {
	case !processingEnabled:
		return StateStopped
	case syncing:
		return StateSyncing
	case peers <= 0:
		return StateReady
	default:
		return StateProcessing
	}
}
