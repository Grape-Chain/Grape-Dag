package dag

import (
	"math/big"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Grape-Chain/Grape-Dag/config"
	"github.com/Grape-Chain/Grape-Dag/crypto"
	grape_wallet "github.com/Grape-Chain/Grape-Dag/crypto"
	grapepeer "github.com/Grape-Chain/Grape-Dag/peer"
	"github.com/Grape-Chain/Grape-Dag/stats"
	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/Grape-Chain/Grape-Dag/utils"
	"github.com/enescakir/emoji"
	"github.com/google/uuid"
	golog "github.com/ipfs/go-log/v2"
	"github.com/ledongthuc/goterators"
	"github.com/pkg/errors"
)

type Link struct {
	source *Node // a source vertex is a vertex that a target vertex references and approves
	target *Node // a target vertex is a vertex that approves the source vertex
}

type Dag struct {
	// optimize lookups by replacing the slice with a map keyed on UUID
	_dag_           []*Node
	mapped_vertices map[uuid.UUID]*Node
	_links_         []Link
	mapped_edges    map[uuid.UUID][]uuid.UUID
	mu_map          sync.RWMutex
	prevMajor       uint64
	prevMinor       uint32
	mux             sync.Mutex
	txCh            chan TxVL
	depthCh         chan Node
	pinCh           chan bool
	stopCh          chan bool
	pins            []*Node
	width           uint8
	exodusWallets   []*grape1crypto.Wallet
}

var (
	_dag_                *Dag                 = nil // global dag
	_pins_               *NodeTxPin           = nil
	__leaderReady__      atomic.Bool          = atomic.Bool{}
	chaintype            tx.ChainType         = tx.PRIVATE_TESTNET
	dagWallet            *grape_wallet.Wallet = nil // node's wallet with its own set of encr keys
	confirmationCounter  confirmations        = nil
	walletCache          *WalletCache         = nil // keep track of the current balances
	walletCacheConfirmed *WalletCache         = nil // keep track of the current balances without unconfirmed payment tx
)

// confirmations - how the DAG decides a site is confirmed and ready for a
// commit transaction. Two implementations exist: ConfirmTracker, which measures
// the share of current tips that confirm each site (the technical paper's
// section 5.1), and ConfirmationCounter, the original fixed two-approver rule,
// kept selectable while the new one earns trust.
type confirmations interface {
	add(vertex *Node)
	relink(vertex *Node)
	pop() []*Node
	tip() []*Node
	getTips() []*Node
	isTip(id uuid.UUID) bool
	markHarvested(id uuid.UUID)
}

type DagAlgo uint8

const (
	DAG_ALGO_MCMCP DagAlgo = iota
	DAG_ALGO_MCMCPP
	DAG_ALGO_RANDOM
)

func (a DagAlgo) Type() string {
	return []string{"mcmc+", "mcmc++", "random"}[a]
}

const (
	DAG_ALPHA      = 0.5
	DAG_APPROVE_TX = 2
	DAG_WIDTH      = 5
	DAG_LAMBDA     = 1
	DAG_TOTAL_TX   = 75
	UUID_TEMPLATE  = "6d89b2ad-9e07-46c4-8484-33c10b2dd6f%x"
	TX_MAXFUEL     = 999
	TX_MAXPRICE    = 10000000
	TX_NEUTRINO    = 0.00000001
)

var logger golog.EventLogger = golog.Logger("dag")
var peerConfig config.PeerConfiguration
var dagConfig config.DagConfiguration
var txConfig config.TxConfiguration

func (dag *Dag) Genesis() *Node {
	return dag.getGenesis()
}

func tipCache() confirmations {
	return confirmationCounter
}

func (d *Dag) GetWalletCache() *WalletCache {
	return walletCache
}

func (dag *Dag) Wallet() *grape_wallet.Wallet {
	if dagWallet == nil {
		dagWallet = grape1crypto.NewWallet()
	}
	return dagWallet
}

func (dag *Dag) Size() uint64 {
	dag.mux.Lock()
	defer dag.mux.Unlock()
	return uint64(len(dag._dag_))
}

func (d *Dag) Tps() float64 {
	d.mux.Lock()
	defer d.mux.Unlock()
	l := len(d._dag_)
	if l > 1 {
		n1 := d._dag_[1]
		nX := d._dag_[l-1]
		dur := nX.time.Sub(n1.time)
		return float64(l) / dur.Seconds()
	}
	return 0.
}

func (d *Dag) AvgDelay() int64 {
	d.mux.Lock()
	defer d.mux.Unlock()
	l := len(d._dag_)
	if l > 1 {
		n1 := d._dag_[1]
		n1dur := n1.time.Sub(time.UnixMilli(int64(n1.tx.GetTimestamp())))
		nM := d._dag_[l/2-1]
		nMdur := nM.time.Sub(time.UnixMilli(int64(nM.tx.GetTimestamp())))
		nX := d._dag_[l-1]
		nXdur := nX.time.Sub(time.UnixMilli(int64(nX.tx.GetTimestamp())))
		return (n1dur.Microseconds() + nXdur.Microseconds() + nMdur.Microseconds()) / 3
	}
	return 0.
}

// SnapshotNodes - a shallow copy of the current site slice, taken under the dag
// lock. Readers iterate the copy so that an append (which may reallocate the
// backing array) cannot race them.
func (d *Dag) SnapshotNodes() []*Node {
	d.mux.Lock()
	defer d.mux.Unlock()
	out := make([]*Node, len(d._dag_))
	copy(out, d._dag_)
	return out
}

func (d *Dag) lookupCache(id uuid.UUID) bool {
	_, ok := d.mapped_vertices[id]
	return ok
}

// mapped Vertices - is cache optimized lookup optimization
func (dag *Dag) lookupCacheUpdate(vertex *Node, targetIds []uuid.UUID) {
	// this, most likely, is a re-entry into dag from the primary thread of execution
	// hence use trylock
	dag.mu_map.Lock()
	defer dag.mu_map.Unlock()
	dag.mapped_vertices[vertex.id.id] = vertex
	dag.mapped_edges[vertex.id.id] = append(dag.mapped_edges[vertex.id.id], targetIds...)
}

// getById - get a node by its UUID
// Arguments:
//
//	id - site unique id
//	cacheOnly - check inly in cache if true, full search otherwise
//
// Returns:
//
//	*Node - if found, nil otherwise
func (d *Dag) getById(id uuid.UUID, cacheOnly bool) *Node {
	d.mu_map.RLock()
	defer d.mu_map.RUnlock()
	v, ok := d.mapped_vertices[id]
	if !ok && !cacheOnly {
		v = _dag_._getById_(id)
	}
	return v
}

// getById - get a *Node by its id (uuid)
// returns *Node if found, nil otherwise
func (d *Dag) _getById_(id uuid.UUID) *Node {
	if d.mux.TryLock() {
		defer d.mux.Unlock()
	}
	// the lookup can be optimized
	node, _, err := goterators.Find(d._dag_, func(n *Node) bool {
		return n.id.id == id
	})
	if err != nil {
		logger.Warnf("Site with id=%s not found in dag", id.String())
		return nil
	}
	return node
}

func (dag *Dag) updateMappedVertices() {
	dag.mu_map.Lock()
	defer dag.mu_map.Unlock()
	goterators.ForEach(dag._dag_, func(vertex *Node) {
		dag.mapped_vertices[vertex.id.id] = vertex
	})
}

func (dag *Dag) updateMappedEdges() {
	dag.mu_map.Lock()
	defer dag.mu_map.Unlock()
	goterators.ForEach(dag._links_, func(edge Link) {
		if _, _, err := goterators.Find(dag.mapped_edges[edge.source.id.id], func(id uuid.UUID) bool {
			return id == edge.target.id.id
		}); err != nil {
			dag.mapped_edges[edge.source.id.id] = append(dag.mapped_edges[edge.source.id.id], edge.target.id.id)
		}
	})
}

func (dag *Dag) Vertex(id uuid.UUID) *Node {
	dag.mu_map.RLock()
	defer dag.mu_map.RUnlock()
	v, ok := dag.mapped_vertices[id]
	if ok {
		return v
	}
	return nil
}

func (dag *Dag) Terminate() {
	utils.ColorizeInfo(logger, "[dag] Persisting the latest DAG state")
	utils.ColorizeWarn(logger, "[dag] @TODO: Implement DAG serialization")
	logger.Info("[dag] Stopping the DAG watcher")
	dag.stopCh <- true
	if dag.txCh != nil {
		logger.Info("[dag] Closing the TX channel")
		close(dag.txCh)
	}
	if dag.depthCh != nil {
		logger.Info("[dag] Closing the tx depth channel")
		close(dag.depthCh)
	}
	if dag.pinCh != nil {
		logger.Info("[dag] Closing the pin tx channel")
		close(dag.pinCh)
	}
	t := time.NewTimer(time.Duration(time.Millisecond * 300))
	<-t.C
	t.Stop()
	logger.Info("[dag] Closing the Stop channel")
	close(dag.stopCh)
	utils.ColorizeInfo(logger, "[dag] DAG successfully persisted and terminated")
}

func (d *Dag) countConfirmed(vertex *Node) {
	confirmationCounter.add(vertex)
}

func (d *Dag) insertMissing(vertex *Node) error {
	d._dag_ = append(d._dag_, vertex)
	// update lookup cache
	// Node: this step is important for efficient lookups
	d.lookupCacheUpdate(vertex, goterators.Map(vertex.targets, func(v *Node) uuid.UUID {
		return v.id.id
	}))
	if d.txCh != nil {
		d.txCh <- TxVL{vertex: *vertex, edges: goterators.Map(vertex.targets, func(v *Node) Node {
			return *v
		})}
	}

	return nil
}

func (dag *Dag) addToDag(node *Node, linksTo []*Node) (*Dag, error) {
	var e []Node = []Node{}
	// at this point we either accept the transaction, update its balance or bail out
	// unless dealing with synchronization
	// in case of synchronization: we still need to reconcile the balances,
	// but updating balances when there are missing transactions makes it difficult
	// @TODO: find a solution to balance update during synchronization; for now skip
	// balance update when we are in sync mode: aka linksTo is nil
	if linksTo != nil && !node.UpdateBalanceIfValid() {
		logger.Errorf("[!] Invalid balance for %s. Ignore", node.tx.String())
		return dag, errors.Errorf("[!] Invalid balance for %s. Ignore", node.tx.String())
	}

	goterators.ForEach(linksTo, func(v *Node) {
		e = append(e, *v)
		// add this node to the tips as
		if v.sources == nil {
			v.sources = []*Node{}
		}
		v.sources = append(v.sources, node)
		if node.targets == nil {
			node.targets = []*Node{}
		}
		node.targets = append(node.targets, v)
	})
	node.time = time.Now()
	dag._dag_ = append(dag._dag_, node)
	// update lookup cache
	// Node: this step is important for efficient lookups
	dag.lookupCacheUpdate(node, goterators.Map(linksTo, func(n *Node) uuid.UUID {
		return n.id.id
	}))
	// after adding a new vertex to DAG, update the tip counter
	// as we need them when finding confirmed vertices
	dag.countConfirmed(node)

	// notify the dag watcher for changes - dag watcher keeps track of all the changes
	// and builds a reverse graph representation of our dag
	// dag_watcher(sync.go) reads from the channel and calls Notify
	// which keeps the graph up to date
	if dag.txCh != nil {
		dag.txCh <- TxVL{vertex: *node, edges: e}
	}
	return dag, nil
}

func (d *Dag) getGenesis() *Node {
	if d != nil {
		return d._dag_[0]
	}
	return nil
}

func initDagWallet(dagConfig config.DagConfiguration) *grape_wallet.Wallet {
	return grape_wallet.LoadWallet(dagConfig.Publickey, dagConfig.Privatekey)
}

func InitDagToStats(statsId uuid.UUID) {
	goterators.ForEach(_dag_._dag_, func(n *Node) {
		stats.Enqueue(statsId, tx.ConvertToGrapeTx(grapepeer.GetHost(), n.tx), stats.TX_TYPE_PUB, 0, time.Duration(0))
	})
}

func Init() {
	logger = golog.Logger("dag")
	x := config.GetConfig()
	if x != nil {
		peerConfig = x.Peer
		dagConfig = x.Dag
		txConfig = x.Tx
		chaintype = tx.ChainType(x.Peer.Network)
	} else {
		chaintype = tx.PRIVATE_TESTNET
		peerConfig = config.PeerConfiguration{
			Network: 2, // Default value is set to PRIVATE_TESTNET
		}
		dagConfig = config.DagConfiguration{
			Algorithm:    DAG_ALGO_MCMCP.Type(),
			Alpha:        DAG_ALPHA,
			Approvetx:    DAG_APPROVE_TX,
			Initialwidth: DAG_WIDTH,
			Lambda:       DAG_LAMBDA,
			Totaltx:      DAG_TOTAL_TX,
		}
		logger.Warnf("Using default DAG config\n%s", &dagConfig)
		txConfig = config.TxConfiguration{
			Maxfuellimit: TX_MAXFUEL,
			Maxfuelprice: TX_MAXPRICE,
			Neutrino:     TX_NEUTRINO,
		}
	}
	walletCache = newWalletCache()
	walletCacheConfirmed = newWalletCache()
	dagWallet = initDagWallet(dagConfig)
	confirmationCounter = newConfirmations(dagConfig)
	_pins_ = newNodeTxPin()
	// Note: genesis node is the only node that is authorized to create the genesis tx as the starting
	// point in dag. This is not currently implemented.
	// The genesis node creates a genesis tx, all other nodes, upon start, must synchronize with the
	// genesis node and obtain, among other tx, the genesis tx.
	// Currently, every node that starts up, creates its own genesis tx. This must be implemented asap.
	_dag_ = newDag(dagConfig)

	if config.GetConfig().Host.Leader {
		ico, err := _pins_.unsafe_getBalanceForWallet(dagWallet.WalletAddress())
		if err != nil {
			panic(err.Error())
		}
		if ico == nil || ico.Cmp(_dag_.getGenesis().tx.GetAmount()) != 0 {
			panic("Genesis amount in cache is incorrect. Cannot contine initialization")
		}
	}
}

func (sm *DagSyncMngr) RunSynchronization(leader bool, wait_connect bool) {
	if _dag_ != nil {
		if host := grapepeer.GetHost(); host == nil {
			logger.Fatal("This peer's host has not been initialized")
		}
		// we set the sync channel for transactions here so that we can run
		// it only when synchronization is enabled
		_dag_.txCh = make(chan TxVL, 1)
		_dag_.depthCh = make(chan Node, 1)
		_dag_.pinCh = make(chan bool, 1)
		wg := &sync.WaitGroup{}
		wg.Add(1)
		go sm.dag_watcher(leader, wait_connect, wg)
		wg.Wait()
		logger.Info("[dag sync] DAG synchronization is running...")
	} else {
		logger.Fatal("DAG has not been initialized")
	}
}

func GetDag() *Dag {
	return _dag_
}

func GetPin() *NodeTxPin {
	return _pins_
}

func genRandomTxWeight() float64 {
	txWeight := rand.NormFloat64() + config.TX_WEIGHT_MEAN
	if txWeight < config.TX_WEIGHT_LOWER_LIMIT {
		txWeight = config.TX_WEIGHT_LOWER_LIMIT
	} else if txWeight > config.TX_WEIGHT_UPPER_LIMIT {
		txWeight = config.TX_WEIGHT_UPPER_LIMIT
	}
	return txWeight
}

func (d *Dag) approveNode(node *Node) error {
	if node == nil {
		return errors.New("[approveNode] node is nil")
	}
	//
	if !node.valid {
		return errors.New("[approveNode] node is invalid")
	}
	// check balance
	balances, err := GetPin().GetBalances([][]byte{node.tx.GetSender()})
	if err != nil {
		return err
	}
	sndBal := big.NewInt(0).SetBytes(balances[0])
	if sndBal.Sign() < 0 {
		return errors.New("Sender balance is negative")
	}
	// if necessary, check the receiver's balance as well
	return nil
}

// approveTx - process tx approval logic
// requires additional implementation
func (dag *Dag) approveTx(nodes []*Node) bool {
	if len(nodes) == 0 {
		return false
	}
	count := dagConfig.Approvetx
	goterators.ForEach(nodes, func(node *Node) {
		if node != nil {
			if err := dag.approveNode(node); err == nil {
				utils.ColorizeInfo(logger, "Approved TX:[%s|%d.%d]", node.id.id.String(), node.id.idMajor, node.id.idMinor)
			} else {
				utils.ColorizeError(logger, "Failed TX:[%s|%d.%d] err:", node.id.id.String(), node.id.idMajor, node.id.idMinor, err.Error())
			}
		} else {
			count--
		}
	})
	return count == dagConfig.Approvetx
}

func (dag *Dag) getAdjEdges(vertex *Node) []*Node {
	// see where the current node is the source
	edges := goterators.Filter(dag._links_, func(link Link) bool {
		return link.source.Equal(vertex)
	})
	targetVertices := []*Node{}
	goterators.ForEach(edges, func(edge Link) {
		targetVertices = append(targetVertices, edge.target)
	})
	return targetVertices
}

func (dag *Dag) logLast(pref string, node *Node, depth int) {
	//nodes, _ := getDescendants(dag._dag_, dag._links_, node)
	nodes := dag.getAdjEdges(node)
	if len(nodes) == 2 {
		// left_node := fmt.Sprintf("%03d.%d|cW:%04f,tW:%04f", nodes[0].id.idMajor, nodes[0].id.idMinor, nodes[0].cumWeight, nodes[0].txWeight)
		// right_node := fmt.Sprintf("%03d.%d|cW:%04f,tW:%04f", nodes[1].id.idMajor, nodes[1].id.idMinor, nodes[1].cumWeight, nodes[1].txWeight)
		// this_node := fmt.Sprintf("%03d.%d|cW:%04f,tW:%04f", node.id.idMajor, node.id.idMinor, node.cumWeight, node.txWeight)
		utils.ColorizeInfo(logger,
			"\n\n%s [%03d.%d] <<-- [%03d.%d] -->> [%03d.%d]\n",
			pref,
			nodes[0].id.idMajor, nodes[0].id.idMinor,
			node.id.idMajor, node.id.idMinor,
			nodes[1].id.idMajor, nodes[1].id.idMinor,
		)
	} else {
		goterators.ForEach(nodes, func(v *Node) {
			utils.ColorizeInfo(logger,
				//"\n\n%s[%03d.%d|cW:%04f,tW:%04f] <<-- [%03d.%d|cW:%04f,tW:%04f]\n",
				"\n\n%s[%03d.%d] <<-- [%03d.%d]\n",
				pref,
				v.id.idMajor, v.id.idMinor, node.id.idMajor, node.id.idMinor,
			)
		})
	}
}

func isNodeTip(links []Link, node *Node, approvetx int) bool {
	if len(links) == 0 {
		return true
	}

	return tipCache().isTip(node.id.id)
	// count := 0

	// for _, link := range links {
	// 	if link.target.Equal(node) {
	// 		count++
	// 		// if two other nodes ref this node as target - this is not a tip
	// 		// @TODO: make this aa config parameter for math discovery
	// 		if count == approvetx {
	// 			return false
	// 		}
	// 	}
	// }
	// Otherwise, if at most 1 ref to a target node - this is a tip
	// return true
}

// @Optimize Performance
// getApprovers - get a slice of *Node(s) approvers for the passed in node
// who approved this node?
func getApprovers(links []Link, node *Node) []*Node {

	if node.sources != nil && len(node.sources) > 0 {
		return node.sources
	}
	// get all the links where the node (candidate) is a target
	// approvee <<--[target]-- approver [source]
	lnks := goterators.Filter(links, func(link Link) bool {
		// we are looking for links where the given node is the target
		//return link.target.Equal(node) - no need to do full node comparison (it's expensive)
		// it would suffice to just compare uuids
		return link.target.id.id == node.id.id

	})
	// get a slice of approvers for the node (candidate)
	apprs := goterators.Map(lnks, func(link Link) *Node {
		return link.source
	})
	return apprs
}

func getTargetLists(nodes []*Node, links []Link) map[uint64][]*Node {
	childrenLists := map[uint64][]*Node{}
	// go over all links and for each source tx node add a target tx node
	for _, link := range links {
		childrenLists[link.source.id.idMajor] = append(childrenLists[link.source.id.idMajor], link.target)
	}
	return childrenLists
}

func (d *Dag) SyncUp() {
	if len(d._dag_[1:]) == int(d.width) || (len(d._dag_[1:]) > int(d.width) && !__leaderReady__.Load()) {
		// when we reach this condition we form yet another pin tx with exodus sites
		// exodus sites are sites that directly link to genesis
		genPinTx()
		logger.Infof(" %s LEADER %s READY", emoji.Pushpin, emoji.CheckMarkButton)
		// allow sync to run on the leader peer
		__leaderReady__.Store(true)
	}
}

// reconcileMissingTargets - attemp to re-link targets for sites that were added
// without target links. This happens when peer is out of sync with the network
// and gradually attempts to re-build its own copy of dag
func (d *Dag) ReconcileMissingTargets() {
	d.mux.Lock()
	defer d.mux.Unlock()

	// find all Vertices(nodes) that have missingTargets as not nil
	// and see if we can link them to the sites in our copy of dag
	incompleteVertices := goterators.Filter(d._dag_, func(v *Node) bool {
		return len(v.missingTargets) > 0
	})

	requestVertices := []string{}
	// now, that we have the incomplete vertices, find target vertices
	// to link to
	goterators.ForEach(incompleteVertices, func(v1 *Node) {
		// go over each incomplete vertex and find vertices we can link to
		possibleTargets := goterators.Filter(d._dag_, func(v2 *Node) bool {
			// skip our copy
			if v1.id.id == v2.id.id {
				return false
			}
			if _, ok := v1.missingTargets[v2.id.id.String()]; ok {
				// this is a vertex we can link
				return true
			}
			return false
		})
		goterators.ForEach(possibleTargets, func(v3 *Node) {
			if _, ok := v1.missingTargets[v3.id.id.String()]; ok {
				v1.targets = append(v1.targets, v3)
				d._links_ = append(d._links_, Link{
					source: v1,
					target: v3,
				})
				delete(v1.missingTargets, v3.id.id.String())
			}
		})
		if len(v1.missingTargets) == 0 {
			v1.missingTargets = nil
			// Now that its approval targets are linked, this site can take part
			// in confirmation. Without this it stayed invisible to the tracker
			// and could never be confirmed.
			if confirmationCounter != nil {
				confirmationCounter.relink(v1)
			}
		} else {
			for k := range v1.missingTargets {
				requestVertices = append(requestVertices, k)
			}
		}
	})
	if len(requestVertices) > 0 {
		// request missing sites from other nodes
		requestMissingVertices(requestVertices)
	}
}

func requestMissingVertices(vertexIds []string) {
	logger.Infof("Request missing sites with IDs: %q", vertexIds)
	err := transactSiteRequest(vertexIds)
	if err != nil {
		logger.Errorf("Request missing sites - err:%s", err.Error())
	}
}

func (d *Dag) getVertices(ids []string) ([]*Node, error) {
	vIds := goterators.Map(ids, func(id string) uuid.UUID {
		mapId, _ := uuid.Parse(id)
		return mapId
	})
	var err error = nil
	vertices := []*Node{}
	goterators.ForEach(vIds, func(id uuid.UUID) {
		n := d.Vertex(id)
		if n == nil {
			logger.Warnf("Site with id: %s is missing", id.String())
		} else {
			vertices = append(vertices, n)
		}
	})
	if len(ids) > len(vertices) {
		err = errors.Errorf("Not all sites have been found")
	}
	return vertices, err
}
