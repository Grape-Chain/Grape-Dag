package dag

import (
	"fmt"
	"sync"
	"time"

	"github.com/VG-Grape/luna/config"
	lunapeer "github.com/VG-Grape/luna/peer"
	txqueue "github.com/VG-Grape/luna/queues"
	sm "github.com/VG-Grape/luna/statemachine"
	"github.com/VG-Grape/luna/tx"
	"github.com/enescakir/emoji"
	"github.com/google/uuid"
	"github.com/libp2p/go-libp2p/core/crypto"
)

func isPublishSyncUpRequest(t *tx.Syncv1) bool {
	return t.Msg_Type == tx.STX_SITE_REQUEST && t.Sync_Type == tx.GENESIS
}

func syncUpSites(dsm *DagSyncMngr, wg *sync.WaitGroup) {
	defer wg.Done()
	// in case we stuck here while a global shutdown is issued
	for !dsm.StopFlag.Load() {
		requestSitesFromLeader(dsm)
		t := time.NewTimer(time.Second * config.ANNOUNCE_TXT_PERIOD)
		<-t.C
		if dsm.SitesProcessed.Load() {
			logger.Infof("%s  ~ Site synchronization processed. READY", emoji.Compass)
			break
		}
		if config.GetConfig().Host.Verbose > 1 {
			logger.Warnf("Have not received a site sync response Re-announcing %s ...", emoji.Loudspeaker)
		}
	}
}

func requestSitesFromLeader(dsm *DagSyncMngr) error {
	host := lunapeer.GetHost()
	hostpk := host.Peerstore().PrivKey(host.ID())
	stdkey, _ := crypto.PrivKeyToStdKey(hostpk)
	pk, pubkey, err := crypto.KeyPairFromStdKey(stdkey)
	if err != nil {
		return fmt.Errorf("failed to obtain pubkey. %s", err.Error())
	}

	// prepare for announcement the latest pin tx
	stx := tx.NewSyncv1()
	stx.Ver_type = tx.STVX1
	stx.Sync_Type = tx.GENESIS
	stx.Msg_Type = tx.STX_SITE_REQUEST
	stx.Tracking_Id = uuid.New()
	stx.Sender_Pubk, err = crypto.MarshalPublicKey(pubkey)
	if err != nil {
		return fmt.Errorf("error marshaling public key: %s", err.Error())
	}
	stx.Data = []byte{}
	stx.SyncHash = []byte{}
	stx.Signature = stx.GenerateSignature(pk)

	syncsm.resetSM(stx.Tracking_Id)
	// Prepare a state machine to handle our sync sequence
	syncsm.changeToSM(stx.Tracking_Id, sm.SYNC_DISPATCH_BEGIN)
	// we publish sync tx to a sync queue
	txqueue.GetSyncQueue().Enqueue(stx)
	syncsm.waitForSM(stx.Tracking_Id, sm.SYNC_DISPATCH_END, 1000)
	return nil
}
