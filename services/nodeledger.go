package services

import (
	"math/big"

	"github.com/Grape-Chain/Grape-Dag/app"
	"github.com/Grape-Chain/Grape-Dag/config"
	"github.com/Grape-Chain/Grape-Dag/dag"
	grapepeer "github.com/Grape-Chain/Grape-Dag/peer"
	"github.com/Grape-Chain/Grape-Dag/services/node"
)

/*
The live implementation of node.Ledger.

It lives here rather than in services/node because that package deliberately
depends on nothing inside the ledger: the endpoints, the state machine and the
processing gate are testable on their own precisely because they only see the
interface. This file is the one place that knows both sides, and package
services already imports app, dag, config and peer, so it adds no new edge to
the dependency graph.

Every accessor here has to survive being called before the node has finished
starting - the REST server is listening before the sync manager exists, and a
wallet application polls the status endpoint from the moment it launches the
node. So each one checks for the thing it reads rather than assuming it.
*/

// nodeLedger - reads the running node for the /node endpoints.
type nodeLedger struct{}

// NewNodeLedger - the live node.Ledger for this process.
func NewNodeLedger() node.Ledger { return nodeLedger{} }

// PinHeight - the newest commit transaction this node has applied. Rises
// whether or not the node is processing, which is what makes it the honest
// evidence that a stopped node is still syncing.
func (nodeLedger) PinHeight() int64 {
	p := dag.GetPin()
	if p == nil {
		return 0
	}
	return int64(p.CurrentHeight())
}

func (nodeLedger) PeerCount() int {
	host := grapepeer.GetHost()
	if host == nil {
		// Asked before the host was built. Nought connected is the truth, and
		// it is also what stops this from panicking during start-up.
		return 0
	}
	return len(host.Network().Peers())
}

// Syncing - true until the node has both applied the chain it started from and
// taken in the sites that came with it.
//
// A node with no sync manager yet is reported as syncing rather than ready: it
// has not caught up, because it has not started trying.
func (nodeLedger) Syncing() bool {
	a := app.GetApp()
	if a == nil || a.App_dagsyncmgr == nil {
		return true
	}
	mgr := a.App_dagsyncmgr
	return !mgr.HaveJoined.Load() || !mgr.SitesProcessed.Load()
}

// WalletAddress - the account this node's fees are paid to.
//
// Read from configuration rather than from dag.GetDag().Wallet(), which mints a
// throwaway wallet when none is configured: reporting an address that exists
// only in memory would tell a wallet application to watch an account that
// nothing will ever pay.
func (nodeLedger) WalletAddress() string {
	cfg := config.GetConfig()
	if cfg == nil {
		return ""
	}
	return cfg.Dag.Wallet
}

// recentCredits - how many individual payments the earnings endpoint reports.
// Enough for a wallet application to show a history without asking it to render
// the whole chain.
const recentCredits = 50

// EarningsFor - fees credited to a wallet, read from the commit-transaction
// chain.
//
// Zero is an honest answer here, not a placeholder: a node earns nothing until
// fees are switched on network-wide, and until then the chain genuinely carries
// no reward records. See docs/economics.md.
func (nodeLedger) EarningsFor(wallet string) (*big.Int, *big.Int, []node.Credit, error) {
	lifetime, pending, credits := dag.EarningsFor(wallet, recentCredits)
	out := make([]node.Credit, 0, len(credits))
	for _, c := range credits {
		out = append(out, node.Credit{
			Pin:    c.Pin,
			Amount: c.Amount,
			At:     c.At,
		})
	}
	return lifetime, pending, out, nil
}
