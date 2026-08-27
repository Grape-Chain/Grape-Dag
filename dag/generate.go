package dag

import (
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/Grape-Chain/Grape-Dag/config"
	"github.com/Grape-Chain/Grape-Dag/crypto"
	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/Grape-Chain/Grape-Dag/vm"
	"github.com/Grape-Chain/Grape-Dag/wallet"
	"github.com/ledongthuc/goterators"

	grape_wallet "github.com/Grape-Chain/Grape-Dag/crypto"
	"github.com/google/uuid"
)

const INITIAL_OFFERING string = "10000000000000000000000000000000"

func newDag(dagConfig config.DagConfiguration) *Dag {
	// generate this node's pseudowallet
	//wg := dagWallet // NOTE: each node should have its own permanent wallet
	// ccounter := newConfirmationCounter()
	//dagWallet = grape1crypto.NewWallet() // should serialize and reuse
	genesis_tx := tx.NewGenesisTxv1(chaintype, dagWallet)
	amount, _ := big.NewInt(0).SetString(INITIAL_OFFERING, 10)
	logger.Infof("Set genesis balance=%s for genesis account=%s", amount.String(), dagWallet.WalletAddress())
	genesis_tx.Amount = amount.Bytes()
	genesis_tx.Sign(dagWallet.PrivateKey())
	height := Height{minheight: 0, maxheight: 0}
	genesis := &Node{txWeight: 1, id: NodeID{id: uuid.Must(uuid.Parse("00000000-0000-0000-0000-000000000000"))}, tx: &genesis_tx, height: height}
	genesis.valid = true
	// create pin tx store
	_pins_ = newNodeTxPin()
	dag := []*Node{}

	var inDagPins []*Node = []*Node{}
	mapped_vertices := make(map[uuid.UUID]*Node)
	// A leader with a stored chain recovers it rather than opening a new one.
	if config.GetConfig().Host.Leader && !recoveringChain {
		dag = append(dag, genesis)
		mapped_vertices[genesis.id.id] = genesis
		inDagPins = []*Node{genesis}
		// confirmation counter needs to know about genesis as well
		confirmationCounter.add(genesis)
		// Form our first pinning tx - genesis
		_pins_.set(genesis, dagWallet.WalletAddress())
	}

	__dag__ := &Dag{_dag_: dag,
		_links_:         nil,
		prevMajor:       0,
		prevMinor:       0,
		txCh:            nil, // we set this when we call RunSynchronization (to be able to turn sync off)
		stopCh:          make(chan bool, 1),
		mux:             sync.Mutex{},
		mapped_vertices: mapped_vertices,
		mapped_edges:    make(map[uuid.UUID][]uuid.UUID),
		mu_map:          sync.RWMutex{},
		pins:            inDagPins,
		exodusWallets:   nil,
		genesis:         genesis,
		width:           uint8(dagConfig.Initialwidth), // this param determines the num of direct links to genesis before we
		// start using the random walk algo for linking sites
	}
	// The leader starts with genesis already in the graph; a joining node picks
	// it up from the network. Either way the count reflects what has been added.
	if len(dag) > 0 {
		__dag__.sitesAdded.Store(uint64(len(dag)))
	}

	//	__dag__.addToDag(genesis, []*Node{})
	return __dag__
}

// Everything must start with a genesis peer holding a genesis block
// all other txs will receive their funds from genesis
// Refactor this behavior so that all initial tx from genesis form
// a predefined width of DAG to grow properly
func GenerateWideDag(dagWidth uint8) *Dag {
	// generate this node's pseudowallet
	//wg := dagWallet // NOTE: each node should have its own permanent wallet
	// ccounter := newConfirmationCounter()
	dagWallet = grape1crypto.NewWallet() // should serialize and reuse
	genesis_tx := tx.NewGenesisTxv1(chaintype, dagWallet)
	genesisAmount := big.NewInt(0)
	genesisAmount, _ = genesisAmount.SetString(INITIAL_OFFERING, 10)
	genesis_tx.Amount = genesisAmount.Bytes()
	genesis_tx.Sign(dagWallet.PrivateKey())
	height := Height{minheight: 0, maxheight: 0}
	genesis := &Node{txWeight: 1, id: NodeID{id: uuid.Nil}, tx: &genesis_tx, height: height}
	genesis.valid = true
	confirmationCounter.add(genesis)
	// Form our first pinning tx - genesis
	_pins_.set(genesis, dagWallet.WalletAddress())
	dag := []*Node{}
	dag = append(dag, genesis)
	//ccounter.add(genesis)
	var prevMajor uint64
	var prevMinor uint32
	// adjust the genesis height for the 1st order nodes
	height = Height{1, 1}
	ico, _ := big.NewInt(0).SetString(INITIAL_OFFERING, 10)
	equal_amount := ico.Div(ico, big.NewInt(int64(dagWidth+1)))
	// generate a collection of nodes distributed in time
	exodusWallets := []*grape1crypto.Wallet{}
	// our second pin tx is exodus nodes
	exodusNodes := []*Node{}
	for count := uint8(0); count < dagWidth; count++ {
		tx := tx.NewTxv1(chaintype)
		exodusWallet := vm.GenesisWallets[int(count)%len(vm.GenesisWallets)].GrapeCryptoWallet()
		exodusWallets = append(exodusWallets, exodusWallet)
		tx.GeneratePayment(
			wallet.GenPaymentTransaction(dagWallet, exodusWallet, equal_amount),
			uint8(peerConfig.Network),
		)
		tx.Sign(dagWallet.PrivateKey())

		if err := tx.Verify(); err != nil {
			logger.Fatalf("Failed to verify newly generated tx: %s", err.Error())
		}
		prevMajor++
		node := &Node{
			id: NodeID{
				id:      uuid.MustParse(fmt.Sprintf(UUID_TEMPLATE, count)),
				idMajor: prevMajor,
				idMinor: prevMinor,
			},
			txWeight: 1,
			time:     time.Now(),
			tx:       tx,
			height:   height,
			valid:    true,
		}
		node.GenerateAddress()
		if err := tx.Verify(); err != nil {
			logger.Fatalf("Failed to verify the genesis tx: %s", err.Error())
		}
		confirmationCounter.add(node)
		genesis.sources = append(genesis.sources, node)
		node.targets = append(node.targets, genesis)
		dag = append(dag, node)
		if !node.UpdateBalanceIfValid() {
			panic("Unable to update balance")
		}

		exodusNodes = append(exodusNodes, node)
		//ccounter.add(node)

	}
	// store the node links in this slice
	links := []Link{}
	// traverse dag and create all links as if each node is a new transaction
	goterators.ForEach(dag[1:], func(node *Node) {
		links = append(links, Link{source: node, target: genesis})
	})
	switch dagAlgorithm() {
	case DAG_ALGO_MCMCP.Type():
		// See insert.go: the node slice is already in the order
		// updateCumWeights needs.
		dag = updateCumWeights(dag, links)
	}
	//dag = updateFwdWeights(dag, links)
	// form our second pin tx - exodus nodes
	err := _pins_.add(exodusNodes, []tx.Transaction{})
	if err != nil {
		panic(err)
	}
	return &Dag{_dag_: dag,
		_links_:         links,
		prevMajor:       prevMajor,
		prevMinor:       prevMinor,
		txCh:            nil, // we set this when we call RunSynchronization (to be able to turn sync off)
		stopCh:          make(chan bool, 1),
		mapped_vertices: make(map[uuid.UUID]*Node),
		mapped_edges:    make(map[uuid.UUID][]uuid.UUID),
		pins:            []*Node{genesis},
		exodusWallets:   exodusWallets,
		width:           dagWidth,
	}
}

func GenerateRandomDag(width uint8, height uint32) *Dag {
	d := GenerateWideDag(width)
	for ; height != 0; height-- {
		t := tx.NewTxv1(tx.PRIVATE_TESTNET)
		sw := grape_wallet.NewWallet()
		rw := grape_wallet.NewWallet()
		tt := wallet.NewTransaction(sw.PrivateKey(), sw.PublicKey(), sw.WalletAddress(), rw.WalletAddress(), big.NewInt(1))
		t.GenerateRandom(1000, 1000, tt, uint8(tx.PRIVATE_TESTNET))
		d.AddTxDag(NewDagNode(t, false))
	}
	return d
}
