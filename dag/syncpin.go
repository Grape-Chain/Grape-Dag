package dag

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/VG-Grape/luna/config"
	"github.com/VG-Grape/luna/statemachine"
	"github.com/VG-Grape/luna/tx"
	"github.com/VG-Grape/luna/tx/pb"
	"github.com/enescakir/emoji"
	"github.com/golang/protobuf/proto"
	"github.com/libp2p/go-libp2p/core/crypto"
)

func SyncPin() {
	if config.GetConfig().Host.Verbose > 1 {
		logger.Infof("%s  ~ pin tx synchronization is running  %s", emoji.RoundPushpin, emoji.PersonRunning)
		logger.Infof("%s  ~ analyzing pin tx blockchain %s", emoji.RoundPushpin, emoji.MagnifyingGlassTiltedRight)
	}
	// analyze if our blockchain has gaps, and if it does, issue requests to obtain these pintxs
	GetPin().mu.Lock()
	defer GetPin().mu.Unlock()
	reconstruction := false
	for idx := len(GetPin().pins) - 1; idx >= 0; idx-- {
		if idx < 0 {
			break
		}
		if config.GetConfig().Host.Verbose > 1 {
			logger.Infof("%s  ~ pin[%03d] Sign %s -> Prev %s",
				emoji.RoundPushpin,
				idx,
				hex.EncodeToString(GetPin().pins[idx].Sign)[:10],
				func() string {
					if len(GetPin().pins[idx].Prev) > 0 {
						return hex.EncodeToString(GetPin().pins[idx].Prev)[:10]
					} else {
						return "NIL"
					}
				}(),
			)
		}

		prev := GetPin().pins[idx].Prev
		if len(prev) == 0 && idx > 0 {
			logger.Warnf("%s  ~ out of order pin tx[%d]", emoji.RoundPushpin, idx)
		}

		if len(prev) > 0 && idx == 0 {
			// previous transaction is missing, request it
			logger.Warnf("%s  ~ pin tx blockchain is incomplete. Requesting %s... ->", emoji.Link, hex.EncodeToString(prev)[:10])
			transactMissingPinRequest(prev)
			reconstruction = true
		} else if idx > 0 {
			sign := GetPin().pins[idx-1].Sign
			if len(prev) > 0 && !bytes.Equal(prev, sign) {
				logger.Infof("%s  ~ pin tx blockchain is incomplete. Requesting %s... ->", emoji.Link, hex.EncodeToString(prev)[:10])
				transactMissingPinRequest(prev)
				reconstruction = true
			}
		}
	}
	if config.GetConfig().Host.Verbose > 1 {
		if reconstruction {
			logger.Warnf("%s  ~ pin tx blockchain is incomplete. %s Reconstructing...", emoji.Link, emoji.NutAndBolt)
		} else {
			if len(GetPin().pins) > 0 {
				logger.Infof("%s  ~ pin tx blockchain is up to date  %s", emoji.RoundPushpin, emoji.Chains)
			} else {
				logger.Warnf("%s  ~ pin tx blockchain is empty. %s Awaiting updates...", emoji.Link, emoji.HourglassNotDone)
			}
		}
	}
}

func SyncSnapshot() {
	if config.GetConfig().Host.Verbose > 1 {
		logger.Infof("%s  ~ balance snapshot synchronization is running  %s", emoji.RoundPushpin, emoji.PersonRunning)
	}
	for {
		if GetPin().IsReady() {
			logger.Infof("Hooray! Node has downloaded & applied Balances Snapshot from Leader. Exiting snapshot request loop...")
			break
		}
		err, trackingId := sendSnapshotRequest()
		if err != nil {
			logger.Errorf("spawning balance snapshot request: %s", err.Error())
			continue
		}
		logger.Infof("Balance snapshot request=%s has been successfully sent", trackingId.String())
		_, err = syncsm.waitForSMInLoop(*trackingId, statemachine.SYNC_HANDLE_END, time.Second*120)
		if err != nil {
			logger.Errorf("awaiting for balance snapshot successful application: %s", err.Error())
			continue
		}
		logger.Infof("Snapshot Balances have been successfully received & applied")
		syncsm.deleteSM(*trackingId)
		break
	}

}

func processMissPinResponse(rec *tx.Syncv1) error {
	logger.Infof("%s  ~ process missing pin tx response [%s]",
		emoji.HammerAndWrench,
		rec.Tracking_Id,
	)
	pubkey, err := crypto.UnmarshalPublicKey(rec.Sender_Pubk)
	if err != nil {
		return fmt.Errorf("failed to unmarshal public key. err: %s", err.Error())
	}
	verify, err := rec.VerifySignature(pubkey)
	if err != nil {
		return fmt.Errorf("failed to verify signature. err: %s", err.Error())
	}
	if !verify {
		return fmt.Errorf("failed to verify signature")
	}
	pin := &pb.TxPin{}
	err = proto.Unmarshal(rec.Data, pin)
	if err != nil {
		return fmt.Errorf("failed to unmarshal pin tx. err: %s", err.Error())
	}
	// let's insert this pin where it belongs
	GetPin().insertIfNotFound(pin)
	return nil
}

func syncUpPin(dsm *DagSyncMngr, announceWg *sync.WaitGroup) {
	defer announceWg.Done()
	// at this point we are at the last known pin tx
	// See if there are other nodes that are already running and have the latest DAG
	// This call will help this node to sync up to the latest DAG state
	// in case we stuck here while a global shutdown is issued
	for !dsm.StopFlag.Load() {
		announceLatestPin()
		t := time.NewTimer(time.Second * config.ANNOUNCE_TXT_PERIOD)
		<-t.C
		if dsm.HaveJoined.Load() {
			logger.Infof("%s  ~ Our pin tx announcement has been acknowledged", emoji.RoundPushpin)
			break
		}
		if config.GetConfig().Host.Verbose > 1 {
			logger.Warnf("%s  ~ Have not received a response Re-announcing %s ...", emoji.Warning, emoji.Loudspeaker)
		}
	}

}

func (s *DagSyncMngr) syncPin(wg *sync.WaitGroup, stop_ch <-chan bool) {
	logger.Infof("%s Sync Pin is running %s ->", emoji.RoundPushpin, emoji.PersonRunning)
	defer wg.Done()
	t := time.NewTicker(time.Second * 15)
	defer t.Stop()
	for {
		select {
		case <-stop_ch:
			logger.Infof("%s  ~ Sync Pin stopped  %s", emoji.VerticalTrafficLight, emoji.RoundPushpin)
			return
		case <-t.C:
			SyncPin()
		}
	}
}

func (s *DagSyncMngr) syncSnapshot(wg *sync.WaitGroup, stop_ch <-chan bool) {
	logger.Infof("%s Sync Snapshot is running %s ->", emoji.RoundPushpin, emoji.PersonRunning)
	defer wg.Done()
	SyncSnapshot()
}
