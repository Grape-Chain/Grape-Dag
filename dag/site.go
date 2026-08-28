package dag

import "github.com/google/uuid"

// getConfirmedSites - return a list of confirmed sites, from the last batch of confirmed sites
//
// returns:
//
//	[]*Node - a list of 100% confirmed (directly or indirectly) sites
func (d *Dag) GetConfirmedSites() []*Node {
	d.mux.Lock()
	defer d.mux.Unlock()
	return confirmationCounter.pop()
}

func (d *Dag) GetTips() []*Node {
	d.mux.Lock()
	defer d.mux.Unlock()
	return confirmationCounter.tip()
}

// PeekConfirmedSites - the confirmed sites without consuming them. What a
// validator reports at the start of an epoch; see ConfirmTracker.peek.
func (d *Dag) PeekConfirmedSites() []*Node {
	d.mux.Lock()
	defer d.mux.Unlock()
	return confirmationCounter.peek()
}

// TakeConfirmedSites - consume exactly the sites a commit transaction settles.
func (d *Dag) TakeConfirmedSites(ids []uuid.UUID) []*Node {
	d.mux.Lock()
	defer d.mux.Unlock()
	return confirmationCounter.take(ids)
}
