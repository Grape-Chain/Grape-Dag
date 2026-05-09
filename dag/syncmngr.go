package dag

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/VG-Grape/luna/config"
	lunapeer "github.com/VG-Grape/luna/peer"
	txqueue "github.com/VG-Grape/luna/queues"
	sm "github.com/VG-Grape/luna/statemachine"
	"github.com/VG-Grape/luna/tx"
	"github.com/VG-Grape/luna/tx/pb"
	"github.com/VG-Grape/luna/utils"
	"github.com/enescakir/emoji"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/record"
	dutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
)

type DagSyncMngr struct {
	SyncSub        *DagSyncSub
	Leader         bool
	Ctx            context.Context
	CtxCancel      context.CancelFunc
	HaveJoined     atomic.Bool // indicate when startup pin tx processed
	SitesProcessed atomic.Bool // indicate when startup site sync processed
	StopChs        []chan bool
	Wg             sync.WaitGroup
	StopFlag       atomic.Bool
}

var syncTopicPeerCount atomic.Int64 = atomic.Int64{}

func (dsm *DagSyncMngr) Stop() {
	logger.Infof("%s  ~ Stopping DAG Sync Manager...", emoji.HorizontalTrafficLight)
	for _, ch := range dsm.StopChs {
		ch <- true
	}
	dsm.StopFlag.Store(true)
	logger.Infof("%s  ~ Sending stop request to sync publisher %s ->", emoji.HorizontalTrafficLight, emoji.IncomingEnvelope)
	txqueue.GetSyncQueue().Enqueue(tx.NewSyncv1Stop())
	dsm.CtxCancel()
	logger.Infof("%s  ~ Waiting for sync routines to stop...", emoji.Stopwatch)
	dsm.Wg.Wait()
	logger.Infof("%s  ~ Sync routines stopped  %s", emoji.VerticalTrafficLight, emoji.BalanceScale)
	dsm.SyncSub.destroy()
	logger.Infof("%s  ~ DAG Sync Manager stopped", emoji.VerticalTrafficLight)
}

func (s *DagSyncMngr) subEvHndlr(ctx context.Context, wg *sync.WaitGroup) {
	logger.Info("[dag sync evt hndlr] Running...")
	defer wg.Done()
	verbosity := config.GetConfig().Host.Verbose
	for {
		evt, err := s.SyncSub.teh.NextPeerEvent(ctx)
		if err != nil {
			logger.Infof("%s  ~ Stopping Sub Event Handler for Topic: %s ", emoji.HorizontalTrafficLight, s.SyncSub.topic.String())
			break
		}

		switch evt.Type {
		case pubsub.PeerJoin:
			// wait for at least one more peer to join the topic before announcing [except to leader]
			logger.Infof("%s  Topic: %s PEER_JOIN : %s", emoji.Collision, s.SyncSub.topic.String(), evt.Peer.String())
			syncTopicPeerCount.Add(1)

			if verbosity > 0 {
				track_peers[evt.Peer.String()] = time.Now()
				// if s.Leader {
				peer_ais, err := dutil.FindPeers(context.Background(), s.SyncSub.rd, s.SyncSub.rendezvous)
				if err != nil {
					logger.Errorf("[dag sync evt hdl] Find peers: %s", err.Error())
				}
				if len(peer_ais) >= int(syncTopicPeerCount.Load()) {
					logger.Infof("[dag sync evt hdl] [+] %d <-> %d", len(peer_ais), syncTopicPeerCount.Load())
				}
				if verbosity > 1 {
					peers := s.SyncSub.topic.ListPeers()
					if len(peers) != int(syncTopicPeerCount.Load()) {
						logger.Errorf("[dag sync evt hdl] [E] *** PEER_JOIN:%s but count != %d ***", evt.Peer.String(), syncTopicPeerCount.Load())
						for i, v := range track_peers {
							logger.Infof("[dag sync evt hdl] Topic: %s Host: %s For: %f sec",
								s.SyncSub.topic.String(), i, time.Since(v).Seconds(),
							)
						}
					}
					for _, p := range peer_ais {
						logger.Infof("[dag sync evt hdl] [+] Peer %s has joined the topic: %s", p.String(), s.SyncSub.rendezvous)
						peerId := lunapeer.GetHost().ID().String()
						if peerId == p.ID.String() {
							utils.ColorizeInfo(logger, "[dag sync evt hdl] [!] Our peer %s has joined %s. READY", peerId, s.SyncSub.rendezvous)
						}
					}
				}
			}
			// }

		case pubsub.PeerLeave:
			logger.Infof("[dag sync evt hdl] [-] Topic:%s PEER_LEAVE: %s after %f sec",
				s.SyncSub.topic.String(),
				evt.Peer.String(),
				time.Since(track_peers[evt.Peer.String()]).Seconds(),
			)
			syncTopicPeerCount.Add(-1)
			delete(track_peers, evt.Peer.String())
		}
	}
	logger.Infof("%s  ~ Sub Event Handler stopped", emoji.VerticalTrafficLight)
}

func whenToPublish(mt tx.SyncMsgType) bool {
	return mt == tx.STX_ANNOUNCE ||
		mt == tx.STX_REQUEST ||
		mt == tx.STX_RESPONSE ||
		mt == tx.STX_PIN_REQUEST ||
		mt == tx.STX_PIN_RESPONSE ||
		mt == tx.STX_SITE_REQUEST ||
		mt == tx.STX_SITE_RESPONSE ||
		mt == tx.STX_SNAPSHOT_BALANCE_REQUEST ||
		mt == tx.STX_SNAPSHOT_BALANCE_RESPONSE ||
		mt == tx.STX_PIN_DOWNLOAD_REQUEST ||
		mt == tx.STX_PIN_DOWNLOAD_RESPONSE
}

// syncPublish - a go routine that retrieves sync packets from a sync queue
// and publishes them to the network. Is launched by DagSync.
func (syncmgr *DagSyncMngr) syncPublish(ctx context.Context, wg *sync.WaitGroup) {
	// keep on looping until asked to stop: sync pub topic is closed
	defer wg.Done()
	var syncTx *tx.Syncv1
	pubopt := []pubsub.PubOpt{}
	for !syncmgr.StopFlag.Load() {
		// get the latest sync packet from the queue
		deq := txqueue.GetSyncQueue().Dequeue()
		if deq != nil {
			syncTx = deq.(*tx.Syncv1)
			if syncTx == nil {
				logger.Errorf("[dag sync] publish cast to tx.Syncv1 fails. %T", deq)
				continue
			} else if syncTx.Msg_Type == tx.STX_STOP {
				logger.Infof("%s  ~ Sync Publish stopped", emoji.VerticalTrafficLight)
				return
			}
		} else {
			logger.Debugf("[dag sync] No txs in SyncQueue, skip sync publish")
			continue
		}
		// seal the packet
		envelope, err := record.Seal(syncTx, lunapeer.GetHost().Peerstore().PrivKey(lunapeer.GetHost().ID()))
		if err != nil {
			utils.ColorizeError(logger, "[dag sync] Failed to seal the record. %v", err)
		}
		// need in binary format to send over a pub
		bytes, _ := envelope.Marshal()
		// Note: Publish may be very sticky, make sure where get to this point
		if config.GetConfig().Host.Verbose > 1 {
			logger.Infof("[dag sync] PUBLISH SYNC: %s", syncTx.Msg_Type.String())
		}
		if whenToPublish(syncTx.Msg_Type) {
			if !syncmgr.Leader {
				paddr, err := dutil.FindPeers(context.Background(), syncmgr.SyncSub.rd, syncmgr.SyncSub.rendezvous)
				if err != nil {
					logger.Errorf("[dag sync] Find peers: %s", err.Error())
				}
				if syncTopicPeerCount.Load() == 0 && len(paddr) == 0 {
					logger.Warnf("%s  No peers found for topic: %s. Will not publish...", emoji.Warning, syncmgr.SyncSub.topic.String())
					syncsm.changeToSM(syncTx.Tracking_Id, sm.SYNC_CANCEL_STATE)
					continue
				}
				pubopt = append(pubopt, pubsub.WithReadiness(pubsub.MinTopicSize(1)))
			}
			if config.GetConfig().Host.Verbose > 0 {
				logger.Infof("%s  => Publishing %s|%s [%s]",
					emoji.IncomingEnvelope,
					syncTx.Msg_Type,
					syncTx.Sync_Type,
					syncTx.Tracking_Id)
			}
			tp := time.Now()
			pub_ctx, pub_csl := context.WithTimeout(ctx, time.Second*15)
			chDonePub := make(chan bool)
			wg := sync.WaitGroup{}
			wg.Add(1)
			flag := atomic.Int32{}

			go func() {
				defer wg.Done()
				select {
				case <-chDonePub:
					flag.Store(int32(STATE_DONE))
					pub_csl()
					return
				case <-pub_ctx.Done():
					flag.Store(int32(STATE_DEADLINE))
					logger.Errorf("%s ~ %s  Deadline has been reached for %s|%s %s",
						emoji.AlarmClock,
						emoji.CrossMark,
						syncTx.Msg_Type,
						syncTx.Sync_Type,
						syncTx.Tracking_Id,
					)
					return
				}
			}()

			err := syncmgr.SyncSub.topic.Publish(pub_ctx, bytes, pubopt...)
			pub_flag := pub_state(flag.Load())
			if pub_flag == STATE_NONE {
				logger.Infof("%s  ~ Signaling to publish monitoring about publish event....", emoji.TriangularFlag)
				chDonePub <- true
			}
			logger.Infof("%s  ~ Waiting for publish monitoring to finish...", emoji.HourglassNotDone)
			wg.Wait()
			logger.Infof("%s  ~ Publish monitoring finished...", emoji.HourglassDone)
			close(chDonePub)
			if syncmgr.StopFlag.Load() {
				break
			}
			if pub_flag == STATE_DEADLINE || err != nil {
				logger.Errorf("%s  ~ Did not Publish Sync %s|%s %s",
					emoji.Warning,
					syncTx.Msg_Type,
					syncTx.Sync_Type,
					syncTx.Tracking_Id,
				)
				syncsm.changeToSM(syncTx.Tracking_Id, sm.SYNC_CANCEL_STATE)
				pub_csl()
				//break
			} else {
				if config.GetConfig().Host.Verbose > 0 {
					logger.Infof("%s  ~ Published %s|%s [%s] after %f sec",
						emoji.CheckMarkButton,
						syncTx.Msg_Type,
						syncTx.Sync_Type,
						syncTx.Tracking_Id,
						time.Since(tp).Seconds(),
					)
				}
				syncsm.resetSM(syncTx.Tracking_Id)
				syncsm.changeToSM(syncTx.Tracking_Id, sm.SYNC_DISPATCH_BEGIN)
				syncsm.changeToSM(syncTx.Tracking_Id, sm.SYNC_DISPATCH_END)
			}
		}
	}
	logger.Infof("%s  ~ Sync Publisher stopped", emoji.VerticalTrafficLight)
}

func isSyncPinRequest(stx *tx.Syncv1, leader bool) bool {
	return stx.Ver_type == tx.STVX1 &&
		stx.Sync_Type == tx.CURRENT &&
		stx.Msg_Type == tx.STX_ANNOUNCE && leader
}

func isSyncSiteRequest(stx *tx.Syncv1, leader bool) bool {
	return stx.Ver_type == tx.STVX1 &&
		stx.Sync_Type == tx.SITE &&
		stx.Msg_Type == tx.STX_REQUEST && leader
}

func isSyncBalanceSnapshotRequest(stx *tx.Syncv1, leader bool) bool {
	return stx.Ver_type == tx.STVX1 &&
		stx.Sync_Type == tx.CURRENT &&
		stx.Msg_Type == tx.STX_SNAPSHOT_BALANCE_REQUEST && leader
}

func isSyncBalanceSnapshotResponse(stx *tx.Syncv1) bool {
	return stx.Ver_type == tx.STVX1 &&
		stx.Sync_Type == tx.CURRENT &&
		stx.Msg_Type == tx.STX_SNAPSHOT_BALANCE_RESPONSE &&
		syncsm.currentSM(stx.Tracking_Id) != sm.SYNC_ZERO_STATE && config.GetConfig().Peer.SnapshotSync
}

func isPinDownloadRequest(stx *tx.Syncv1, leader bool) bool {
	return stx.Ver_type == tx.STVX1 &&
		stx.Sync_Type == tx.CURRENT &&
		stx.Msg_Type == tx.STX_PIN_DOWNLOAD_REQUEST && leader
}

func isPinDownloadResponse(stx *tx.Syncv1) bool {
	return stx.Ver_type == tx.STVX1 &&
		stx.Sync_Type == tx.CURRENT &&
		stx.Msg_Type == tx.STX_PIN_DOWNLOAD_RESPONSE &&
		syncsm.existsSM(stx.Tracking_Id) &&
		config.GetConfig().Peer.SnapshotSync
}

func isSyncSiteResponse(stx *tx.Syncv1) bool {
	return stx.Ver_type == tx.STVX1 &&
		stx.Sync_Type == tx.SITE &&
		stx.Msg_Type == tx.STX_RESPONSE
}

func isSyncUpSiteRequest(stx *tx.Syncv1, leader bool) bool {
	return stx.Ver_type == tx.STVX1 &&
		stx.Sync_Type == tx.GENESIS &&
		stx.Msg_Type == tx.STX_SITE_REQUEST && leader
}

func isSyncUpSiteResponse(stx *tx.Syncv1) bool {
	return stx.Ver_type == tx.STVX1 &&
		stx.Sync_Type == tx.GENESIS &&
		stx.Msg_Type == tx.STX_SITE_RESPONSE
}

func isSyncPinResponse(stx *tx.Syncv1) bool {
	return stx.Ver_type == tx.STVX1 &&
		stx.Sync_Type == tx.LATEST &&
		stx.Msg_Type == tx.STX_RESPONSE && !config.GetConfig().Peer.SnapshotSync
}

func isNewPinAnnounce(stx *tx.Syncv1) bool {
	return stx.Ver_type == tx.STVX1 &&
		stx.Sync_Type == tx.LATEST &&
		stx.Msg_Type == tx.STX_ANNOUNCE
}

func isMissingPinRequest(stx *tx.Syncv1) bool {
	return stx.Ver_type == tx.STVX1 &&
		stx.Sync_Type == tx.MISSING &&
		stx.Msg_Type == tx.STX_PIN_REQUEST && !config.GetConfig().Peer.SnapshotSync
}

func isMissingPinResponse(stx *tx.Syncv1) bool {
	return stx.Ver_type == tx.STVX1 &&
		stx.Sync_Type == tx.MISSING &&
		stx.Msg_Type == tx.STX_PIN_RESPONSE && !config.GetConfig().Peer.SnapshotSync
}

// syncSubscribe - receive sync messages published on the sync topic
func (syncmgr *DagSyncMngr) syncSubscribe(ctx context.Context, pid peer.ID, wg *sync.WaitGroup, bs bool) {
	defer wg.Done()
	logger.Infof("[dag sync sub] syncSubscriber peer.ID %s is running %s ->", pid.String(), emoji.PersonRunning)
	for {
		// get the next sync packet
		msg, err := syncmgr.SyncSub.sub.Next(ctx)
		if err != nil {
			logger.Info("[dag sync sub] Cancel request received")
			break
		}

		// only consider messages delivered by other peers
		if msg.ReceivedFrom == pid {
			continue
		}
		// tx are embedded in the msg Data field
		data_bytes := msg.Data
		syncTx := &tx.Syncv1{}
		_, err = record.ConsumeTypedEnvelope(data_bytes, syncTx)
		if err != nil {
			logger.Errorf("[dag sync sub] Failed to consume message. %v", err)
		}

		if config.GetConfig().Host.Verbose > 0 {
			logger.Infof("%s  ~ *[SYNC SUB]*  %s|%s [%s]", emoji.IncomingEnvelope, syncTx.Msg_Type, syncTx.Sync_Type, syncTx.Tracking_Id)
		}

		switch {
		// through pubsub the leader receives a request to provide pin tx missing from the node
		// that issued the request.
		case isSyncUpSiteRequest(syncTx, syncmgr.Leader):
			logger.Infof("%s  Handling SyncUp Request", emoji.CheckBoxWithCheck)
			handleSyncUpRequest(syncTx)
			syncsm.deleteSM(syncTx.Tracking_Id)
		case isSyncUpSiteResponse(syncTx):
			logger.Infof("%s  Handling SyncUp Response", emoji.CheckBoxWithCheck)
			err := handleSyncUpResponse(syncTx)
			if err != nil {
				logger.Errorf("Failed to process SyncUp response: %s", err.Error())
			} else {
				syncmgr.SitesProcessed.Store(true)
			}
			syncsm.deleteSM(syncTx.Tracking_Id)
		case isMissingPinRequest(syncTx) && syncmgr.Leader:
			logger.Infof("%s  Handling Missing Pin Request", emoji.CheckBoxWithCheck)
			handleMissPinRequest(syncTx)
			syncsm.deleteSM(syncTx.Tracking_Id)
		case isMissingPinResponse(syncTx):
			logger.Infof("%s  Handling Missing Pin Response", emoji.CheckBoxWithCheck)
			err := handleMissPinResponse(syncTx)
			if err != nil {
				logger.Errorf("[dag sync sub] Failed to process MISS PIN RESPONSE. err:%s", err.Error())
			}
			syncsm.deleteSM(syncTx.Tracking_Id)
		case isSyncSiteRequest(syncTx, syncmgr.Leader):
			logger.Infof("%s  Handling Sync Site Request", emoji.CheckBoxWithCheck)
			syncsm.resetSM(syncTx.Tracking_Id)
			syncsm.changeToSM(syncTx.Tracking_Id, sm.SYNC_QUERY_BEGIN)
			handleSyncSiteRequest(syncTx)
			syncsm.deleteSM(syncTx.Tracking_Id)
		case isSyncSiteResponse(syncTx):
			logger.Infof("%s  Handling Sync Site Response", emoji.CheckBoxWithCheck)
			if syncsm.existsSM(syncTx.Tracking_Id) {
				if syncsm.currentSM(syncTx.Tracking_Id) == sm.SYNC_DISPATCH_END {
					syncsm.resetSM(syncTx.Tracking_Id)
					syncsm.changeToSM(syncTx.Tracking_Id, sm.SYNC_QUERY_BEGIN)
					handleSyncSiteResponse(syncTx)
					if syncsm.currentSM(syncTx.Tracking_Id) == sm.SYNC_HANDLE_END {
						syncsm.changeToSM(syncTx.Tracking_Id, sm.SYNC_QUERY_END)
						// at this point we finished processing the tx and the sm associated
						// with it may be distroyed
						syncsm.deleteSM(syncTx.Tracking_Id)
					}
				}
			} else {
				logger.Warnf("%s  ~ Response %s not for us. [SM missing] Ignoring...", emoji.HammerAndWrench, syncTx.Tracking_Id)
			}
		case isSyncPinRequest(syncTx, syncmgr.Leader):
			logger.Infof("%s  Handling Sync Pin Request", emoji.CheckBoxWithCheck)
			syncsm.resetSM(syncTx.Tracking_Id)
			syncsm.changeToSM(syncTx.Tracking_Id, sm.SYNC_QUERY_BEGIN)
			handleSyncPinRequest(syncTx, syncmgr.Leader)
			if syncsm.currentSM(syncTx.Tracking_Id) == sm.SYNC_HANDLE_END {
				syncsm.changeToSM(syncTx.Tracking_Id, sm.SYNC_QUERY_END)
				// at this point we finished processing the tx and the sm associated
				// with it may be distroyed
				syncsm.deleteSM(syncTx.Tracking_Id)
			}
		case isNewPinAnnounce(syncTx):
			logger.Infof("%s  Handling New Pin Announce", emoji.CheckBoxWithCheck)
			// we are announcing a new pin
			pin := pb.TxPin{}
			err := pin.UnmarshalBinary(syncTx.Data)
			if err != nil {
				logger.Errorf("[dag sync sub] error marshaling latest pin tx: %s", err.Error())
			} else {
				if GetPin().IsReady() && (config.GetConfig().Peer.NodeType == 0 || config.GetConfig().Peer.NodeType == 1) {
					processPin(&pin)
				} else {
					logger.Infof("Ignore received Pin=%d, nodeReady=%t, nodeType=%d", pin.PinNumber, GetPin().IsReady(), config.GetConfig().Peer.NodeType)
				}
			}
		case isSyncPinResponse(syncTx):
			logger.Infof("%s  Handling Sync Pin Response", emoji.CheckBoxWithCheck)
			if syncsm.existsSM(syncTx.Tracking_Id) {
				if config.GetConfig().Host.Verbose > 1 {
					logger.Infof("%s  ~ SINC PIN RESPONSE: [%s] \n%s", emoji.RoundPushpin, syncTx.Tracking_Id, syncTx.String())
				}
				if syncsm.currentSM(syncTx.Tracking_Id) != sm.SYNC_DISPATCH_END {
					count := 0
					for {
						syncsm.waitForSM(syncTx.Tracking_Id, sm.SYNC_DISPATCH_END, 500)
						if syncsm.currentSM(syncTx.Tracking_Id) == sm.SYNC_DISPATCH_END {
							break
						}
						count++
						if count > 5 {
							logger.Warnf("%s  Past the deadline to wait for the state change from %s to SYNC_DISPATCH_END",
								emoji.Warning, syncsm.currentSM(syncTx.Tracking_Id))
							break
						}
					}
				}
				if !syncmgr.HaveJoined.Load() {
					logger.Infof("\n%s  ~ Our PIN SYNC %s REQUEST -> RESPONSE received\n\n", emoji.IncomingEnvelope, emoji.RoundPushpin)
					syncmgr.HaveJoined.Store(true)
				}
				logger.Infof("[dag sync sub] Handle PIN response id:%s", syncTx.Tracking_Id.String())
				if err = handleSyncPinResponse(syncTx); err != nil {
					logger.Errorf("%s  ~ Failed to insert a pin tx: %s", emoji.Warning, err.Error())
				} else {
					logger.Infof("\n%s  ~ Our PIN SYNC %s RESPONSE handled\n\n", emoji.CheckMarkButton, emoji.RoundPushpin)
				}
				// after we handle the response - destroy the state machine
				syncsm.deleteSM(syncTx.Tracking_Id)
			}
		case isSyncBalanceSnapshotRequest(syncTx, syncmgr.Leader):
			logger.Infof("%s  Handling Sync Balance Snaphsot Request", emoji.CheckBoxWithCheck)
			syncsm.resetSM(syncTx.Tracking_Id)
			handleSyncBalanceRequest(syncTx)
			syncsm.deleteSM(syncTx.Tracking_Id)
		case isSyncBalanceSnapshotResponse(syncTx):
			logger.Infof("%s  Handling Sync Balance Snaphsot Response, id %s", emoji.CheckBoxWithCheck, syncTx.Tracking_Id.String())
			syncsm.changeToSM(syncTx.Tracking_Id, sm.SYNC_HANDLE_BEGIN)
			handleSyncBalanceResponse(syncTx)
			syncsm.changeToSM(syncTx.Tracking_Id, sm.SYNC_HANDLE_END)
		case isPinDownloadRequest(syncTx, syncmgr.Leader):
			logger.Infof("%s  Handling Download Pins Request", emoji.CheckBoxWithCheck)
			syncsm.resetSM(syncTx.Tracking_Id)
			respondAllPinsFrom(syncTx)
			syncsm.deleteSM(syncTx.Tracking_Id)
		case isPinDownloadResponse(syncTx):
			logger.Infof("%s  Handling Download Pins Response, id %s", emoji.CheckBoxWithCheck, syncTx.Tracking_Id.String())
			syncsm.changeToSM(syncTx.Tracking_Id, sm.SYNC_HANDLE_BEGIN)
			handleDownloadedPinsFromLeader(syncTx)
			syncsm.changeToSM(syncTx.Tracking_Id, sm.SYNC_HANDLE_END)
		default:
			logger.Infof("%s  Handling Default message", emoji.CheckBoxWithCheck)
			if syncsm.existsSM(syncTx.Tracking_Id) {
				if syncsm.currentSM(syncTx.Tracking_Id) == sm.SYNC_DISPATCH_END {
					utils.ColorizeError(logger, "[dag sync sub] [!] UNHANDLED [%s] Msg:%s Type:%s \n%s",
						syncTx.Tracking_Id, syncTx.Msg_Type, syncTx.Sync_Type, syncTx.String())
				}
			} else {
				logger.Warnf("%s  ~ Message %s not intended for us. Ignoring...", emoji.Warning, syncTx.Tracking_Id)
			}
		}
		// }
	}
	logger.Infof("%s  ~ Sync Subscriber stopped", emoji.VerticalTrafficLight)
}

func processPin(pin *pb.TxPin) {
	_pins_.LockPin()
	defer _pins_.UnlockPin()
	pinJson, _ := pin.MarshalJSONShort()
	logger.Infof("=> [dag sync sub] Received a new pin tx : \n%s\n", string(pinJson))
	currentHeight := _pins_.CurrentHeight()
	if int(currentHeight+1) == int(pin.PinNumber) {
		logger.Infof("[No gaps detected] Process latest pin from leader as our latest pin=%d", pin.PinNumber)
		_pins_.SyncPins(pin)
		walletCache.copyFrom(walletCacheConfirmed)
		_pins_.pins = append(_pins_.pins, pin)
	} else {
		logger.Warnf("[Gap detected] Our current latest pin=%d, but got pin=%d from leader, pin downloading required", currentHeight, pin.PinNumber)
		_pins_.openPinDownloading()
		err := sendPindDownloadRequest(int(currentHeight) + 1)
		if err != nil {
			logger.Errorf("[Gap detected] downloading missing pins to catch up with leader: %s, will try again on next leader's new pin announce", err.Error())
		} else {
			timer := time.After(time.Second * 120)
			loggedWhenFirstDownloadedPinReceived := false
		pinProcessing:
			for {
				select {
				case downloadedPin, closed := <-_pins_.downloadedPins:
					if closed {
						logger.Infof("[Gap detected] No downloaded pins left to process")
						break pinProcessing
					}
					if !loggedWhenFirstDownloadedPinReceived {
						logger.Infof("[Gap detected] Start downloaded pin processing")
						loggedWhenFirstDownloadedPinReceived = true
					}
					if int(downloadedPin.PinNumber) == int(currentHeight+1) {
						logger.Infof("[Gap detected] Process downloaded pin at height=%d", currentHeight+1)
						_pins_.SyncPins(downloadedPin)
						_pins_.pins = append(_pins_.pins, downloadedPin)
						currentHeight = currentHeight + 1
					} else {
						logger.Errorf("[Gap detected] Downloaded pin=%d is out of order, required %d, exit downloading loop", downloadedPin.PinNumber, currentHeight+1)
						break pinProcessing
					}
				case <-timer:
					logger.Errorf("Gap detected] Timeout waiting for missing pin response from leader")
					break pinProcessing
				}
			}
			logger.Infof("[Gap detected] Processed pins up to %d height", currentHeight)
			walletCache.copyFrom(walletCacheConfirmed)
		}
	}

}
