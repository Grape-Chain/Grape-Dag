package diffusion

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
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
	// The insert queue is typed rather than interface{}-based: every item is a
	// SubInTx, and a queue of interface{} allocated twice per item - once to box
	// the struct and once for the queue's holder.
	subintxq *txqueue.LockFreeQueueOf[SubInTx] = nil
)

/*
Verification concurrency.

Every received record costs an ed25519 verify, which was a quarter of the
node's CPU and ran on the one goroutine that also read from the subscription
and built sites. The node sat at 35% CPU on four cores with 69% of its blocking
in select and 25% blocked sending on a channel: not short of CPU, short of
concurrency.

The pipeline is:

	subscriber.Next -> [verifyWorkers goroutines] -> collector -> insert queue

Verification is pure CPU over independent messages, so it fans out. Building
the site does not: dag.NewDagNode increments dag-wide counters without
synchronisation, so it stays on the single collector goroutine. See the note on
buildSite.

The collector delivers in arrival order. Nothing downstream requires it -
InsertTxDag records approvals it cannot resolve in missingTargets and a ticker
reconciles them later, which is the mechanism for out-of-order arrival, and
gossip offers no ordering across peers in any case - but ordered delivery costs
one bounded channel and removes a whole class of question from the diff.
*/
const (
	// verifyWorkersMax - the ceiling on the fan-out. Verification is CPU-bound,
	// so past the number of cores more workers buy nothing and each one holds a
	// record in flight. The cap exists because GOMAXPROCS on a large host would
	// otherwise put a large multiple of that in flight.
	verifyWorkersMax = 16
	// verifyWorkersMin - two rather than one, so that the fan-out is real even
	// on a single-core container: the verify is long enough that overlapping it
	// with the collector's work is worth a context switch.
	verifyWorkersMin = 2
	// inFlightPerWorker - how deep the pipeline is allowed to get, per worker.
	// This is the memory bound: an unbounded goroutine or channel per message
	// is a memory attack from the network, since the sender chooses the rate.
	// Four is enough that a worker finishing has something to pick up without
	// waiting for the reader, and small enough that the in-flight set stays in
	// the tens of records.
	inFlightPerWorker = 4
	// subBufferSize - the subscription's own buffer, in messages.
	//
	// pubsub.WithBufferSize is documented as "the size of the subscribe output
	// buffer", default 32, and it allocates the channel up front. It was
	// 1<<20 here: 1,048,576 message pointers, 8MB of channel, allocated at
	// start-up and never smaller. 4096 covers over a second of the measured
	// arrival rate, which is far longer than any scheduling hiccup, and costs
	// 32KB. Past that, pubsub drops for this subscription, which is what
	// backpressure means and is recoverable: a site whose approvals are missing
	// is exactly the case missingTargets and ReconcileMissingTargets exist for.
	subBufferSize = 1 << 12
)

// verifyWorkers - how many goroutines check signatures in parallel.
//
// GRAPE_VERIFY_WORKERS overrides it for a process. Read from the environment
// rather than the yml because the right number is a property of the machine the
// node runs on, not of the network it joins.
func verifyWorkers() int {
	n := runtime.GOMAXPROCS(0)
	if env := os.Getenv("GRAPE_VERIFY_WORKERS"); env != "" {
		if parsed, err := strconv.Atoi(env); err == nil && parsed > 0 {
			n = parsed
		}
	}
	if n < verifyWorkersMin {
		n = verifyWorkersMin
	}
	if n > verifyWorkersMax {
		n = verifyWorkersMax
	}
	return n
}

// verifiedRecord - a gossip record whose transaction signature has been checked.
//
// The only thing that produces one with a non-nil rec is verifyRecord, and
// buildSite refuses one whose checkedSig does not match the transaction it
// carries. The point is that "has this been verified" is answered by the value
// rather than by reading the call chain, because the call chain is now three
// goroutines long.
//
// The zero value means "rejected", which is why rejection needs no separate
// flag travelling alongside.
type verifiedRecord struct {
	rec   *tx.GrapeTx
	delay time.Duration
	// checkedSig - a copy of the signature that was actually verified.
	//
	// A bool would say "something was verified"; this says "this transaction
	// was verified". buildSite compares it against the transaction it is about
	// to encapsulate, so a record swapped between verification and build is
	// dropped rather than inserted. Copied, not aliased: comparing a slice
	// against itself would make the check vacuous.
	checkedSig []byte
}

// verifyJob - one message on its way through the pipeline.
type verifyJob struct {
	raw  []byte
	from peer.ID
	// out - where the worker leaves its answer. Capacity one, so a worker never
	// blocks handing back a result. That is what keeps the ordered collector
	// from being able to deadlock the pool: a worker always finishes the job it
	// took, whatever the collector is doing.
	out chan verifiedRecord
}

// jobs are pooled because at a few thousand messages a second the job and its
// result channel are pure garbage otherwise. The collector is the last owner of
// a job, so it is the only thing that returns one.
var verifyJobPool = sync.Pool{
	New: func() any {
		return &verifyJob{out: make(chan verifiedRecord, 1)}
	},
}

func subscribe(subscriber *pubsub.Subscription,
	ctx context.Context,
	hostID peer.ID,
	statsId uuid.UUID,
	bs bool,
	synch bool,
	leader bool) {
	subintxq = txqueue.NewLockFreeQueueOf[SubInTx](true)
	// Depth, ceiling and backpressure of the queue between verification and the
	// graph. Without this the difference between what ingress accepts and what
	// the graph inserts is invisible until the queue is full.
	stats.RegisterQueue("insert", subintxq)
	stats.RegisterQueue("publish", txqueue.GetPublishQueue())

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

	pipeline := startVerifyPipeline(verifyWorkers(), func(v verifiedRecord) {
		buildSite(v, statsId)
	})

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
		pipeline.submit(msg.Data, msg.ReceivedFrom)
	}
	pipeline.stop()
	// terminate the secondary queue processing routine
	subintxq.Enqueue(SubInTx{cmd: SUBINTX_STOP})
	ch <- true
	wg.Wait()
	logger.Info("Subscriber has stopped")
}

// verifyPipeline - the bounded, ordered fan-out.
type verifyPipeline struct {
	// order carries every submitted job in arrival order, and its capacity is
	// the in-flight bound.
	order chan *verifyJob
	// jobs is unbuffered: a job is handed to whichever worker is free, and when
	// none is, the submitter waits. That is the backpressure that stops the
	// pipeline growing past the bound.
	jobs        chan *verifyJob
	workers     int
	workerWg    sync.WaitGroup
	collectorWg sync.WaitGroup
}

// startVerifyPipeline - start the workers and the collector.
//
// deliver is called once for every accepted record, on the collector goroutine
// and in arrival order, so it may touch state that is not safe to share. It is
// not called for a record that failed verification.
func startVerifyPipeline(workers int, deliver func(verifiedRecord)) *verifyPipeline {
	if workers < 1 {
		workers = 1
	}
	p := &verifyPipeline{
		workers: workers,
		order:   make(chan *verifyJob, workers*inFlightPerWorker),
		jobs:    make(chan *verifyJob),
	}
	stats.SubVerifyWorkers.Set(float64(workers))
	for i := 0; i < workers; i++ {
		p.workerWg.Add(1)
		go func() {
			defer p.workerWg.Done()
			for job := range p.jobs {
				job.out <- verifyRecord(job.raw, job.from)
			}
		}()
	}
	p.collectorWg.Add(1)
	go func() {
		defer p.collectorWg.Done()
		p.collect(deliver)
	}()
	return p
}

// submit - hand one received message to the pipeline, waiting if it is full.
func (p *verifyPipeline) submit(raw []byte, from peer.ID) {
	job := verifyJobPool.Get().(*verifyJob)
	job.raw = raw
	job.from = from
	stats.SubVerifyInFlight.Inc()
	// order before jobs, deliberately. order is the bounded one, so the
	// submitter is held there when the collector falls behind; a job that has
	// reached order is always then handed to a worker, so the collector waiting
	// on that job's result cannot wait forever.
	p.order <- job
	p.jobs <- job
}

// maxInFlight - the most records that can be between submit and delivery.
//
// cap(order) sitting in the ordered channel, one that a blocked submitter is
// holding, and one the collector has taken out and is delivering. This is the
// memory bound: without it, a goroutine or a channel slot per message would let
// whoever is sending choose how much memory the node uses.
func (p *verifyPipeline) maxInFlight() int { return cap(p.order) + 2 }

// stop - drain the pipeline and wait for it.
//
// Shut down from the submitting end so that nothing in flight is dropped:
// closing jobs lets the workers finish what they have taken, and only once they
// have all returned is order closed, by which point every job still in it has
// its result waiting.
func (p *verifyPipeline) stop() {
	close(p.jobs)
	p.workerWg.Wait()
	close(p.order)
	p.collectorWg.Wait()
}

// collect - take verified records in arrival order and hand them on.
func (p *verifyPipeline) collect(deliver func(verifiedRecord)) {
	for job := range p.order {
		v := <-job.out
		job.raw = nil
		verifyJobPool.Put(job)
		stats.SubVerifyInFlight.Dec()
		// The zero verifiedRecord is a rejection, and verifyRecord has already
		// logged and counted the reason.
		if v.rec == nil {
			continue
		}
		// The gate again, here rather than only inside deliver. Two places
		// enforce it - where a record is verified and where it is handed on -
		// so that editing either half alone cannot open it. Unreachable as the
		// pipeline stands, which is the point of checking.
		if !v.verifiedFor() {
			logger.Errorf("[SUB] A record reached the collector without having been verified. Discarded")
			stats.SubVerifyRejected.WithLabelValues("unverified").Inc()
			continue
		}
		deliver(v)
	}
}

// verifyRecord - unmarshal a received record and check its transaction's
// signature.
//
// This is the whole of the expensive work on the receive path and the whole of
// the trust decision, in one function on purpose: there is no way to get a
// record out of here without the signature having been checked, and no second
// route into buildSite.
//
// Runs on a worker goroutine, so it must touch nothing shared. It does not: the
// record is built from bytes that pubsub allocated per message, and verification
// reads only the transaction.
func verifyRecord(raw []byte, from peer.ID) verifiedRecord {
	defer stats.Time(stats.SubVerify)()

	rec := &tx.GrapeTx{}
	if err := rec.UnmarshalRecord(raw); err != nil {
		// Previously logged and carried on, which then dereferenced a nil
		// Transaction. A peer choosing to send rubbish should cost us this
		// message and nothing else.
		logger.Errorf("Failed to unmarshal tx. err: %s", err.Error())
		stats.SubVerifyRejected.WithLabelValues("unmarshal").Inc()
		return verifiedRecord{}
	}
	if rec.Transaction == nil {
		// GrapeTxFromProtobuf leaves this nil for a record whose version it
		// does not know, and every use of it below is a method call. One
		// unrecognised version number was enough to stop the node.
		logger.Warnf("[SUB] Record from %s carries no usable transaction. Discarded", from.String())
		stats.SubVerifyRejected.WithLabelValues("no_transaction").Inc()
		return verifiedRecord{}
	}
	rec.PeerID = from

	// The gate. A record that does not verify here reaches nothing: the caller
	// gets the zero verifiedRecord and the collector drops it.
	if err := verifySignature(rec.Transaction); err != nil {
		// Deliberately cheap. This used to format the whole transaction with
		// Transaction.String() on every rejection, which is work a peer can ask
		// for by sending rubbish; the full dump is now behind the verbose flag.
		logger.Warnf("[SUB] Tx from %s cannot be verified. Discarded: %s", from.String(), err.Error())
		if config.GetConfig() != nil && config.GetConfig().Host.Verbose > 1 {
			logger.Warnf("[SUB] Refused tx \n%s\n", rec.Transaction.String())
		}
		stats.SubVerifyRejected.WithLabelValues("signature").Inc()
		return verifiedRecord{}
	}
	// Parsed here rather than in the collector because uuid.MustParse panics,
	// and rec.Tx is a string chosen by whoever sent the message.
	if _, err := uuid.Parse(rec.Tx); err != nil {
		logger.Warnf("[SUB] Record from %s carries an unusable tx id %q. Discarded", from.String(), rec.Tx)
		stats.SubVerifyRejected.WithLabelValues("tx_id").Inc()
		return verifiedRecord{}
	}

	//  time check
	td := time.Since(time.UnixMilli(int64(rec.Transaction.GetTimestamp())))
	if td.Seconds() > config.PUBSUB_DELAY {
		logger.Errorf("[SUB] Tx %d.%d running with %f sec delay...", rec.TxIdMajor, rec.TxIdMinor, td.Seconds())
	}

	return verifiedRecord{
		rec:        rec,
		delay:      td,
		checkedSig: append([]byte(nil), rec.Transaction.GetSignature()...),
	}
}

// verifySignature - run the transaction's own signature check, turning a panic
// into a refusal.
//
// crypto/ed25519.Verify panics on a public key that is not 32 bytes, and
// Ed25519DSA.Verify (crypto/dsa.go) only rejects an empty one, so a peer sending
// a transaction with a five-byte sender public key stopped the node with a
// single gossip message - on the subscriber goroutine before this change, and on
// a verify worker after it. Either way the process dies, because an unrecovered
// panic on any goroutine takes the whole node with it.
//
// Recovered here because this is the trust boundary: everything past it is data
// we have accepted, and the root cause is in crypto/dsa.go, which is not ours to
// change (see the report - dag/attribution.go already guards the identical case
// for the processor key, with the same reasoning). A panic becomes an error, so
// this can only refuse a transaction, never admit one.
func verifySignature(t tx.Transaction) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic while checking the signature: %v", r)
		}
	}()
	return t.VerifySignature()
}

// verifiedFor - is this record carrying the transaction whose signature was
// checked?
//
// The gate, as a single expression so that there is one place to read and one
// place to break. verifyRecord is the only thing that fills in checkedSig, and
// it does so only after tx.VerifySignature has returned nil, so an empty
// checkedSig means the record never went through the verifier. The comparison
// is what makes this a statement about this transaction rather than about some
// earlier one: a bool would still be true for a record swapped in afterwards.
func (v verifiedRecord) verifiedFor() bool {
	if len(v.checkedSig) == 0 || v.rec == nil || v.rec.Transaction == nil {
		return false
	}
	return bytes.Equal(v.checkedSig, v.rec.Transaction.GetSignature())
}

// buildSite - encapsulate a verified transaction into a site and queue it for
// insertion.
//
// Not fanned out, and not for want of trying: dag.NewDagNode increments
// _dag_.prevMajor without synchronisation, so concurrent calls are a data race
// on dag-wide state. Those version numbers are then overwritten by InsertTxDag
// from the ones on the record, so on this path the increment is pure waste as
// well as unsafe - but dag/node.go is not ours to change. Making prevMajor
// atomic there would let the whole build move onto the workers and take the
// remaining per-message work off this goroutine too.
func buildSite(v verifiedRecord, statsId uuid.UUID) {
	// The gate, checked where the site is actually made rather than only where
	// the record was accepted. Impossible to fail as the pipeline stands; here
	// because if a later change makes it possible, the site is dropped instead
	// of joining the graph, and an unverified transaction in the graph is the
	// worst outcome available.
	if !v.verifiedFor() {
		logger.Errorf("[SUB] Refusing a site whose transaction is not the one that was verified. Discarded")
		stats.SubVerifyRejected.WithLabelValues("unverified").Inc()
		return
	}
	rec := v.rec
	// Verification is not skipped by passing false here, it has already
	// happened: verifyRecord ran tx.VerifySignature on a worker goroutine, and
	// the check above is what ties that result to this transaction. Passing
	// true would put the ed25519 verify back on this single goroutine, which is
	// the entire cost this pipeline exists to move.
	dagNode := dag.NewDagNode(rec.Transaction, false)
	if dagNode == nil {
		// Still reachable: NewDagNode also refuses a transaction from another
		// network, whatever the verify flag says.
		logger.Warnf("[SUB] Tx \n%s\n cannot be verified. Discrarded", rec.Transaction.String())
		stats.SubVerifyRejected.WithLabelValues("build").Inc()
		return
	}
	// The claim travels on the record, because this is a site we built from
	// somebody else's transaction. Carried, not trusted: it is verified
	// where the site joins the graph, which is the first point at which its
	// approvals are known and the signature can be recomputed.
	dag.SetProcessorAttribution(dagNode, rec.ProcessorAddress, rec.ProcessorPk, rec.ProcessorSig)
	// Already parsed once in verifyRecord, so this cannot fail.
	txUUID, _ := uuid.Parse(rec.Tx)
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
	stats.Enqueue(statsId, rec, stats.TX_TYPE_SUB, 0, v.delay)
}

func runInsert(wg *sync.WaitGroup, leader bool) {
	defer wg.Done()
	// warnAt - the depth at which the queue is worth a line in the log. Left at
	// the value it had, but stated against the ceiling so the message says how
	// much room is left rather than just a number.
	const warnAt = 500
	for {
		qmsg, sz, ok := subintxq.TryDequeue()
		if !ok {
			// A synchronised queue blocks until it has something, so this is a
			// spurious wakeup rather than an idle queue. Yield and look again.
			runtime.Gosched()
			continue
		}
		if qmsg.cmd == SUBINTX_STOP {
			logger.Info("Stopping the subscriber inbound tx queue processing...")
			break
		}
		if sz >= warnAt {
			logger.Warnf("DAG Insert queue size is critically high: %d of %d. Reduce the transaction rate to avoid stalling the producers",
				sz, subintxq.Capacity())
		}
		err := dag.GetDag().InsertTxDag(qmsg.vertex, qmsg.id, qmsg.idMajor, qmsg.idMinor, qmsg.targetIds...)
		if err != nil {
			logger.Warnf("[SUB] Tx \n%s\n added with deferred source tx. %s", qmsg.vertex.String(), err.Error())
			continue
		}
		if leader {
			dag.GetDag().SyncUp()
		}
		// Built only when it will be printed. This ran a uuid.String() and a
		// string concatenation per approval on every insert, and then handed the
		// result to a logger that discards it below its level.
		if config.GetConfig().Host.Verbose > 0 {
			buf := bytes.Buffer{}
			for _, id := range qmsg.targetIds {
				buf.WriteString(id.Id.String() + " ")
			}
			utils.ColorizeInfo(logger, "[SUB] Tx:%s Approves:[ %s]", qmsg.id.String(), buf.String())
		}
	}
}
