package dag

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"math/big"
	"time"

	"github.com/VG-Grape/luna/tx"
	"github.com/VG-Grape/luna/tx/pb"
	"github.com/VG-Grape/luna/crypto"
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
	// [earlier nodes] <==targets== [this node] <==sources== [later nodes]
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

func NewDagNode(tx tx.Transaction, verifyRequired bool) *Node {
	_dag_.prevMajor++
	n := &Node{
		id: NodeID{
			id:      uuid.New(),
			address: "",
			idMajor: _dag_.prevMajor,
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
	// add to wallet cache
	subInt := big.NewInt(0).SetBytes(n.tx.GetAmount().Bytes())
	if bytes.Compare(n.tx.GetSender(), n.tx.GetRecipient()) != 0 {
		err := walletCache.sub(luna1crypto.BytesToAddress(n.tx.GetSender()), n.id.id.String(), subInt)
		if err != nil {
			logger.Errorf(err.Error())
			lastKnownBalance, err1 := _pins_.unsafe_getBalanceForWallet(luna1crypto.BytesToAddress(n.tx.GetSender())) //walletCache.get(string(n.tx.Sender))
			if err1 != nil {
				logger.Errorf("Cannot locate balance for wallet %s. %s", luna1crypto.BytesToAddress(n.tx.GetSender()), err1.Error())
			} else {
				logger.Errorf("Error subtracting %d from %d in wallet %s in tx %s. %s",
					subInt.Uint64(), lastKnownBalance.Uint64(), luna1crypto.BytesToAddress(n.tx.GetSender()), n.id.id.String(), err.Error())
			}
			return false
		}
		addInt := big.NewInt(0).SetBytes(n.tx.GetAmount().Bytes())
		err = walletCache.add(luna1crypto.BytesToAddress(n.tx.GetRecipient()), n.id.id.String(), addInt)
		if err != nil {
			logger.Errorf("Error adding %d to %s in tx %s. %s", subInt.Uint64(), n.tx.GetRecipient(), n.id.id.String(), err.Error())
			return false
		}
	}
	return true
	// this should only be called in pin tx, not when updating current balances
	// return _pins_.UpdateIfValid(string(n.tx.Sender), big.NewInt(0).SetBytes(n.tx.Amount))
}
