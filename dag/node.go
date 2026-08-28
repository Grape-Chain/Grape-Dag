package dag

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"math/big"
	"time"

	"github.com/Grape-Chain/Grape-Dag/crypto"
	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"github.com/google/uuid"
	"go.uber.org/atomic"

	"google.golang.org/protobuf/types/known/timestamppb"
)

type NodeID struct {
	id      uuid.UUID
	address string
	idMajor uint64
	idMinor uint32
}

func (x NodeID) String() string {
	buf := bytes.Buffer{}
	buf.WriteString("NodeID")
	buf.WriteString(fmt.Sprintf("\tid:%s", x.id))
	buf.WriteString(fmt.Sprintf("\taddress:%s", x.address))
	buf.WriteString(fmt.Sprintf("\tidMajor:%d", x.idMajor))
	buf.WriteString(fmt.Sprintf("\tidMinor:%d", x.idMinor))
	return buf.String()
}

type Height struct {
	minheight uint64
	maxheight uint64
}

func (x Height) String() string {
	buf := bytes.Buffer{}
	buf.WriteString("Height")
	buf.WriteString(fmt.Sprintf("\tminheight:%d", x.minheight))
	buf.WriteString(fmt.Sprintf("\tmaxheight:%d", x.maxheight))
	return buf.String()
}

type Node struct {
	id             NodeID
	cumWeight      atomic.Float64
	txWeight       float64
	time           time.Time
	valid          bool
	height         Height
	tx             tx.Transaction
	sources        []*Node // nodes in the dag that point to this node
	targets        []*Node // nodes in the dag this node points to
	missingTargets map[string]bool
	// slicedTargets - approvals this site made whose sites have since been
	// settled into a slice. The edge pointer is dropped so the settled site can
	// be collected, but the id is kept so the approval is still reported.
	slicedTargets []uuid.UUID
	// [earlier nodes] <==targets== [this node] <==sources== [later nodes]

	// Processor attribution - the node that encapsulated this site's transaction
	// into the site, and its signature over the site's identity. See
	// attribution.go. Empty on a site from a peer built before attribution
	// existed, which is a valid site that simply earns nobody a fee.
	processorAddress []byte
	processorPk      []byte
	processorSig     []byte
}

func (x *Node) String() string {
	buf := bytes.Buffer{}
	buf.WriteString("Node")
	buf.WriteString(fmt.Sprintf("\tid: %s", x.id.String()))
	buf.WriteString(fmt.Sprintf("\tcumWeight: %f", x.cumWeight.Load()))
	buf.WriteString(fmt.Sprintf("\ttxWeight: %f", x.txWeight))
	buf.WriteString(fmt.Sprintf("\ttime: %s", x.time.Local()))
	buf.WriteString(fmt.Sprintf("\tvalid: %t", x.valid))
	buf.WriteString(fmt.Sprintf("\theight: %s", x.height.String()))
	buf.WriteString(fmt.Sprintf("\ttx: %s", x.tx.String()))
	buf.WriteString(fmt.Sprintf("\tmissingTargets: %v", x.missingTargets))

	return buf.String()
}

func (n *Node) ToPbNode() *pb.Node {
	pbn := &pb.Node{}
	pbn.CumWeight = float32(n.cumWeight.Load())
	pbn.Height = &pb.Node_Height{
		Minheight: n.height.minheight,
		Maxheight: n.height.maxheight,
	}
	id, _ := n.id.id.MarshalBinary()
	pbn.Id = &pb.Node_NodeId{
		Id:      id,
		IdMajor: n.id.idMajor,
		IdMinor: n.id.idMinor,
		Address: n.id.address,
	}
	pbn.Time = timestamppb.New(n.time)
	pbn.Tx = n.tx.MarshalBinary()
	pbn.TxWeight = float32(n.txWeight)
	pbn.Valid = n.valid
	// when exchanging nodes across peers, indicate that
	// additional sites can be requested via missing targets
	pbn.MissingTargets = make(map[string]bool)
	for _, v := range n.targets {
		pbn.MissingTargets[v.id.id.String()] = true
	}
	// Approvals whose sites have been settled still have to be reported, or a
	// peer rebuilding this site's edges would come up short.
	for _, id := range n.slicedTargets {
		pbn.MissingTargets[id.String()] = true
	}
	// Attribution travels as-is. Copied out rather than shared so a peer
	// serialising a site cannot be handed a slice that aliases the live site.
	pbn.ProcessorAddress = append([]byte(nil), n.processorAddress...)
	pbn.ProcessorPk = append([]byte(nil), n.processorPk...)
	pbn.ProcessorSig = append([]byte(nil), n.processorSig...)
	return pbn
}

func (n *Node) FromPbNode(pbn *pb.Node) {
	n.cumWeight.Store(float64(pbn.CumWeight))
	n.height = Height{
		minheight: pbn.Height.Minheight,
		maxheight: pbn.Height.Maxheight,
	}
	id := &uuid.UUID{}
	id.UnmarshalBinary(pbn.Id.Id)
	n.id = NodeID{
		id:      *id,
		idMajor: pbn.Id.IdMajor,
		idMinor: pbn.Id.IdMinor,
		address: pbn.Id.Address,
	}
	n.time = pbn.Time.AsTime()
	n.tx = tx.UnmarshalBinary(pbn.Tx)

	n.txWeight = float64(pbn.TxWeight)
	n.valid = pbn.Valid
	n.missingTargets = pbn.MissingTargets
	// Taken verbatim, including absent. A site from a peer that predates
	// attribution leaves all three nil, which verifyProcessor reports as
	// unattributed rather than invalid.
	n.processorAddress = pbn.ProcessorAddress
	n.processorPk = pbn.ProcessorPk
	n.processorSig = pbn.ProcessorSig
}

func (n *Node) GetMinHeight() uint64 {
	return n.height.minheight
}

func (n *Node) GetMaxHeight() uint64 {
	return n.height.maxheight
}

func (n *Node) GenerateAddress() {
	n.id.address = base64.StdEncoding.EncodeToString(n.tx.GetSignature())
}

func (n *Node) IsVerified() bool {
	return n.valid
}

func (n *Node) Id() string {
	return n.id.id.String()
}

func (n *Node) Address() string {
	return n.id.address
}

func (n *Node) NodeToVertex() *tx.Vertex {
	return &tx.Vertex{
		Id: tx.VertexId{
			Id:      n.id.id,
			Address: n.id.address,
			IdMajor: n.id.idMajor,
			IdMinor: n.id.idMinor,
		},
		CumWeight: n.cumWeight,
		TxWeight:  n.txWeight,
		Timestamp: n.time,
		Tx:        n.tx,
	}
}

// NewDagNode - wrap a transaction in a new site.
//
// The major id is taken with one atomic add rather than an increment followed by
// a read. Two goroutines call this concurrently - the publisher for a
// transaction this node accepted, and the subscriber for one a peer announced -
// so the two-statement version was both a data race and a way for two sites to
// end up with the same major id.
func NewDagNode(tx tx.Transaction, verifyRequired bool) *Node {
	if _dag_ == nil {
		logger.Errorf("[NewDagNode] No graph to add a site to yet")
		return nil
	}
	n := &Node{
		id: NodeID{
			id:      uuid.New(),
			address: "",
			idMajor: _dag_.prevMajor.Add(1),
			idMinor: _dag_.prevMinor,
		},
		cumWeight: *atomic.NewFloat64(0),
		txWeight:  genRandomTxWeight(),
		time:      time.Now(),
		tx:        tx,
	}
	if int(tx.GetChainType()) != peerConfig.Network {
		logger.Errorf(" Tx %s came from a different network. We are on: %d", tx.String(), peerConfig.Network)
		return nil
	}
	if verifyRequired {
		if err := tx.VerifySignature(); err != nil {
			return nil
		} else {
			n.valid = true
		}
	} else {
		n.valid = true
	}
	n.GenerateAddress()
	return n
}

func (n *Node) GetID() uuid.UUID {
	if n != nil {
		return n.id.id
	}
	return uuid.Nil
}

// UpdateBalanceIfValid - if a transaction can be processed without making the balance negative,
// it will also update the wallet cache which keeps track of the balances for all wallets
// in the current dag slice
func (n *Node) UpdateBalanceIfValid() bool {
	// Escrow: the sender pays the amount and the fee, the recipient receives the
	// amount, and the difference is what the commit transaction divides between
	// the processors that settled it (see dag/rewardbuild.go).
	//
	// Debiting only the amount would pay rewards out of money nobody had paid
	// in, so the supply would grow by the fee on every payment. The fee is
	// nought until fees are switched on, so this is the same arithmetic it has
	// always been until then.
	subInt := new(big.Int).Add(n.tx.GetAmount(), settledFee(n))
	if bytes.Compare(n.tx.GetSender(), n.tx.GetRecipient()) != 0 {
		err := walletCache.sub(grape1crypto.BytesToAddress(n.tx.GetSender()), n.id.id.String(), subInt)
		if err != nil {
			logger.Errorf(err.Error())
			lastKnownBalance, err1 := _pins_.unsafe_getBalanceForWallet(grape1crypto.BytesToAddress(n.tx.GetSender())) //walletCache.get(string(n.tx.Sender))
			if err1 != nil {
				logger.Errorf("Cannot locate balance for wallet %s. %s", grape1crypto.BytesToAddress(n.tx.GetSender()), err1.Error())
			} else {
				logger.Errorf("Error subtracting %d from %d in wallet %s in tx %s. %s",
					subInt.Uint64(), lastKnownBalance.Uint64(), grape1crypto.BytesToAddress(n.tx.GetSender()), n.id.id.String(), err.Error())
			}
			return false
		}
		// The amount only: the recipient does not receive the fee, and does not
		// pay it either.
		addInt := big.NewInt(0).SetBytes(n.tx.GetAmount().Bytes())
		err = walletCache.add(grape1crypto.BytesToAddress(n.tx.GetRecipient()), n.id.id.String(), addInt)
		if err != nil {
			logger.Errorf("Error adding %d to %s in tx %s. %s", subInt.Uint64(), n.tx.GetRecipient(), n.id.id.String(), err.Error())
			return false
		}
	}
	return true
	// this should only be called in pin tx, not when updating current balances
	// return _pins_.UpdateIfValid(string(n.tx.Sender), big.NewInt(0).SetBytes(n.tx.Amount))
}
