package dag

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/VG-Grape/luna/config"
	lunapeer "github.com/VG-Grape/luna/peer"
	txqueue "github.com/VG-Grape/luna/queues"
	sm "github.com/VG-Grape/luna/statemachine"
	"github.com/VG-Grape/luna/tx"
	"github.com/VG-Grape/luna/tx/pb"
	"github.com/enescakir/emoji"
	"github.com/google/uuid"
	"github.com/ledongthuc/goterators"
	"github.com/libp2p/go-libp2p/core/crypto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func handleSyncUpRequest(rec *tx.Syncv1) {
	logger.Infof("%s  ~ Handle SyncUp Site Request", emoji.Receipt)
	syncsm.resetSM(rec.Tracking_Id)
	syncsm.changeToSM(rec.Tracking_Id, sm.SYNC_DISPATCH_BEGIN)
	if err := processSyncUpRequest(rec.Tracking_Id); err != nil {
		logger.Errorf("%s  ~ Requesting GENESIS. err: %s", emoji.CrossMark, err.Error())
	}
	// wait for the state to change to SYNC_DISPATCH_END first
	syncsm.waitForSM(rec.Tracking_Id, sm.SYNC_DISPATCH_END, 500)
	syncsm.changeToSM(rec.Tracking_Id, sm.SYNC_HANDLE_END)
}

func handleSyncUpResponse(syncTx *tx.Syncv1) error {
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

func processSyncUpRequest(trackingId uuid.UUID) error {
	logger.Infof("%s  ~ Processing SyncUp Request [%s]", emoji.HammerAndWrench, trackingId)
	host := lunapeer.GetHost()
	hostpk := host.Peerstore().PrivKey(host.ID())
	stdkey, _ := crypto.PrivKeyToStdKey(hostpk)
	pk, pubkey, err := crypto.KeyPairFromStdKey(stdkey)
	if err != nil {
		return fmt.Errorf("failed to obtain pubkey. %v+", err)
	}

	// grab the latest known pin tx
	genesis := GetDag().getGenesis()
	// prepare for announcement the latest pin tx
	stx := tx.NewSyncv1()
	stx.Ver_type = tx.STVX1
	stx.Sync_Type = tx.GENESIS
	stx.Msg_Type = tx.STX_SITE_RESPONSE
	if trackingId == uuid.Nil {
		trackingId = uuid.New()
	}
	stx.Tracking_Id = trackingId
	// prepare a state machine for when we receive a responce
	syncsm.resetSM(stx.Tracking_Id)
	stx.Sender_Pubk, err = crypto.MarshalPublicKey(pubkey)
	if err != nil {
		return fmt.Errorf("error marshaling public key: %s", err.Error())
	}
	any, _ := anypb.New(genesis.ToPbNode())
	stx.Details = append(stx.Details, any)
	payload, _ := proto.Marshal(stx.MarshalBinary())
	stx.SyncHash = sha256.New().Sum(payload)
	stx.Signature = stx.GenerateSignature(pk)
	// Prepare a state machine to handle our sync sequence
	syncsm.changeToSM(stx.Tracking_Id, sm.SYNC_DISPATCH_BEGIN)
	// we publish sync tx to a sync queue
	txqueue.GetSyncQueue().Enqueue(stx)
	syncsm.waitForSM(stx.Tracking_Id, sm.SYNC_DISPATCH_END, 1000)
	return nil
}

func syncUpPeer(dsm *DagSyncMngr) {
	logger.Infof("%s  ~ Announcing ourselves [Requesting GENESIS+PIN]...", emoji.Loudspeaker)
	announceWg := sync.WaitGroup{}
	if !config.GetConfig().Peer.SnapshotSync {
		announceWg.Add(1)
		go syncUpPin(dsm, &announceWg)
	}
	announceWg.Add(1)
	go syncUpSites(dsm, &announceWg)
	runtime.Gosched()
	t := time.NewTimer(time.Second)
	<-t.C
	logger.Infof("%s  ~ Waiting for announcements to be publislhed...", emoji.HourglassNotDone)
	announceWg.Wait()
	if !config.GetConfig().Peer.SnapshotSync {
		if dsm.HaveJoined.Load() {
			logger.Infof("%s  ~ Successfully synchronized pins %s", emoji.CheckBoxWithCheck, emoji.RoundPushpin)
		}
	}
	if dsm.SitesProcessed.Load() {
		logger.Infof("%s  ~ Successfully synchronized sites %s", emoji.CheckBoxWithCheck, emoji.Compass)
	}
}

func handleSyncPinRequest(rec *tx.Syncv1, leader bool) {
	logger.Infof("[handle sync] * Handle Sync Pin request:\n%s\n", rec.String())
	// I know about this transaction, handle it as if the other node
	// announces their own current state in response to ours
	// if their state is newer than ours - we need to update
	// our state, otherwise, assume they saw our state and responded with theirs
	// If their state is older, do nothing after the announcement
	// Need to pay attention to the pintx sequence number as well
	// See if there is a difference in the number of transactions
	syncsm.changeToSM(rec.Tracking_Id, sm.SYNC_HANDLE_BEGIN)
	respondLatestPin(rec) // this call will try to dispatch the sync event, wait until it's done
	// wait for the state to change to sm.SYNC_DISPATCH_END first
	syncsm.waitForSM(rec.Tracking_Id, sm.SYNC_DISPATCH_END, 100)
	syncsm.changeToSM(rec.Tracking_Id, sm.SYNC_HANDLE_END)
}

func handleSyncBalanceRequest(rec *tx.Syncv1) {
	logger.Infof("[handle sync] * Handle Sync Balance request:\n%s\n", rec.String())
	respondLatestBalances(rec)
}

func handleSyncBalanceResponse(rec *tx.Syncv1) {
	logger.Infof("[handle sync] * Handle Sync Balance response:\n%s\n", rec.String())
	handleLatestBalances(rec)
}

func handleMissPinRequest(rec *tx.Syncv1) {
	logger.Infof("%s  ~ Handle missing pin tx request", emoji.HammerAndWrench)
	// let's find a pin tx that matches the signature in the request
	_pins_.mu.Lock()
	defer _pins_.mu.Unlock()
	for _, p := range _pins_.pins {
		if bytes.Equal(p.Sign, rec.Data) {
			publishMissPinResponse(rec.Tracking_Id, p)
		}
	}
}

func handleMissPinResponse(rec *tx.Syncv1) error {
	logger.Infof("%s  ~ Handle missing pin tx response", emoji.HammerAndWrench)
	return processMissPinResponse(rec)
}

func publishMissPinResponse(trackingId uuid.UUID, pin *pb.TxPin) error {
	logger.Infof("%s ~ Publish response to MISS PIN REQ [%s]", emoji.IncomingEnvelope, trackingId)
	host := lunapeer.GetHost()
	// messages are signed with the host private key, not the wallet's
	hostpk := host.Peerstore().PrivKey(host.ID())
	stdkey, _ := crypto.PrivKeyToStdKey(hostpk)
	pk, pubkey, err := crypto.KeyPairFromStdKey(stdkey)
	if err != nil {
		return fmt.Errorf("failed to obtain pubkey. %v+", err)
	}

	// prepare miss pin tx response
	stx := tx.NewSyncv1()
	stx.Ver_type = tx.STVX1
	stx.Sync_Type = tx.MISSING
	stx.Msg_Type = tx.STX_PIN_RESPONSE
	stx.Tracking_Id = trackingId
	// prepare a state machine for when we receive a responce
	syncsm.resetSM(stx.Tracking_Id)
	defer syncsm.deleteSM(stx.Tracking_Id)

	// we are using RSA pub key to check the payload signature
	stx.Sender_Pubk, err = crypto.MarshalPublicKey(pubkey)
	if err != nil {
		return fmt.Errorf("error marshaling public key: %s", err.Error())
	}

	// add the requested pin tx
	pbtx, err := pin.MarshalBinary()
	if err != nil {
		return fmt.Errorf("Failed to marshal pin tx. err:%s", err.Error())
	}
	stx.Data = pbtx
	payload, _ := proto.Marshal(stx.MarshalBinary())
	stx.SyncHash = sha256.New().Sum(payload)
	stx.Signature = stx.GenerateSignature(pk)
	// Prepare a state machine to handle our sync sequence
	syncsm.changeToSM(stx.Tracking_Id, sm.SYNC_DISPATCH_BEGIN)
	// we publish sync tx to a sync queue
	txqueue.GetSyncQueue().Enqueue(stx)
	st, err := syncsm.waitForSM(stx.Tracking_Id, sm.SYNC_DISPATCH_END, 500)
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

// handleSyncSiteRequest - process the incoming request
// in response provide all the vertices(nodes) based on uuid
func handleSyncSiteRequest(rec *tx.Syncv1) {
	logger.Infof("%s  ~ Handle Sync Site Request", emoji.Receipt)
	syncsm.changeToSM(rec.Tracking_Id, sm.SYNC_HANDLE_BEGIN)

	ids := []string{}
	for _, v := range rec.Details {
		prMsg, _ := v.UnmarshalNew()
		// we expect type wrapper.StringValue
		switch t := prMsg.(type) {
		case *wrapperspb.StringValue:
			ids = append(ids, t.GetValue())
		}
	}
	if err := handleSiteRequest(rec.Tracking_Id, ids); err != nil {
		logger.Errorf("%s  ~ Requesting sites %s. err: %s", emoji.CrossMark, ids, err.Error())
	}
	// wait for the state to change to SYNC_DISPATCH_END first
	syncsm.waitForSM(rec.Tracking_Id, sm.SYNC_DISPATCH_END, 100)
	syncsm.changeToSM(rec.Tracking_Id, sm.SYNC_HANDLE_END)
}

func handleSyncRecord(rec *tx.Syncv1, leader bool) {
	logger.Infof("[handle sync] * Handle Sync msg:\n%s\n", rec.String())
	if rec.Msg_Type == tx.STX_ANNOUNCE {
		if syncsm.existsSM(rec.Tracking_Id) {
			// I know about this transaction, handle it as if the other node
			// announces their own current state in response to ours
			// if their state is newer than ours - we need to update
			// our state, otherwise, assume they saw our state and responded with theirs
			// If their state is older, do nothing after the announcement
			// Need to pay attention to the pintx sequence number as well
			logger.Info("[handle sync] STX_ANNOUNCE Response")
			// See if there is a difference in the number of transactions
			syncsm.changeToSM(rec.Tracking_Id, sm.SYNC_HANDLE_BEGIN)
			for {
				if PIN_TX_STACK.Len() == 0 {
					t := time.NewTimer(time.Millisecond * 100)
					<-t.C
					t.Stop()
					continue
				}
				break
			}
			latest_pinning := PIN_TX_STACK.Peek().(*tx.Syncv1)
			// Other peer
			otherPeer := tx.NewTxv1(chaintype)
			otherPeer.UnmarshalJSON(rec.Data)
			// Our peer
			thisPeer := tx.NewTxv1(chaintype)
			thisPeer.UnmarshalJSON(latest_pinning.Data)
			thisPeer_Depth := binary.BigEndian.Uint64(thisPeer.Data[:8])
			otherPeer_Depth := binary.BigEndian.Uint64(otherPeer.Data[:8])
			if thisPeer_Depth > otherPeer_Depth {
				logger.Warnf("We are ahead by %d @ depth %d. Publishing our pinning tx %s",
					thisPeer_Depth-otherPeer_Depth,
					thisPeer_Depth,
					latest_pinning.String(),
				)
				// if we are the leader node - push the update
				if leader {
					update_pin_tx := *latest_pinning
					update_pin_tx.Sync_Type = tx.SyncType(tx.STX_UPDATE)
					update_pin_tx.Tracking_Id = rec.Tracking_Id
					syncsm.resetSM(rec.Tracking_Id)
					syncsm.changeToSM(rec.Tracking_Id, sm.SYNC_DISPATCH_BEGIN)
					txqueue.GetSyncQueue().Enqueue(&update_pin_tx)
					return
				} else {
					// just ignore this tx - we are not a leader to handle it
					syncsm.changeToSM(rec.Tracking_Id, sm.SYNC_HANDLE_END)
					return
				}
				// Publish our pin tx - @TODO
				// syncsm.machines[rec.Tracking_Id] = NewSyncStateMachine()
				// _ = syncsm.machines[rec.Tracking_Id].ChangeTo(sm.SYNC_DISPATCH_BEGIN)
				// logger.Infof("[handle sync] Enqueue %s\n", rec.String())
				// update_pin_tx := *latest_pinning
				// update_pin_tx.Tracking_Id = rec.Tracking_Id
				// txqueue.GetSyncQueue().Enqueue(&update_pin_tx)
			} else if thisPeer_Depth < otherPeer_Depth {
				logger.Warnf("We are behind by %d @ depth %d [their depth: %d]. Accepting their pinning tx %s",
					otherPeer_Depth-thisPeer_Depth,
					thisPeer_Depth,
					otherPeer_Depth,
					rec.String(),
				)
				// store their pin tx - @TODO
				PIN_TX_STACK.Push(rec)
				logger.Warnf("Catching up to their depth %d by accepting their tx %s",
					otherPeer_Depth,
					rec.String(),
				)
				logger.Error("@TODO: implement DAG update as well")
				// tell our DAG to update to the latest based on info in the sync stack
				// @TODO
			} else {
				if bytes.Equal(rec.SyncHash, latest_pinning.SyncHash) {
					logger.Infof("[handle sync] We are in sync @ DAG depth %d", thisPeer_Depth)
				} else {
					lid, rid := txDifference(latest_pinning, rec)
					l_rid, l_lid := len(rid), len(lid)
					logger.Infof("[handle sync] @ Depth %d We have %d, the other node has %d tx that are different",
						thisPeer_Depth, l_lid, l_rid,
					)
					if l_rid > 0 {
						logger.Infof("[handle sync] @ Depth %d: Adding the following transaction to our DAG:", thisPeer_Depth)
						goterators.ForEach(rid, func(id uuid.UUID) {
							logger.Infof("\tTx: %s", id.String())
						})
					}
					if l_lid > 0 {
						logger.Infof("[handle sync] @ Depth %d: Publishing the following transaction to the other peer's DAG:", thisPeer_Depth)
						goterators.ForEach(lid, func(id uuid.UUID) {
							logger.Infof("\tTx: %s", id.String())
						})
					}
					if l_lid == 0 && l_rid == 0 {
						logger.Warnf("[handle sync] We are in sync @ depth %d [but hashes are different]", thisPeer_Depth)
					}
				}
			}
			syncsm.changeToSM(rec.Tracking_Id, sm.SYNC_HANDLE_END)
		} else {
			// this is an unknown to me announcement
			// most likely this is a new node announcing its current state
			// check if their's starting point is behind ours
			// and if so, response with the latest pinning transaction
			// along with all the transactions in the current slice
			logger.Info("[DAG SYNC] STX_ANNOUNCE Request")
			// Let's see if their pinning transaction is in our history
			// Every genesis transaction should be the same

			// No need to update our own statemachine for this transaction
			// Just reply with the same tracking id
			syncsm.changeToSM(rec.Tracking_Id, sm.SYNC_HANDLE_BEGIN)
			latest_pinning := PIN_TX_STACK.Peek().(*tx.Syncv1)
			// Other peer
			otherPeer := tx.NewTxv1(chaintype)
			otherPeer.UnmarshalJSON(rec.Data)
			// Our peer
			thisPeer := tx.NewTxv1(chaintype)
			thisPeer.UnmarshalJSON(latest_pinning.Data)
			thisPeer_Depth := binary.BigEndian.Uint64(thisPeer.Data[:8])
			otherPeer_Depth := binary.BigEndian.Uint64(otherPeer.Data[:8])
			if bytes.Equal(rec.SyncHash, latest_pinning.SyncHash) {
				// transactions are the same, just send back all the transactions
				// from the current pinning transaction in dag and in cache
				// that are missing from the tx_ids field
				if otherPeer_Depth < thisPeer_Depth {
					// they are behind on transactions with the current pin
					DONOTHING()
				} else if otherPeer_Depth > thisPeer_Depth {
					// they have more than us, update ourselves
					DONOTHING()
				} else {
					logger.Info("[!]DAGs are in sync across the peers")
				}
			} else {
				// see who is behind
				if otherPeer_Depth < thisPeer_Depth {
					// they are behind
					// send all we have from the latest pinning transaction on
					update := prepareCurrentDag(rec)
					txqueue.GetSyncQueue().Enqueue(update)
					logger.Infof("The other node with depth %d is behind of our depth %d",
						otherPeer_Depth, thisPeer_Depth,
					)
					// send our latest pinning tx with all the tx we know about
				} else if otherPeer_Depth > thisPeer_Depth {
					logger.Infof("The other node with depth %d is ahead of our depth %d",
						otherPeer_Depth, thisPeer_Depth,
					)
					// accept their pinning tx with their tx, keep ours, and, if there is
					// a difference after merge - send the list of tx missing on the other node
				} else {
					logger.Infof("The other node with depth %d is the same as our depth %d but they are different",
						otherPeer_Depth, thisPeer_Depth,
					)
					// exchange tx, send all ours, receive all theirs, and insert theirs into our DAG
				}
			}
			// Handle different states as well, when needed
			syncsm.changeToSM(rec.Tracking_Id, sm.SYNC_HANDLE_END)
		}
	}
}

func handleSyncPinResponse(stx *tx.Syncv1) error {
	logger.Infof("[handle sync response] Handle Sync msg:\n%s\n", stx.String())

	pubkey, err := crypto.UnmarshalPublicKey(stx.Sender_Pubk)
	if err != nil {
		return fmt.Errorf("error marshaling public key: %s", err.Error())
	}
	ok, err := stx.VerifySignature(pubkey)
	if err == nil && ok {
		latest_pin := &pb.TxPin{}
		err = latest_pin.UnmarshalBinary(stx.Data)
		if err != nil {
			return fmt.Errorf("%s  ~ error unmarshaling latest pin tx: %s", emoji.CrossMark, err.Error())
		}
		_pins_.insertIfNotFound(latest_pin)
	} else {
		logger.Errorf("%s  ~ %s Cannot verify signature: %s", emoji.RoundPushpin, emoji.LockedWithKey, err.Error())
	}
	return err

}
