package stats

import (
	"math/big"
	"runtime"
	"sync"
	"time"

	"github.com/VG-Grape/luna/config"
	"github.com/VG-Grape/luna/db"
	"github.com/VG-Grape/luna/db/base"
	txqueue "github.com/VG-Grape/luna/queues"
	"github.com/VG-Grape/luna/tx"
	"github.com/VG-Grape/luna/crypto"
	"github.com/google/uuid"
	golog "github.com/ipfs/go-log/v2"
)

type StatsDataType uint8

type StatsData struct {
	tx      *tx.LunaTx
	tx_type StatsDataType
	rctime  time.Time
	sz      int64
	diff    time.Duration
}

type StatsSession struct {
	dbmngr     base.DbManager
	statsWrite base.StatsWriter
	statsCh    chan bool
	statsQueue *txqueue.LockFreeQueue
	wg         *sync.WaitGroup
}

var (
	logger   golog.EventLogger = nil
	mu       sync.RWMutex
	sessions map[uuid.UUID]*StatsSession
)

const (
	TX_TYPE_PUB StatsDataType = iota
	TX_TYPE_SUB
)

func (tp StatsDataType) String() string {
	return []string{"PUB-TX", "SUB-TX"}[tp]
}

func init() {
	logger = golog.Logger("p2p-db-stats")
	mu = sync.RWMutex{}
	sessions = make(map[uuid.UUID]*StatsSession)
}

func NewStatsSession() uuid.UUID {
	ss := &StatsSession{}
	ss.dbmngr = db.Create(config.STATS_DB)
	if ss.dbmngr == nil {
		logger.Warn("Failed to init stats collection. Will continue without stats")
		return uuid.Nil
	}
	switch ss.dbmngr.(type) {
	case base.DbManager:
		if v, ok := ss.dbmngr.(base.StatsWriter); ok {
			ss.statsWrite = v
		}
	}

	ss.statsCh = make(chan bool)
	ss.statsQueue = txqueue.NewLockFreeQueue(true)
	session_id := uuid.New()
	ss.wg = &sync.WaitGroup{}
	ss.wg.Add(1)
	mu.Lock()
	sessions[session_id] = ss
	mu.Unlock()
	go ss.processStatsQueue()
	logger.Infof("New stats session has been created with session id %s", session_id.String())
	return session_id
}

func StopSession(id uuid.UUID) bool {
	mu.RLock()
	v, ok := sessions[id]
	if ok {
		// indicate to the stats routine to terminate
		v.statsCh <- true
		v.wg.Wait()
	}
	mu.RUnlock()
	logger.Infof("Stats session has been terminated. Status: %t", ok)
	return ok
}

func Enqueue(id uuid.UUID, rec *tx.LunaTx, tp StatsDataType, sz int64, dur time.Duration) {
	// if id != uuid.Nil {
	// mu.RLock()
	// defer mu.RUnlock()
	if id != uuid.Nil && rec.Transaction.GetTransactionType() == tx.PAYMENT {
		if v, ok := sessions[id]; ok {
			if sz == 0 {
				sz = v.statsQueue.Len()
			}
			v.statsQueue.Enqueue(StatsData{
				tx:      rec,
				tx_type: tp,
				rctime:  time.Now(),
				sz:      sz,
				diff:    dur,
			})
		}
	}
	// }
}

func (ss *StatsSession) processStatsQueue() {
	stopFlag := false
	defer ss.wg.Done()
	var bulk []interface{} = []interface{}{}
	//noActivityTime := time.Now()
	for {
		select {
		case <-ss.statsCh:
			stopFlag = true
			logger.Info("Received a request to stop. Waiting until all tx are written to db...")
			//return
		default:
			if v, _ := ss.statsQueue.Dequeue(); v != nil {
				sd := v.(StatsData)
				db_doc := TxToStats(&sd)
				// bulk = append(bulk, db_doc)
				// if len(bulk)%1000 == 0 {
				// 	t1 := time.Now()
				// 	if err := ss.statsWrite.WriteMany(bulk); err != nil {
				// 		logger.Errorf("Error writing bulk stats. err: %s", err.Error())
				// 	}
				// 	t2 := time.Now()
				// 	diff := t2.Sub(t1)
				// 	logger.Infof("Writing 1000 tx records took %04fsec. Queue sz: %d\n", diff.Seconds(), sz)
				// 	bulk = []interface{}{}
				// }
				// break
				ss.statsWrite.Write(db_doc)
				break
			} else if stopFlag {
				if len(bulk) > 0 {
					if err := ss.statsWrite.WriteMany(bulk); err != nil {
						logger.Errorf("Error writing bulk stats on exit. err: %s", err.Error())
					}
				}
				logger.Info("All tx have been processed. Will terminate now.")
				return
			}
			// at this point the queue may be empty but we may still have some records stored in cache
			// unload them
			// t := time.Now()
			// diff := t.Sub(noActivityTime)
			// if diff.Seconds() > time.Duration(time.Second*30).Seconds() {
			// 	noActivityTime = time.Now()
			// 	if len(bulk) > 0 {
			// 		if err := ss.statsWrite.WriteMany(bulk); err != nil {
			// 			logger.Errorf("Error writing bulk stats. err: %s", err.Error())
			// 		}
			// 		bulk = []interface{}{}
			// 	}
			// }
			runtime.Gosched()
		}
	}
}

func TxToStats(sd *StatsData) *base.TxStats { //} tx *tx.LunaTx, tx_type StatsDataType, rctime time.Time) *base.TxStats {
	if sd.tx != nil {
		txs := &base.TxStats{}
		switch sd.tx_type {
		case TX_TYPE_PUB:
			txs.PeerID = string([]byte(sd.tx.PeerID))
		case TX_TYPE_SUB:
			txs.PeerID = sd.tx.PeerID.String()
		}

		txs.TxID = sd.tx.Tx
		txs.TxSender = luna1crypto.BytesToAddress(sd.tx.Transaction.GetSender())
		txs.TxRecipient = luna1crypto.BytesToAddress(sd.tx.Transaction.GetRecipient())
		txs.TxAmount = big.NewInt(0).SetBytes(sd.tx.Transaction.GetAmount().Bytes()).String()
		txs.TxType = sd.tx_type.String()
		txs.TxTime = time.UnixMilli(int64(sd.tx.Transaction.GetTimestamp())).Local().String()
		txs.RcTime = sd.rctime.Local().String()
		txs.DbTime = time.Now().Local().String()
		txs.DiffTime = sd.diff.String()
		txs.MsgQueue = sd.sz
		return txs
	}
	return nil
}
