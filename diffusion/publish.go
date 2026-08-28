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

// maxPublishAttempts - how many times one transaction is offered to the graph
// before it is dropped.
//
// The refusals this exists for are races, not verdicts: tip selection can pick
// sites that get settled before the new site is linked, which was measured at
// five refusals in two hundred and forty inserts under aggressive slicing. Those
// succeed on the next attempt, so a small number is enough.
//
// It is small rather than large because the alternative failure is worse. A
// transaction that can never be inserted - a balance that will not cover it, say
// - would be retried forever, and the publisher is the only consumer of the
// publish queue, so it would stall every transaction behind it indefinitely.
// Three attempts, and then a counted drop.
const maxPublishAttempts = 3

func publish(ctx context.Context, topic *pubsub.Topic, statsId uuid.UUID, leader bool) {
	//runtime.LockOSThread()
	//defer runtime.UnlockOSThread()
	var rec *tx.GrapeTx = nil
	source := &publishSource{queue: txqueue.GetPublishQueue()}
	// var yield_time int64 = config.PUBSUB_QUEUE_YIELD_MIN
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
		deq, sz := source.next()
		if deq != nil {
			rec = tx.NewGrapeTx(grapepeer.GetHost())
			rec.Transaction = deq
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
			// Not a race: the transaction is from another network, or there is no
			// graph yet. Retrying would refuse it again, so it is dropped here
			// and counted rather than held.
			logger.Errorf("[PUB] Tx \n%s\n cannot be made into a site. Dropped", rec.Transaction.String())
			source.drop(dropUnusable)
			continue
		}
		rec.Ids = tx.Ids{}
		ids, signatures, err := dag.GetDag().AddTxDag(dagNode)
		// AddTxDag signs the site as ours, so the claim exists only after it
		// returns. Copied onto the record here because a subscribing peer
		// builds its own site from the transaction and never sees this one.
		rec.ProcessorAddress, rec.ProcessorPk, rec.ProcessorSig = dag.ProcessorAttribution(dagNode)
		if err != nil {
			if !source.refused(rec.Transaction) {
				// Out of attempts. Dropping an accepted payment is bad; dropping
				// it silently is worse, and stalling every transaction behind it
				// forever is worse still.
				logger.Errorf("[PUB] Tx \n%s\n refused %d times. Dropped. %s",
					rec.Transaction.String(), maxPublishAttempts, err)
				continue
			}
			logger.Warnf("[PUB] Tx %s refused (attempt %d of %d), retrying. %s",
				dagNode.GetID().String(), source.attempts, maxPublishAttempts, err)
			// Yield rather than spin: the refusals this retries are lost races
			// with another goroutine, so the next attempt wants that goroutine
			// to have run.
			runtime.Gosched()
			continue
		}
		// Inserted. Anything held for a retry is done with.
		source.accepted()
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
			// Was logged and then published anyway, which sends peers a payload
			// they cannot parse. The site is already in our graph by this point,
			// so the transaction is not lost - it is unannounced, and peers will
			// ask for it through the missing-target path. Counted because an
			// unannounced site is a divergence between this node and the network.
			logger.Errorf("[PUB] Failed to marshal a tx record \n%s\n err:%s", rec.Transaction.String(), err.Error())
			stats.TxPublishDropped.WithLabelValues(dropMarshal).Inc()
			continue
		}
		// Built per publish. This was declared outside the loop and appended to
		// on every transaction, so it grew by one option per transaction for
		// the life of the process and every Publish was handed the whole list -
		// a leak, and a cost that rose with the number of transactions the node
		// had ever sent.
		pubopt := []pubsub.PubOpt{}
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

// Drop reasons. Also the reason label on grape_tx_publish_dropped_total.
const (
	// dropUnusable - the transaction could not be made into a site at all.
	dropUnusable = "unusable"
	// dropInsertRefused - the graph refused it maxPublishAttempts times.
	dropInsertRefused = "insert_refused"
	// dropMarshal - the site exists locally but could not be serialised, so no
	// peer will hear about it from us.
	dropMarshal = "marshal"
	// dropNotATransaction - something that is not a transaction was put on the
	// publish queue. A programming error rather than a network one, and the
	// alternative to counting it is a panic on the publisher goroutine.
	dropNotATransaction = "not_a_transaction"
)

// publishSource - the publisher's supply of transactions, and the one held back
// for another attempt.
//
// A type rather than two locals because the retry state is exactly what was
// wrong. A refused insert used to be a bare continue after the dequeue: the
// transaction had already left the queue, so a payment a client had been told
// was accepted simply vanished, with nothing counting it. Here there are two
// states - holding one for another attempt, or not - and every way out of an
// attempt has to say which.
//
// Refused transactions are held here rather than pushed back onto the queue. A
// re-enqueue was the obvious fix and is wrong twice over: it puts the
// transaction behind everything that has arrived since, and the publisher is the
// queue's only consumer, so re-enqueueing onto a queue at its ceiling would
// block the one goroutine that could drain it.
type publishSource struct {
	queue    *txqueue.LockFreeQueue
	pending  tx.Transaction
	attempts int
}

// next - the transaction to work on, and the queue depth behind it.
//
// Nothing is taken off the queue while one is being retried, so a refused
// transaction cannot be lost between attempts. Returns nil when there is
// nothing to do.
func (s *publishSource) next() (tx.Transaction, int64) {
	if s.pending != nil {
		return s.pending, s.queue.Len()
	}
	deq, sz := s.queue.Dequeue()
	if deq == nil {
		return nil, sz
	}
	t, ok := deq.(tx.Transaction)
	if !ok {
		logger.Errorf("[PUB] The publish queue held a %T, which is not a transaction. Dropped", deq)
		stats.TxPublishDropped.WithLabelValues(dropNotATransaction).Inc()
		return nil, sz
	}
	return t, sz
}

// refused - the graph would not take this transaction. Returns true if it will
// be offered again, false if it has now been dropped.
func (s *publishSource) refused(t tx.Transaction) bool {
	s.attempts++
	if s.attempts >= maxPublishAttempts {
		s.drop(dropInsertRefused)
		return false
	}
	s.pending = t
	stats.TxPublishRetries.Inc()
	return true
}

// drop - give up on this transaction, counted under reason so that a lost
// payment leaves a number behind.
func (s *publishSource) drop(reason string) {
	stats.TxPublishDropped.WithLabelValues(reason).Inc()
	s.pending, s.attempts = nil, 0
}

// accepted - the transaction reached the graph.
func (s *publishSource) accepted() {
	s.pending, s.attempts = nil, 0
}
