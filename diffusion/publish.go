package diffusion

import (
	"bytes"
	"context"
	"runtime"
	"time"

	"github.com/Grape-Chain/Grape-Dag/config"
	"github.com/Grape-Chain/Grape-Dag/dag"
	"github.com/Grape-Chain/Grape-Dag/discovery"
	grapepeer "github.com/Grape-Chain/Grape-Dag/peer"
	txqueue "github.com/Grape-Chain/Grape-Dag/queues"
	"github.com/Grape-Chain/Grape-Dag/services/node"
	"github.com/Grape-Chain/Grape-Dag/stats"
	"github.com/Grape-Chain/Grape-Dag/tx"
	utils "github.com/Grape-Chain/Grape-Dag/utils"
	"github.com/enescakir/emoji"
	"github.com/google/uuid"
	"github.com/ledongthuc/goterators"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

// processingPausedPoll - how long the publisher waits before looking at the
// processing switch again. Long enough that a stopped node costs nothing, short
// enough that starting it feels immediate to whoever pressed the button.
const processingPausedPoll = 100 * time.Millisecond

func publish(ctx context.Context, topic *pubsub.Topic, statsId uuid.UUID, leader bool) {
	//runtime.LockOSThread()
	//defer runtime.UnlockOSThread()
	var rec *tx.GrapeTx = nil
	// var yield_time int64 = config.PUBSUB_QUEUE_YIELD_MIN
	pubopt := []pubsub.PubOpt{}
	for {
		// The NXT-forging switch. Checked before the dequeue, not after: a
		// stopped node that dequeued first would take transactions off the
		// queue and drop them, which is losing work rather than pausing it.
		// Sleeps rather than yielding, because a stopped node would otherwise
		// spin a core for as long as it stays stopped. The gate starts enabled,
		// so this changes nothing until somebody stops the node.
		if !node.ProcessingEnabled() {
			time.Sleep(processingPausedPoll)
			continue
		}
		deq, sz := txqueue.GetPublishQueue().Dequeue()
		if deq != nil {
			rec = tx.NewGrapeTx(grapepeer.GetHost())
			rec.Transaction = deq.(tx.Transaction)
			// yield_time = config.PUBSUB_QUEUE_YIELD_MIN
		} else {
			if sz > 0 {
				logger.Errorf("[PUB] Spurious wakeup on dequeuing. Tx may be lost. Queue size: %d", sz)
			}
			runtime.Gosched()
			// We keep incrementing until we reach max yeild time
			// time.Sleep(time.Microsecond * time.Duration(yield_time))
			// if yield_time < config.PUBSUB_QUEUE_YIELD_MAX {
			// 	yield_time += config.PUBSUB_QUEUE_YEILD_INCR
			// }
			continue
		}
		if sz > config.QUEUE_RELIEF_SIZE {
			logger.Errorf("[PUB] Queue size exceeds queue relief capacity [%d]. Current capacity is: %d", config.QUEUE_RELIEF_SIZE, sz)
		}
		if rec.Transaction.IsService() {
			if rec.Transaction.IsPayload(tx.TX_SERVICE_STOP) {
				// this is our cue from the grpc service to terminate
				break
			}
		}

		// construct a new node, verify if the transaction is valid
		dagNode := dag.NewDagNode(rec.Transaction, false)
		if dagNode == nil {
			logger.Warnf("[publish] Tx \n%s\n cannot be verified. Discrarded", rec.Transaction.String())
			continue
		}
		rec.Ids = tx.Ids{}
		ids, signatures, err := dag.GetDag().AddTxDag(dagNode)
		if err != nil {
			// if for whatever reason we failed to add to dag indicate it here
			logger.Warnf("[publish] Tx \n%s\n payment cannot be processed. %s", rec.Transaction.String(), err)
			continue
		}
		goterators.ForEach(ids, func(slice uuid.UUID) {
			empty := (slice == uuid.Nil)
			rec.Ids.IDs = append(rec.Ids.IDs, tx.UuidSlice{
				Is_empty: empty,
				Id:       slice,
			})
		})
		rec.Ids.Signatures = signatures
		rec.TxIdMajor, rec.TxIdMinor = dag.GetDag().GetNodeVer(dagNode)
		rec.Tx = dagNode.GetID().String()

		diff := time.Since(time.UnixMilli(int64(rec.Transaction.GetTimestamp())))

		if diff.Seconds() > 30.0 {
			logger.Errorf("[PUB] Queue:%d Tx:[%d.%d] running with delay %f sec...", sz, rec.TxIdMajor, rec.TxIdMinor, diff.Seconds())
		}

		// envelope, err := record.Seal(rec, grapepeer.GetHost().Peerstore().PrivKey(grapepeer.GetHost().ID()))
		// if err != nil {
		// 	utils.ColorizeError(logger, "Failed to seal the record. %v", err)
		// }
		// rec_bytes, _ := envelope.Marshal()

		payload, err := rec.MarshalRecord()
		if err != nil {
			logger.Errorf("Failed to marshal a tx record \n%s\n err:%s", rec.Transaction.String(), err.Error())
		}
		if !leader {
			const pub_threshold = 1
			pubopt = append(pubopt, pubsub.WithReadiness(pubsub.MinTopicSize(pub_threshold)))

			mesh := discovery.GetMesh().Get(topic.String())
			if config.GetConfig().Host.Verbose > 1 {
				if len(mesh) <= pub_threshold {
					logger.Warnf("%s  Most likely will not publish... [%d]", emoji.Warning, len(mesh))
				}
			}
		}
		// if err := topic.Publish(ctx, rec_bytes); err != nil {
		if err := topic.Publish(ctx, payload, pubopt...); err != nil {
			logger.Info("Topic closed. Stop publisher. ", err)
			//t.Stop()
			break
		} else {
			stats.Enqueue(statsId, rec, stats.TX_TYPE_PUB, sz, diff)

			// Built only when it will be printed: one uuid.String() and one
			// string concatenation per approval, on the publish path.
			if config.GetConfig().Host.Verbose > 0 {
				buf := bytes.Buffer{}
				for _, id := range rec.Ids.IDs {
					buf.WriteString(id.Id.String() + " ")
				}
				utils.ColorizeInfo(logger, "[PUB] Tx:%s Approves:[ %s]", rec.Tx, buf.String())
			}
			if leader {
				dag.GetDag().SyncUp()
			}
		}
		//t.Stop()
		txCount++
	}
	logger.Info("Publisher has stopped")
}
