package dag

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
