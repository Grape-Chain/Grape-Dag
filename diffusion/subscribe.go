package diffusion

import (
	"bytes"
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/Grape-Chain/Grape-Dag/config"
	"github.com/Grape-Chain/Grape-Dag/dag"
	txqueue "github.com/Grape-Chain/Grape-Dag/queues"
	"github.com/Grape-Chain/Grape-Dag/stats"
	"github.com/Grape-Chain/Grape-Dag/tx"
	utils "github.com/Grape-Chain/Grape-Dag/utils"
	"github.com/google/uuid"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
)

type SubInTxCmd uint8

const (
	SUBINTX_INSERT SubInTxCmd = iota
	SUBINTX_STOP
)

type SubInTx struct {
	cmd       SubInTxCmd
	vertex    *dag.Node
	id        uuid.UUID
	idMajor   uint64
	idMinor   uint32
	targetIds []tx.UuidSlice
}

var (
	// subintxq *txqueue.BasicQueue = txqueue.NewBasicQueue()
	// subintxq = qv2.NewLockfreeQueue()
	subintxq *txqueue.LockFreeQueue = nil
)

func subscribe(subscriber *pubsub.Subscription,
	ctx context.Context,
	hostID peer.ID,
	statsId uuid.UUID,
	bs bool,
	synch bool,
	leader bool) {
	subintxq = txqueue.NewLockFreeQueue(true)
	// as part of the subscription logic we want to handle out of sync reconciliations
	// signal to the goroutine to stop with this channel
	ch := make(chan bool)
	// wait group to sync coordination upon termination
	wg := sync.WaitGroup{}
	wg.Add(1)
	// run reconciliation logic on a separate goroutine
	go func() {
		// ticker runs every second (?) - is this enough or too much/little?
		t := time.NewTicker(time.Second)
		defer t.Stop()
		defer wg.Done()
		for {
			select {
			// channel to indicate stop condition
			case <-ch:
				return
			case <-t.C:
				// try to reconcile missing targets in dag
				dag.GetDag().ReconcileMissingTargets()
			}
		}
	}()
	wg.Add(1)
	go runInsert(&wg, leader)
	//runtime.LockOSThread()
	//defer runtime.UnlockOSThread()
	//var yield_time int64 = config.PUBSUB_QUEUE_YIELD_MIN
	for {
		msg, err := subscriber.Next(ctx)
		if err != nil {
			logger.Infof("Subscription for topic %s cancelled. %s", subscriber.Topic(), err.Error())
			break
		}
		// only consider messages delivered by other peers
		if msg.ReceivedFrom == hostID {
			continue
		}
		rec_bytes := msg.Data
		rec := &tx.GrapeTx{}
		err = rec.UnmarshalRecord(rec_bytes)
		if err != nil {
			logger.Errorf("Failed to unmarshal tx. err: %s", err.Error())
		}

		// _, err = record.ConsumeTypedEnvelope(rec_bytes, rec)
		// if err != nil {
		// 	logger.Errorf("[SUB] Failed to consume message. %v", err)
		// }
		rec.PeerID = msg.ReceivedFrom
		//  time check
		td := time.Since(time.UnixMilli(int64(rec.Transaction.GetTimestamp())))
		if td.Seconds() > config.PUBSUB_DELAY {
			logger.Errorf("[SUB] Tx %d.%d running with %f sec delay...", rec.TxIdMajor, rec.TxIdMinor, td.Seconds())
		}
		dagNode := dag.NewDagNode(rec.Transaction, true)
		if dagNode == nil {
			logger.Warnf("[SUB] Tx \n%s\n cannot be verified. Discrarded", rec.Transaction.String())
			continue
		}
		txUUID := uuid.MustParse(rec.Tx)
		// secondary queue for inbound tx - we want to prevent tx loss,
		// we try to accept as many incoming tx as we possibly can
		subintxq.Enqueue(SubInTx{
			cmd:       SUBINTX_INSERT,
			vertex:    dagNode,
			id:        txUUID,
			idMajor:   rec.TxIdMajor,
			idMinor:   rec.TxIdMinor,
			targetIds: rec.Ids.IDs,
		})
		// enqueu to the db queue for processing on a separate go routine
		stats.Enqueue(statsId, rec, stats.TX_TYPE_SUB, 0, td)
	}
	// terminate the secondary queue processing routine
	subintxq.Enqueue(SubInTx{cmd: SUBINTX_STOP})
	ch <- true
	wg.Wait()
	logger.Info("Subscriber has stopped")
}

func runInsert(wg *sync.WaitGroup, leader bool) {
	//runtime.LockOSThread()
	//defer runtime.UnlockOSThread()
	defer wg.Done()
	// var yield_time int64 = config.PUBSUB_QUEUE_YIELD_MIN
	for {
		qmsg, sz := subintxq.Dequeue()
		if qmsg == nil {
			// queue is empty - wait for a new entry
			// We keep incrementing until we reach max yeild time
			//time.Sleep(time.Microsecond * time.Duration(yield_time))
			// if yield_time < config.PUBSUB_QUEUE_YIELD_MAX {
			// 	yield_time += config.PUBSUB_QUEUE_YEILD_INCR
			// }
			runtime.Gosched()
			continue
		}
		// yield_time = config.PUBSUB_QUEUE_YIELD_MIN
		tx := qmsg.(SubInTx)
		if tx.cmd == SUBINTX_STOP {
			logger.Info("Stopping the subscriber inbound tx queue processing...")
			break
		}
		if sz >= 500 {
			logger.Warnf("DAG Insert queue size is critically high: %d. Reduce the transaction rate to avoid data loss", sz)
		}
		err := dag.GetDag().InsertTxDag(tx.vertex, tx.id, tx.idMajor, tx.idMinor, tx.targetIds...)
		if err != nil {
			logger.Warnf("[SUB] Tx \n%s\n added with deferred source tx. %s", tx.vertex.String(), err.Error())
			continue
		}
		buf := bytes.Buffer{}
		for _, id := range tx.targetIds {
			buf.WriteString(id.Id.String() + " ")
		}
		if leader {
			dag.GetDag().SyncUp()
		}

		utils.ColorizeInfo(logger, "[SUB] Tx:%s Approves:[ %s]", tx.id.String(), buf.String())
	}
}
