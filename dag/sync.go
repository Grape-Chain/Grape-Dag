package dag

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Grape-Chain/Grape-Dag/config"
	"github.com/Grape-Chain/Grape-Dag/discovery"
	"github.com/Grape-Chain/Grape-Dag/graph"
	"github.com/Grape-Chain/Grape-Dag/graph/iface"
	grapepeer "github.com/Grape-Chain/Grape-Dag/peer"
	txqueue "github.com/Grape-Chain/Grape-Dag/queues"
	"github.com/Grape-Chain/Grape-Dag/smc"
	sm "github.com/Grape-Chain/Grape-Dag/statemachine"
	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"github.com/Grape-Chain/Grape-Dag/utils"
	"github.com/enescakir/emoji"
	"github.com/golang-collections/collections/set"
	"github.com/golang-collections/collections/stack"
	"github.com/google/uuid"
	"github.com/ledongthuc/goterators"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type TxVL struct {
	vertex Node
	edges  []Node
}

type DagModifiedIf interface {
	Notify(n TxVL)
}

type DepthHandler struct {
	graph iface.DirectedBuilder
}

// @TODO - add synchronization
var PIN_TX_STACK *stack.Stack = stack.New()

// Receiving transactions and building a search optimized structure
// to determine when to insert a pinning transaction
// Need to negotiate with other peers on which pinning transaction is
// the right transaction by taking votes from all peers in the network
// On each update - see if the depth is suitable to prepare a candidate
// tx
func (h DepthHandler) Notify(txvl TxVL) {
	//logger.Infof("Handling update: %s", tx.vertex.tx.String())
	v := graph.NewVertex(
		iface.Vertex_ID_type(txvl.vertex.id.id),
		txvl.vertex.id.idMajor,
		txvl.vertex.id.idMinor,
	)
	_graph_.AddVertex(v)

	if len(txvl.edges) > 0 {
		goterators.ForEach(txvl.edges, func(e Node) {
			// get id of the vertex to which link this vertex
			// Remember: we are interested in reverse order of edges
			// to support graph traversal from the genesis
			vid := iface.Vertex_ID_type(e.id.id)
			srcVertex := _graph_.Vertex(vid)
			if srcVertex == nil {
				// we are out of sync
				logger.Errorf("[dag notify] DAG is out of sync. Tx %d.%d is missing source site %d.%d",
					txvl.vertex.id.idMajor, txvl.vertex.id.idMinor, e.id.idMajor, e.id.idMinor)
			} else {
				// Need to reverse the order and add the edge to the vertex
				// this vertex is referencing
				srcVertex.AddEdge(
					_graph_.NewEdge(
						vid,
						iface.Vertex_ID_type(txvl.vertex.id.id),
					),
				)
				v.SetDepth(srcVertex.GetDepth() + 1)
			}
		})
	}
	// A site can be in the graph before its transaction is: an approval target
	// named by a site that arrived first is inserted as a placeholder and filled
	// in when the transaction turns up. Asking such a placeholder what kind of
	// transaction it carries dereferences nothing and takes the node down - seen
	// on a validator catching up eleven commit transactions at once, where
	// placeholders are created faster than they are resolved.
	if txvl.vertex.tx == nil {
		return
	}
	if txvl.vertex.tx.GetTransactionType() == tx.SERVICE_PIN || txvl.vertex.tx.GetTransactionType() == tx.SERVICE_GENESIS {
		graph.GetGraphPinStack().Push(txvl.vertex.id.id)
		if graph.GetGraphPinStack().Len() > 1 {
			_graph_.Visualize()
		}
	}
}

var dag_modified_handlers []DagModifiedIf

var _graph_ iface.DirectedBuilder

func init() {
	_graph_ = graph.NewGraph()
	dag_modified_handlers = append(dag_modified_handlers, DepthHandler{graph: _graph_})
}

func syncUp() {
	vertices := GetDag()._dag_
	edges := GetDag()._links_

	fn := func(id uuid.UUID) []Node {
		i := 0
		e := []Node{}
		if id == uuid.Nil {
			return e
		}
		for i < len(edges) {
			l, idx, err := goterators.Find(edges[i:], func(v Link) bool {
				return v.source.id.id == id
			})
			if err == nil && idx >= 0 {
				i = idx + 1
				e = append(e, *l.target)
			} else {
				break
			}
		}
		return e
	}

	goterators.ForEach(vertices, func(v *Node) {
		edges := fn(v.id.id)
		dag_modified_handlers[0].Notify(TxVL{vertex: *v, edges: edges})
	})
}

func pinMerkelHash(_ids []*pb.SiteID) []byte {
	buf := []string{}
	goterators.ForEach(_ids, func(id *pb.SiteID) {
		zid, _ := uuid.FromBytes(id.Id)
		buf = append(buf, zid.String())
	})
	sort.Strings(buf)
	prevHash := md5.Sum(make([]byte, 16))
	goterators.ForEach(buf, func(id string) {
		bbytes := []byte{}
		bbytes = append(bbytes, prevHash[:]...)
		bbytes = append(bbytes, []byte(id)...)
		prevHash = md5.Sum(bbytes)
	})
	return prevHash[:]
}

func pinTxIDs(tips *set.Set) []uuid.UUID {
	dag_tips := []uuid.UUID{}
	tips.Do(func(v interface{}) {
		id := uuid.UUID(v.(iface.Vertex).Id())
		dag_tips = append(dag_tips, id)
	})
	tx_ids := _dag_.traverseDagSlice(dag_tips)
	return tx_ids
}

func updateGraph() {
	_dag_.updateMappedVertices()
	_dag_.updateMappedEdges()

	for _, vv := range _dag_.mapped_vertices {
		// for the vertex vv we have two edges which point to target nodes
		// let's reverse the order so that the graph edges point from the
		// target to the source
		v := graph.NewVertex(
			iface.Vertex_ID_type(vv.id.id),
			vv.id.idMajor,
			vv.id.idMinor,
		)
		_graph_.AddVertex(v)
		// let's locate one or two previous vertices that point to this vertex

		// find links where v is the target
		es := _dag_.mapped_edges[vv.id.id]
		for _, e := range es {
			// each edge points to a parent vertex
			ev := _dag_.mapped_vertices[e]
			// if ev is not in _graph_ yet, add it
			v1 := _graph_.Vertex(iface.Vertex_ID_type(ev.id.id))
			if v1 == nil {
				_graph_.AddVertex(
					graph.NewVertex(
						iface.Vertex_ID_type(v1.Id()),
						v1.Major(),
						v1.Minor(),
					),
				)
			}
			// from v1 to vv edge
			v1.AddEdge(
				_graph_.NewEdge(
					v1.Id(),
					iface.Vertex_ID_type(vv.id.id),
				),
			)
			v.SetDepth(v1.GetDepth() + 1)
		}
	}
}

// announceLatestPin - wrap the latest known pin tx in a sync packet and send it over a sync channel
// returns:
//
//	error
func announceLatestPin() error {
	logger.Infof("%s  ~ Announcing latest pin tx %s ...", emoji.Loudspeaker, emoji.RoundPushpin)
	return transactLatestPin(uuid.Nil, tx.CURRENT, tx.STX_ANNOUNCE)
}

func announceNewPin() error {
	logger.Info("Announce new pin tx")
	return transactLatestPin(uuid.Nil, tx.LATEST, tx.STX_ANNOUNCE)
}

func respondLatestPin(rec *tx.Syncv1) error {
	logger.Info("Respond with the latest pin tx")
	return transactLatestPin(rec.Tracking_Id, tx.LATEST, tx.STX_RESPONSE)
}

func respondLatestBalances(rec *tx.Syncv1) error {
	logger.Info("Respond with the latest Balance Snapshot, id=%s", rec.Tracking_Id.String())
	host := grapepeer.GetHost()
	hostpk := host.Peerstore().PrivKey(host.ID())
	stdkey, _ := crypto.PrivKeyToStdKey(hostpk)
	pk, pubkey, err := crypto.KeyPairFromStdKey(stdkey)
	if err != nil {
		return fmt.Errorf("failed to obtain pubkey. %s", err.Error())
	}
	p := GetPin()
	latest_pin := _pins_.getLast()
	if latest_pin == nil {
		logger.Warnf("%s  ~ no pinning tx exists. Nothing to return as balances snapshot", emoji.RoundPushpin)
		return errors.New("leader hasn't been initialized yet with at least one pin tx. no balances exist at the moment")
	}

	stx := tx.NewSyncv1()
	stx.Ver_type = tx.STVX1
	stx.Sync_Type = tx.CURRENT
	stx.Msg_Type = tx.STX_SNAPSHOT_BALANCE_RESPONSE
	stx.Tracking_Id = rec.Tracking_Id
	stx.Data, _ = latest_pin.MarshalBinary()

	currentHeight := latest_pin.PinNumber
	pbHeight, _ := anypb.New(wrapperspb.Int64(currentHeight))
	stx.Details = append(stx.Details, pbHeight)
	// long running operation
	balances, _ := p.GetPinnedBalances(currentHeight)
	pbLength, _ := anypb.New(wrapperspb.Int32(int32(len(balances))))
	stx.Details = append(stx.Details, pbLength)
	stx.Data, _ = proto.Marshal(latest_pin)
	logger.Infof("Balance Snapshot: total balances: %d, current height: %d, id=%s", len(balances), currentHeight, rec.Tracking_Id.String())
	for wallet, balance := range balances {
		pbWallet, _ := anypb.New(wrapperspb.String(wallet))
		stx.Details = append(stx.Details, pbWallet)
		pbBalance, _ := anypb.New(wrapperspb.Bytes(balance))
		stx.Details = append(stx.Details, pbBalance)
	}

	stx.Sender_Pubk, err = crypto.MarshalPublicKey(pubkey)
	if err != nil {
		return fmt.Errorf("error marshaling public key: %s", err.Error())
	}
	stx.SyncHash = []byte{}

	stx.Signature = stx.GenerateSignature(pk)

	txqueue.GetSyncQueue().Enqueue(stx)
	_, err = syncsm.waitForSMInLoop(stx.Tracking_Id, sm.SYNC_DISPATCH_END, time.Second*25)
	for err != nil { // ensure response is published
		logger.Warnf("Trying to place Balances Snapshot into SyncQueue, err=%s, id=%s", err.Error(), rec.Tracking_Id.String())
		syncsm.resetSM(stx.Tracking_Id)
		txqueue.GetSyncQueue().Enqueue(stx)
		_, err = syncsm.waitForSMInLoop(stx.Tracking_Id, sm.SYNC_DISPATCH_END, time.Second*25)
	}
	logger.Infof("Balances Snapshot responded to Sync Queue, id=%s", rec.Tracking_Id.String())

	return nil
}

func handleLatestBalances(rec *tx.Syncv1) error {
	logger.Info("Handle latest Balance Snapshot received from leader, id=%s", rec.Tracking_Id.String())
	p := GetPin()
	p.LockPin()
	defer p.UnlockPin()
	if p.ready {
		logger.Warnf("Latest Balance snapshot from Leader was already applied, id=%s", rec.Tracking_Id.String())
		return nil
	}
	latestPinFromLeader := pb.TxPin{}
	err := proto.Unmarshal(rec.Data, &latestPinFromLeader)
	if err != nil {
		return errors.New("unmarshaling latest pin from leader withing BalanceSnapshot response: " + err.Error())
	}
	p.unsafe_appendPin(&latestPinFromLeader)

	chMsg, _ := rec.Details[0].UnmarshalNew()
	chIntValue := chMsg.(*wrapperspb.Int64Value)
	currentHeightAnnounced := chIntValue.GetValue()
	logger.Infof("Leader's snapshot height is %d, id=%s", currentHeightAnnounced, rec.Tracking_Id.String())
	// p.LockPin() is held for the duration of this handler, so use the
	// lock-free accessor here.
	if p.unsafe_currentHeight() != int(currentHeightAnnounced) {
		return fmt.Errorf("announced and current height mismatch"+
			"on node when applying balances snapshot: current: %d, announced: %d", p.unsafe_currentHeight(), currentHeightAnnounced)
	}
	lMsg, _ := rec.Details[1].UnmarshalNew()
	lIntValue := lMsg.(*wrapperspb.Int32Value)
	balanceLength := int(lIntValue.GetValue())
	offset := 2
	logger.Infof("Sync %d balances on peer, id=%s", balanceLength, rec.Tracking_Id.String())
	for i := 0; i < balanceLength; i++ {
		wMsg, _ := rec.Details[offset].UnmarshalNew()
		wStringValue := wMsg.(*wrapperspb.StringValue)
		walletAddress := wStringValue.GetValue()
		offset++
		bMsg, _ := rec.Details[offset].UnmarshalNew()
		bBytesValue := bMsg.(*wrapperspb.BytesValue)
		balanceBytes := bBytesValue.GetValue()
		offset++
		logger.Infof("Set balance for %s as %d from leader's snapshot, id=%s", walletAddress, big.NewInt(0).SetBytes(balanceBytes).String(), rec.Tracking_Id.String())
		latestPinFromLeader.Balance.Balance[walletAddress] = balanceBytes
	}
	p.ready = true
	// This pin now carries the leader's full balance snapshot, which is this
	// node's opening statement of the ledger. Record it, so a restart resumes
	// from here instead of seeding from a later pin that only states the
	// balances it happened to touch.
	chainStartCommitted(&latestPinFromLeader)
	logger.Infof("Sync of %d balances from leader's snapshot has been finished, id=%s", balanceLength, rec.Tracking_Id.String())
	return nil
}

func respondAllPinsFrom(rec *tx.Syncv1) error {
	logger.Infof("Respond with all pins from height, id=%s", rec.Tracking_Id.String())
	host := grapepeer.GetHost()
	hostpk := host.Peerstore().PrivKey(host.ID())
	stdkey, _ := crypto.PrivKeyToStdKey(hostpk)
	pk, pubkey, err := crypto.KeyPairFromStdKey(stdkey)
	if err != nil {
		return fmt.Errorf("failed to obtain pubkey. %s", err.Error())
	}
	p := GetPin()

	stx := tx.NewSyncv1()
	stx.Ver_type = tx.STVX1
	stx.Sync_Type = tx.CURRENT
	stx.Msg_Type = tx.STX_PIN_DOWNLOAD_RESPONSE
	stx.Tracking_Id = rec.Tracking_Id

	fromMsg, _ := rec.Details[0].UnmarshalNew()
	fromIntValue := fromMsg.(*wrapperspb.Int32Value)
	from := fromIntValue.GetValue()
	allPins := p.getAllFrom(int(from))
	// One message, so it has to fit in one message. A commit transaction under
	// a validator quorum carries every transaction it settles, which at load is
	// megabytes each; nine of them went over the pubsub limit and were dropped
	// in silence, and the node that asked timed out and stayed behind for ever.
	// Send what fits and let the requester ask again from where this batch left
	// it - the catch-up already re-triggers on the next announcement.
	batch, sent := pinsThatFit(allPins, catchUpBudget())
	logger.Infof("Found %d pins from height %d, sending %d of them (%d bytes), id=%s",
		len(allPins), from, len(batch), sent, rec.Tracking_Id.String())
	pbLength, _ := anypb.New(wrapperspb.Int32(int32(len(batch))))
	stx.Details = append(stx.Details, pbLength)
	for _, pin := range batch {
		pinBytes, _ := pin.MarshalBinary()
		pinBytesWrapper, _ := anypb.New(wrapperspb.Bytes(pinBytes))
		stx.Details = append(stx.Details, pinBytesWrapper)
	}

	stx.Sender_Pubk, err = crypto.MarshalPublicKey(pubkey)
	if err != nil {
		return fmt.Errorf("error marshaling public key: %s", err.Error())
	}
	stx.Data = []byte{}
	stx.SyncHash = []byte{}

	stx.Signature = stx.GenerateSignature(pk)

	txqueue.GetSyncQueue().Enqueue(stx)
	_, err = syncsm.waitForSMInLoop(stx.Tracking_Id, sm.SYNC_DISPATCH_END, time.Second*25)
	for err != nil { // ensure response is published
		logger.Warnf("Trying to place AllPinsFromHeight into SyncQueue, err=%s, id=%s", err.Error(), rec.Tracking_Id.String())
		syncsm.resetSM(stx.Tracking_Id)
		txqueue.GetSyncQueue().Enqueue(stx)
		_, err = syncsm.waitForSMInLoop(stx.Tracking_Id, sm.SYNC_DISPATCH_END, time.Second*25)
	}
	logger.Infof("AllPinsFromHeight response was successfully sent back to calling peer, id=%s", rec.Tracking_Id.String())

	return nil
}

func handleDownloadedPinsFromLeader(rec *tx.Syncv1) error {
	logger.Infof("[Gap detected] Handle pins received from leader, id=%s", rec.Tracking_Id.String())
	defer GetPin().ClosePinDownloading()
	lMsg, _ := rec.Details[0].UnmarshalNew()
	lIntValue := lMsg.(*wrapperspb.Int32Value)
	amountOfPins := lIntValue.GetValue()
	logger.Infof("Leader returned %d pins", amountOfPins)

	// Details is [count, pin, pin, ...]: walk it, rather than re-reading the
	// first entry for every pin, which delivered N copies of the same pin and
	// left the receiver one pin further along per round trip at best.
	for i := 0; i < int(amountOfPins); i++ {
		offset := i + 1
		if offset >= len(rec.Details) {
			return fmt.Errorf("leader announced %d pins but sent %d", amountOfPins, len(rec.Details)-1)
		}
		pMsg, err := rec.Details[offset].UnmarshalNew()
		if err != nil {
			return fmt.Errorf("unwrapping pin %d of %d from leader: %s", i+1, amountOfPins, err.Error())
		}
		pBytesValue, ok := pMsg.(*wrapperspb.BytesValue)
		if !ok {
			return fmt.Errorf("pin %d of %d from leader has unexpected type %T", i+1, amountOfPins, pMsg)
		}
		pin := pb.TxPin{}
		if err := proto.Unmarshal(pBytesValue.GetValue(), &pin); err != nil {
			return fmt.Errorf("unmarshaling received pin from leader: %s", err.Error())
		}
		logger.Infof("Received pin=%d from Leader", pin.PinNumber)
		GetPin().AddDownloadedPin(&pin)
	}
	logger.Infof("[Gap detected] Pins received from leader have been successfully added for further processing, id=%s", rec.Tracking_Id.String())

	return nil
}

func transactMissingPinRequest(sign []byte) error {
	logger.Infof("%s  ~ Requesting pin tx  %s ->", emoji.IncomingEnvelope, hex.EncodeToString(sign)[:10])
	host := grapepeer.GetHost()
	hostpk := host.Peerstore().PrivKey(host.ID())
	stdkey, _ := crypto.PrivKeyToStdKey(hostpk)
	pk, pubkey, err := crypto.KeyPairFromStdKey(stdkey)
	if err != nil {
		return fmt.Errorf("failed to obtain pubkey. %v+", err)
	}

	// prepare for announcement the latest pin tx
	stx := tx.NewSyncv1()
	stx.Ver_type = tx.STVX1
	stx.Sync_Type = tx.MISSING
	stx.Msg_Type = tx.STX_PIN_REQUEST
	// prepare a state machine for when we receive a responce
	syncsm.resetSM(stx.Tracking_Id)
	defer syncsm.deleteSM(stx.Tracking_Id)
	stx.Sender_Pubk, err = crypto.MarshalPublicKey(pubkey)
	if err != nil {
		return fmt.Errorf("error marshaling public key: %s", err.Error())
	}
	stx.Data = append(stx.Data, sign...)
	payload, _ := proto.Marshal(stx.MarshalBinary())
	stx.SyncHash = sha256.New().Sum(payload)
	// Note: signature on payload only
	stx.Signature = stx.GenerateSignature(pk)
	// Prepare a state machine to handle our sync sequence
	syncsm.changeToSM(stx.Tracking_Id, sm.SYNC_DISPATCH_BEGIN)
	// we publish sync tx to a sync queue
	txqueue.GetSyncQueue().Enqueue(stx)
	syncsm.waitForSM(stx.Tracking_Id, sm.SYNC_DISPATCH_END, config.STATE_CHANGE_WAIT)
	return nil
}

func sendPindDownloadRequest(fromHeight int) error {

	host := grapepeer.GetHost()
	hostpk := host.Peerstore().PrivKey(host.ID())
	stdkey, _ := crypto.PrivKeyToStdKey(hostpk)
	pk, pubkey, err := crypto.KeyPairFromStdKey(stdkey)
	if err != nil {
		return fmt.Errorf("failed to obtain pubkey. %v+", err)
	}

	stx := tx.NewSyncv1()
	stx.Ver_type = tx.STVX1
	stx.Sync_Type = tx.CURRENT
	stx.Msg_Type = tx.STX_PIN_DOWNLOAD_REQUEST
	logger.Infof("Going to send PinDownloadRequest fromHeight=%d, id=%s", fromHeight, stx.Tracking_Id.String())
	// prepare a state machine for when we receive a responce
	syncsm.resetSM(stx.Tracking_Id)

	stx.Sender_Pubk, err = crypto.MarshalPublicKey(pubkey)
	if err != nil {
		return fmt.Errorf("error marshaling public key: %s", err.Error())
	}
	stx.Data = []byte{}
	pbFrom, _ := anypb.New(wrapperspb.Int32(int32(fromHeight)))
	stx.Details = append(stx.Details, pbFrom)
	payload, _ := proto.Marshal(stx.MarshalBinary())
	stx.SyncHash = sha256.New().Sum(payload)
	// Note: signature on payload only
	stx.Signature = stx.GenerateSignature(pk)
	// Prepare a state machine to handle our sync sequence
	syncsm.changeToSM(stx.Tracking_Id, sm.SYNC_DISPATCH_BEGIN)
	// we publish sync tx to a sync queue
	txqueue.GetSyncQueue().Enqueue(stx)
	return nil
}

func sendSnapshotRequest() (error, *uuid.UUID) {
	logger.Info("Sending Balance snapshot request")
	host := grapepeer.GetHost()
	hostpk := host.Peerstore().PrivKey(host.ID())
	stdkey, _ := crypto.PrivKeyToStdKey(hostpk)
	pk, pubkey, err := crypto.KeyPairFromStdKey(stdkey)
	if err != nil {
		return fmt.Errorf("failed to obtain pubkey. %v+", err), nil
	}

	stx := tx.NewSyncv1()
	stx.Ver_type = tx.STVX1
	stx.Sync_Type = tx.CURRENT
	stx.Msg_Type = tx.STX_SNAPSHOT_BALANCE_REQUEST
	logger.Infof("Going to Send Balance snapshot request=%s", stx.Tracking_Id.String())
	// prepare a state machine for when we receive a responce
	syncsm.resetSM(stx.Tracking_Id)

	stx.Sender_Pubk, err = crypto.MarshalPublicKey(pubkey)
	if err != nil {
		return fmt.Errorf("error marshaling public key: %s", err.Error()), nil
	}
	stx.Data = []byte{}
	payload, _ := proto.Marshal(stx.MarshalBinary())
	stx.SyncHash = sha256.New().Sum(payload)
	// Note: signature on payload only
	stx.Signature = stx.GenerateSignature(pk)
	// Prepare a state machine to handle our sync sequence
	syncsm.changeToSM(stx.Tracking_Id, sm.SYNC_DISPATCH_BEGIN)
	// we publish sync tx to a sync queue
	txqueue.GetSyncQueue().Enqueue(stx)
	_, err = syncsm.waitForSMInLoop(stx.Tracking_Id, sm.SYNC_DISPATCH_END, time.Second*25)
	return err, &stx.Tracking_Id
}

// transactSiteRequest - issue a request to provide this peer with a slice of sites
// that are missing from the local copy of dag. Sig: SITE-STX_REQUEST
func transactSiteRequest(sites []string) error {
	logger.Infof("%s  ~ Issue a miss request for %d site(s)", emoji.Receipt, len(sites))
	host := grapepeer.GetHost()
	hostpk := host.Peerstore().PrivKey(host.ID())
	stdkey, _ := crypto.PrivKeyToStdKey(hostpk)
	pk, pubkey, err := crypto.KeyPairFromStdKey(stdkey)
	if err != nil {
		return fmt.Errorf("failed to obtain pubkey. %v+", err)
	}

	// prepare for announcement the latest pin tx
	stx := tx.NewSyncv1()
	stx.Ver_type = tx.STVX1
	stx.Sync_Type = tx.SITE
	stx.Msg_Type = tx.STX_REQUEST
	// prepare a state machine for when we receive a responce
	syncsm.resetSM(stx.Tracking_Id)
	defer syncsm.deleteSM(stx.Tracking_Id)
	stx.Sender_Pubk, err = crypto.MarshalPublicKey(pubkey)
	if err != nil {
		return fmt.Errorf("error marshaling public key: %s", err.Error())
	}

	for _, v := range sites {
		any, _ := anypb.New(wrapperspb.String(v))
		stx.Details = append(stx.Details, any)
	}
	payload, _ := proto.Marshal(stx.MarshalBinary())
	stx.SyncHash = sha256.New().Sum(payload)
	stx.Signature = stx.GenerateSignature(pk)
	// Prepare a state machine to handle our sync sequence
	syncsm.changeToSM(stx.Tracking_Id, sm.SYNC_DISPATCH_BEGIN)
	// we publish sync tx to a sync queue
	txqueue.GetSyncQueue().Enqueue(stx)
	st, err := syncsm.waitForSM(stx.Tracking_Id, sm.SYNC_DISPATCH_END, config.STATE_CHANGE_WAIT)
	if st != sm.SYNC_CANCEL_STATE && err != nil {
		logger.Warnf("%s  ~ Timeout while waiting for SM change to %s", emoji.HourglassDone, sm.SYNC_DISPATCH_END)
	} else if st == sm.SYNC_CANCEL_STATE {
		logger.Warnf("%s  ~ CANCEL Publish on %s|%s %s",
			emoji.CrossMark,
			stx.Msg_Type,
			stx.Sync_Type,
			stx.Tracking_Id,
		)
	}
	return nil
}

// handleSiteRequest - handle the request to obtain the missing sites
// locate and return all vertices(nodes) matching their IDs. Sig: SITE-STX_RESPONSE
func handleSiteRequest(trackingId uuid.UUID, sites []string) error {
	logger.Infof("%s  ~ Processing site request [%s]", emoji.Receipt, trackingId)
	host := grapepeer.GetHost()
	hostpk := host.Peerstore().PrivKey(host.ID())
	stdkey, _ := crypto.PrivKeyToStdKey(hostpk)
	pk, pubkey, err := crypto.KeyPairFromStdKey(stdkey)
	if err != nil {
		return fmt.Errorf("failed to obtain pubkey. %v+", err)
	}

	// grab the latest known pin tx
	vertices, err := GetDag().getVertices(sites)
	if err != nil || len(vertices) == 0 {
		return fmt.Errorf("no such sites exist: %s", sites)
	}
	// prepare for announcement the latest pin tx
	stx := tx.NewSyncv1()
	stx.Ver_type = tx.STVX1
	stx.Sync_Type = tx.SITE
	stx.Msg_Type = tx.STX_RESPONSE
	if trackingId == uuid.Nil {
		trackingId = uuid.New()
	}
	stx.Tracking_Id = trackingId
	// prepare a state machine for when we receive a responce
	syncsm.resetSM(stx.Tracking_Id)
	defer syncsm.deleteSM(stx.Tracking_Id)
	stx.Sender_Pubk, err = crypto.MarshalPublicKey(pubkey)
	if err != nil {
		return fmt.Errorf("error marshaling public key: %s", err.Error())
	}

	for _, v := range vertices {
		any, _ := anypb.New(v.ToPbNode())
		stx.Details = append(stx.Details, any)
	}
	payload, _ := proto.Marshal(stx.MarshalBinary())
	stx.SyncHash = sha256.New().Sum(payload)
	stx.Signature = stx.GenerateSignature(pk)
	// Prepare a state machine to handle our sync sequence
	syncsm.changeToSM(stx.Tracking_Id, sm.SYNC_DISPATCH_BEGIN)
	// we publish sync tx to a sync queue
	txqueue.GetSyncQueue().Enqueue(stx)
	syncsm.waitForSM(stx.Tracking_Id, sm.SYNC_DISPATCH_END, config.STATE_CHANGE_WAIT)
	return nil
}

// handleSyncSiteResponse - process the sync site response
// the Details field contains a slice of Nodes that this peer wants
// to integrate into its local copy of dag
func handleSyncSiteResponse(syncTx *tx.Syncv1) error {
	// logger.Infof("handleSyncSiteResponse [%s]", syncTx.Tracking_Id)
	vertices := []*Node{}
	if syncTx.Details != nil {
		goterators.ForEach(syncTx.Details, func(a *anypb.Any) {
			prMsg, _ := a.UnmarshalNew()
			switch t := prMsg.(type) {
			case *pb.Node:
				x := &Node{}
				x.FromPbNode(t)
				vertices = append(vertices, x)
			}
		})
	}
	_dag_.mux.Lock()
	defer _dag_.mux.Unlock()
	err := _dag_.InsertIfNotExist(vertices)
	return err
}

func transactLatestPin(trackignId uuid.UUID, syncType tx.SyncType, msgType tx.SyncMsgType) error {
	// a new pinning transaction should confirm all tips
	host := grapepeer.GetHost()
	hostpk := host.Peerstore().PrivKey(host.ID())
	stdkey, _ := crypto.PrivKeyToStdKey(hostpk)
	pk, pubkey, err := crypto.KeyPairFromStdKey(stdkey)
	if err != nil {
		return fmt.Errorf("failed to obtain pubkey. %s", err.Error())
	}

	// grab the latest known pin tx
	latest_pin := _pins_.getLast()
	if latest_pin == nil {
		logger.Warnf("%s  ~ no pinning tx exists", emoji.RoundPushpin)
	}
	// prepare for announcement the latest pin tx
	stx := tx.NewSyncv1()
	stx.Ver_type = tx.STVX1
	stx.Sync_Type = syncType
	stx.Msg_Type = msgType
	if trackignId == uuid.Nil {
		trackignId = uuid.New()
	}
	stx.Tracking_Id = trackignId

	// prepare a state machine for when we receive a responce
	syncsm.resetSM(stx.Tracking_Id)
	defer syncsm.deleteSM(stx.Tracking_Id)

	stx.Sender_Pubk, err = crypto.MarshalPublicKey(pubkey)
	if err != nil {
		return fmt.Errorf("error marshaling public key: %s", err.Error())
	}
	if latest_pin != nil {
		stx.Data, err = latest_pin.MarshalBinary()
		if err != nil {
			return fmt.Errorf("error marshaling latest pin tx: %s", err.Error())
		}
		stx.SyncHash = pinMerkelHash(latest_pin.Sites)
	} else {
		stx.Data = []byte{}
		stx.SyncHash = []byte{}
	}
	stx.Signature = stx.GenerateSignature(pk)

	// Prepare a state machine to handle our sync sequence
	syncsm.resetSM(stx.Tracking_Id)
	defer syncsm.deleteSM(stx.Tracking_Id)

	syncsm.changeToSM(stx.Tracking_Id, sm.SYNC_DISPATCH_BEGIN)
	// we publish sync tx to a sync queue
	txqueue.GetSyncQueue().Enqueue(stx)
	syncsm.waitForSM(stx.Tracking_Id, sm.SYNC_DISPATCH_END, 1000)
	return nil
}

type ConnectionListener struct {
	connCh chan bool
}

func (l *ConnectionListener) NotifyConnected(pid peer.ID) {
	// when called - signal that there is a new connection
	l.connCh <- true
}

func (l *ConnectionListener) NotifyDisconnected(pid peer.ID) {
	l.connCh <- false
}

// Returns true if connection has been established
// or false when a connection has been terminated (host disconnected)
func waitForConnection() bool {
	listener := &ConnectionListener{
		connCh: make(chan bool, 1),
	}
	discovery.AddNotifee(listener)
	connFlag := <-listener.connCh
	discovery.RemoveNotifee(listener)
	close(listener.connCh)
	// @TODO - do proper synchronization and allow proper communication establishment
	// Without giving it time to join all topics all messages vanish inthe abbyss
	t := time.NewTimer(time.Second)
	<-t.C

	return connFlag
}

// genPinTx generate the latest pin tx
// @TODO: new tip tracking algorithm for calculating confirmed txs
//
//	a new pin tx is added to _pins_
func genPinTx() {
	sites := _dag_.GetConfirmedSites()
	smcTxs := smc.GetAllUncofirmed(int(config.GetConfig().Tx.Maxfuellimit))
	logger.Debugf("* [PIN] sites: %d, smc: %d", len(sites), len(smcTxs))
	if len(sites) > 0 || len(smcTxs) > 0 {
		logger.Debugf("* [PIN] Num confirmed sites %d. ADD to PinTx", len(sites))
		_pins_.add(sites, smcTxs)
		// Persist and settle the pin just formed, exactly as a receiving node
		// does when it applies the same pin.
		if latest := _pins_.GetLastPin(); latest != nil {
			pinCommitted(latest)
		}
		announceNewPin()
	}
}

// depth_monitor monitors the depth of the dag and calls the notifier
// when the dag depth reaches the desired depth
func (dsm *DagSyncMngr) dag_watcher(leader bool, wait_connect bool, wg *sync.WaitGroup) {
	// sync up with the generated DAG
	syncUp()

	// When this function returns we should have up to date DAG
	// All transactions that arrive during this time are stored in cache
	// and will be reconsiled once we are in sync

	// wait until dag changes - dag itself will notify us of any changes
	// when dag notifies us - we calculate the new depth and if it exceeds
	// the threshold value - fire notifiers
	var _stop_ atomic.Bool = atomic.Bool{}
	syncTicker := time.NewTicker(config.PIN_TX_TIMER_DEF)
	defer syncTicker.Stop()
	// The validator protocol has phases inside the commit interval, so it needs
	// a finer tick than the interval itself. It runs on this goroutine, the same
	// one that forms commit transactions in leader mode.
	consensusTicker := time.NewTicker(consensusTickInterval())
	defer consensusTicker.Stop()
	//var prevMaxDepth uint64 = 0
	wg.Done()

	// Dag watcher thread - assembles a reverse image of DAG
	notifyWg := sync.WaitGroup{}
	notifyWg.Add(1)
	notifyCh := make(chan bool)
	go func() {
		defer notifyWg.Done()
		notifyCh <- true
		for !_stop_.Load() || !dsm.StopFlag.Load() {
			tx := <-_dag_.txCh
			goterators.ForEach(dag_modified_handlers, func(fn DagModifiedIf) {
				fn.Notify(tx)
			})
		}
		logger.Infof("%s  ~ notifier routine stopped", emoji.VerticalTrafficLight)
	}()
	<-notifyCh
	logger.Info("Notify dag modified routine is running")
	close(notifyCh)

	if dsm.Leader {
		announceLatestPin()
		logger.Info("Successfully announced ourselves")
	} else {
		logger.Infof("%s  ~ Syncing up with the network...", emoji.LeftRightArrow)
		if config.GetConfig().Host.WaitConnect {
			waitForConnection()
		}
		syncUpPeer(dsm)
		logger.Infof("%s  ~ %s  < READY >  %s", emoji.CheckBoxWithCheck, emoji.Grapes, emoji.Grapes)
	}
stop_requested:
	for !_stop_.Load() || !dsm.StopFlag.Load() {
		select {
		case <-_dag_.stopCh:
			logger.Infof("%s  ~ Dag Watcher is stopping ...", emoji.HorizontalTrafficLight)
			_stop_.Store(true)
			break stop_requested
		case <-_dag_.depthCh:
			__noop__()
		case <-consensusTicker.C:
			// In quorum mode a commit transaction is what a quorum of validators
			// agreed to, so every validator drives the protocol - not only the
			// leader. Nodes that are not validators have no runner and do
			// nothing here; they apply what the set agrees.
			if consensusActive() {
				consensusRunner.drive()
			}
		case <-syncTicker.C:
			// as per convention, only leaders may generate pin tx
			if leader && __leaderReady__.Load() && !consensusActive() {
				genPinTx()
			}
		}
	}
	logger.Infof("%s  ~ Waiting for notify routine to stop", emoji.Stopwatch)
	notifyWg.Wait()
	logger.Infof("%s  ~ Dag Watcher stopped", emoji.VerticalTrafficLight)
}

type DagSyncSub struct {
	rd         *routing.RoutingDiscovery
	ps         *pubsub.PubSub
	rendezvous string
	topic      *pubsub.Topic
	rcf        pubsub.RelayCancelFunc
	teh        *pubsub.TopicEventHandler
	sub        *pubsub.Subscription
	err        error
}

func (dssub *DagSyncSub) destroy() {
	logger.Infof("%s  ~ Stopping sync pubsub...", emoji.HorizontalTrafficLight)

	if dssub.teh != nil {
		dssub.teh.Cancel()
	}

	if dssub.sub != nil {
		dssub.sub.Cancel()
	}

	if dssub.topic != nil {
		dssub.topic.Close()
		// by now the context has already been cancelled. it's safe to ignore an error if it happens
	}
	logger.Infof("%s  ~ Sync pubsub stopped", emoji.VerticalTrafficLight)
}

func NewDagSyncSub(rd *routing.RoutingDiscovery, ps *pubsub.PubSub, rendezvous string, bs bool) *DagSyncSub {
	dssub := &DagSyncSub{
		rd:         rd,
		ps:         ps,
		rendezvous: rendezvous,
	}
	dssub.topic, dssub.err = ps.Join(rendezvous)
	if dssub.err != nil {
		dssub.destroy()
		return nil
	}
	if !bs {
		logger.Infof("%s  ~ Waiting until DHT sees our peer join topic %s", emoji.HourglassNotDone, rendezvous)
		for {
			if discovery.GetMesh().In(rendezvous, grapepeer.GetHost().ID()) {
				logger.Infof("%s  ~ %s registered for topic: %s", emoji.CheckBoxWithCheck, grapepeer.GetHost().ID(), rendezvous)
				break
			} else {
				logger.Warnf("%s  ~ %s not registered for topic: %s", emoji.CrossMark, grapepeer.GetHost().ID(), rendezvous)
			}
			tm := time.NewTimer(time.Second * 5)
			<-tm.C
			tm.Stop()
		}
		logger.Infof("%s  ~ our peer is in DHT for topic %s", emoji.CheckBoxWithCheck, rendezvous)
	}

	dssub.rcf, dssub.err = dssub.topic.Relay()
	if dssub.err != nil {
		dssub.destroy()
		return nil
	}

	dssub.teh, dssub.err = dssub.topic.EventHandler()
	if dssub.err != nil {
		dssub.destroy()
		return nil
	}

	subopt := []pubsub.SubOpt{pubsub.WithBufferSize(1 << 20)}
	dssub.sub, dssub.err = dssub.topic.Subscribe(subopt...)
	if dssub.err != nil {
		dssub.destroy()
		return nil
	}

	return dssub
}

func DagSync(host host.Host, idht *dht.IpfsDHT, rd *routing.RoutingDiscovery, ps *pubsub.PubSub, rendezvous string, leader bool, bs bool) *DagSyncMngr {
	utils.ColorizeInfo(logger, "[dag sync] Host %s is joining pubsub topic %s", host.ID().String(), rendezvous)

	dssub := NewDagSyncSub(rd, ps, rendezvous, bs)
	if dssub == nil {
		logger.Error("[dag sync] Failed to initialize synchronization pub/sub")
		return nil
	}
	logger.Infof("%s  ~ Waiting for other peers to register with topic %s", emoji.HourglassNotDone, rendezvous)
	for {
		if discovery.GetMesh().In(rendezvous, grapepeer.GetHost().ID()) {
			logger.Infof("%s  ~ %s registered for topic: %s", emoji.CheckBoxWithCheck, grapepeer.GetHost().ID(), rendezvous)
			break
		} else {
			logger.Warnf("%s  ~ %s not registered for topic: %s", emoji.CrossMark, grapepeer.GetHost().ID(), rendezvous)
		}
		time.Sleep(time.Second * 5)
		c := idht.RefreshRoutingTable()
		<-c
	}
	ids := dssub.topic.ListPeers()
	if len(ids) > 1 {
		logger.Infof("%s  ~ there are other peers in %s:", emoji.CheckBoxWithCheck, dssub.topic)
		for _, id := range ids {
			logger.Infof("%s  ~ T:%s ID:%s", emoji.Ledger, dssub.topic.String(), id)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	syncmgr := &DagSyncMngr{
		SyncSub:    dssub,
		Leader:     leader,
		Ctx:        ctx,
		CtxCancel:  cancel,
		HaveJoined: atomic.Bool{},
		StopChs:    []chan bool{},
		Wg:         sync.WaitGroup{},
	}
	syncmgr.Wg.Add(1)
	go syncmgr.subEvHndlr(ctx, &syncmgr.Wg)
	utils.ColorizeInfo(logger, "[dag sync] DagSync:Publisher is running...")
	syncmgr.Wg.Add(1)
	go syncmgr.syncPublish(ctx, &syncmgr.Wg)
	utils.ColorizeInfo(logger, "[dag sync] DagSync:Subscriber is running...")
	syncmgr.Wg.Add(1)
	go syncmgr.syncSubscribe(ctx, host.ID(), &syncmgr.Wg, bs)

	syncmgr.Wg.Add(1)
	syncmgr.StopChs = append(syncmgr.StopChs, make(chan bool))
	if config.GetConfig().Peer.SnapshotSync && !leader {
		go syncmgr.syncSnapshot(&syncmgr.Wg, syncmgr.StopChs[0])
	} else {
		go syncmgr.syncPin(&syncmgr.Wg, syncmgr.StopChs[0])
	}

	//wg.Wait()
	return syncmgr
}

var track_peers map[string]time.Time = make(map[string]time.Time)

type pub_state uint8

const (
	STATE_NONE pub_state = iota
	STATE_DEADLINE
	STATE_DONE
)

func (ps pub_state) String() string {
	return []string{"STATE_NONE", "STATE_DEADLINE", "STATE_DONE"}[ps]
}

func DONOTHING() {
	utils.ColorizeError(logger, "[DONOTHING] @TODO: IMPLEMENT")
}

func prepareCurrentDag(rec *tx.Syncv1) *tx.Syncv1 {
	return rec
}

func txDifference(left, right *tx.Syncv1) ([]uuid.UUID, []uuid.UUID) {

	ltx := set.New()
	rtx := set.New()
	// goterators.ForEach(left.TxIDs, func(id uuid.UUID) {
	// 	ltx.Insert(id)
	// })
	// goterators.ForEach(right.TxIDs, func(id uuid.UUID) {
	// 	rtx.Insert(id)
	// })
	inter := rtx.Intersection(ltx)
	ldiff := inter.Difference(rtx)
	rdiff := inter.Difference(ltx)
	lid := []uuid.UUID{}
	rid := []uuid.UUID{}
	ldiff.Do(func(i interface{}) {
		lid = append(lid, i.(uuid.UUID))
	})
	rdiff.Do(func(i interface{}) {
		rid = append(rid, i.(uuid.UUID))
	})
	return lid, rid
}

// pinsThatFit - as many commit transactions from the front of the slice as will
// fit in one gossip message, and how many bytes that came to.
//
// Budgeted at half of peer.msize: the pins are the bulk of the message but not
// all of it, and a message over the limit is not an error the sender sees - the
// receiving side drops it while reading, so the only symptom is a peer that
// asked for a range and never heard back. At least one pin is always sent, even
// if it alone is over budget: a batch of none can never make progress, and a
// single oversized commit transaction is a problem to see rather than to stall
// on quietly.
func pinsThatFit(pins []*pb.TxPin, budget int) ([]*pb.TxPin, int) {
	out := make([]*pb.TxPin, 0, len(pins))
	total := 0
	for _, pin := range pins {
		size := proto.Size(pin)
		if len(out) > 0 && total+size > budget {
			break
		}
		out = append(out, pin)
		total += size
	}
	return out, total
}

// catchUpBudget - how many bytes of commit transactions one catch-up response
// may carry. Half of peer.msize, because the pins are the bulk of the message
// but not all of it.
func catchUpBudget() int {
	if cfg := config.GetConfig(); cfg != nil && cfg.Peer.Msize > 0 {
		return cfg.Peer.Msize * config.MB / 2
	}
	return 8 * config.MB
}
