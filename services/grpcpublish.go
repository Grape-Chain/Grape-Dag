package services

import (
	"context"
	"fmt"
	"math/big"
	"net"
	"os"
	"runtime/debug"
	"sync/atomic"
	"syscall"
	"time" // fmt.Printf("\n[Transact] From %s $%d to %s move $%d\n", wsb.WalletAddress(), lastKnownBalance.Uint64(), wrb.WalletAddress(), delta.Uint64())

	"github.com/Grape-Chain/Grape-Dag/app"
	"github.com/Grape-Chain/Grape-Dag/config"
	"github.com/Grape-Chain/Grape-Dag/crypto"
	"github.com/Grape-Chain/Grape-Dag/dag"
	txqueue "github.com/Grape-Chain/Grape-Dag/queues"
	"github.com/Grape-Chain/Grape-Dag/stats"
	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"github.com/Grape-Chain/Grape-Dag/utils"
	"github.com/Grape-Chain/Grape-Dag/wallet"
	"github.com/enescakir/emoji"
	golog "github.com/ipfs/go-log/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const PREFIX = "grpc-trader"

// recoverUnary - turn a panic in any handler into an error for that one call.
//
// grpc-go does not recover panics, so before this a single malformed request to
// any method took the whole node down with it. The handlers on this service read
// request fields directly in several places - PublishTx dereferenced a nil
// transaction, WatchDog divided by a retry count of zero - and every one of them
// is reachable by anyone who can open a connection. Fixing each is right and
// done, but a public endpoint should not depend on having found them all.
//
// The panic is logged with its stack, because a recovered panic that leaves no
// trace is a bug that never gets fixed, and returned as Internal so the caller
// sees a failed call rather than a silent success.
func recoverUnary(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[%s] Panic in %s: %v\n%s", PREFIX, info.FullMethod, r, debug.Stack())
			resp, err = nil, status.Errorf(codes.Internal, "%s failed", info.FullMethod)
		}
	}()
	return handler(ctx, req)
}

type RoboTraderServer struct {
	pb.UnimplementedRoboTraderServer
}

var logger golog.EventLogger
var chaintype = tx.PRIVATE_TESTNET

func init() {
	logger = golog.Logger("grpc")

}

type TxGeneration struct {
	duration float32
	txrate   uint32
	txmax    uint32
	txcount  uint32
	wallet   *grape1crypto.Wallet
	network  uint8
}

func (g *TxGeneration) Trade() (int64, error) {
	tx := tx.NewTxv1(tx.ChainType(g.network))

	var wrb *grape1crypto.Wallet = grape1crypto.NewWallet()

	var delta *big.Int = big.NewInt(1)

	tx.GeneratePayment(
		wallet.GenPaymentTransaction(g.wallet, wrb, delta),
		uint8(g.network),
	)
	tx.Sign(g.wallet.PrivateKey())

	txqueue.GetPublishQueue().Enqueue(tx)

	return txqueue.GetPublishQueue().Len(), nil
}

func (g *TxGeneration) generate() (float32, int32) {
	count := 0

	var flag atomic.Bool = atomic.Bool{}

	var durTm *time.Timer = nil
	var durTime time.Duration
	if g.duration > 0 {
		durTime = time.Second * time.Duration(g.duration)
		durTm = time.NewTimer(durTime)
		go func() {
			defer durTm.Stop()
			<-durTm.C
			utils.ColorizeInfo(logger, "[TxG] Duration timer is up after %f sec", g.duration)
			flag.Store(true)
			durTm = nil
		}()
	}
	// skips := 0
	// delay := 0
	//reset_required := false
	const M1 = 1000000.
	tps := M1 / float64(g.txrate)
	tkr := time.Duration(tps * float64(time.Microsecond)).Microseconds()
	t := time.NewTicker(time.Duration(tkr) * time.Microsecond)
	time1 := time.Now()
	for !flag.Load() {
		// select {
		// case <-t.C:
		// queue relief step
		<-t.C
		// if g.txcount > 0 && count%config.QUEUE_RELIEF_SIZE == 0 {
		// 	t.Stop()
		// 	skips++
		// 	logger.Warn("[!] Inbound tx queue processing relief...")
		// 	if skips > 5 {
		// 		delay = 5
		// 	} else {
		// 		delay = skips
		// 	}
		// 	rt := time.NewTimer(time.Microsecond * time.Duration(delay*100))
		// 	<-rt.C
		// 	reset_required = true
		// }
		count++
		t1 := time.Now()
		sz, _ := g.Trade()
		txtime := time.Since(t1).Microseconds()
		if g.duration == 0 {
			if count == int(g.txmax) {
				t.Stop()
				flag.Store(true)
			}
		}
		if sz > config.QUEUE_GEN_SIZE {
			// need to slow down
			t.Stop()
			tkr += (sz - config.QUEUE_GEN_SIZE) * 100
			t = time.NewTicker(time.Microsecond * time.Duration(tkr))
			logger.Errorf("[-] * Tx insert is slower than the rate. [Q:%05d] Adjusting rate: %d", sz, tkr)
		} else if txtime > tkr {
			t.Stop()
			tkr += (txtime - tkr) + 500
			t = time.NewTicker(time.Microsecond * time.Duration(tkr))
			logger.Errorf("[-] Tx insert is slower than the rate. [Q:%05d] Adjusting rate: %d", sz, tkr)
		} else if txtime < tkr {
			if tkr > time.Duration(tps*float64(time.Microsecond)).Microseconds() {
				t.Stop()
				tkr = time.Duration(tps * float64(time.Microsecond)).Microseconds()
				t = time.NewTicker(time.Microsecond * time.Duration(tkr))
				logger.Errorf("[+] Tx insert is faster than the rate. [Q:%05d] Adjusting rate: %d", sz, tkr)
			}
		}
		// if reset_required {
		// 	reset_required = false
		// 	t = time.NewTicker(time.Duration(tps * float64(time.Millisecond)))
		// }
		//		}
	}
	runTime := time.Since(time1)
	// runTime = runTime - config.QUEUE_RELIEF_DELAY*time.Duration(skips)
	if durTm != nil {
		durTm.Stop()
		utils.ColorizeInfo(logger, "[TxG] Stopping duration timer after %f sec...", durTime.Seconds())
	}

	var reportTime float32
	if g.duration > 0 {
		reportTime = float32(durTime.Microseconds())
	} else {
		reportTime = float32(runTime.Microseconds())
	}

	return reportTime, int32(count)
}

func (ps *RoboTraderServer) GenerateTx(ctx context.Context, req *pb.TxGenerateRequest) (*pb.TxGenerateResponse, error) {

	genesisWallet := grape1crypto.LoadWallet(req.Publickey, req.Privatekey)
	if genesisWallet == nil {
		logger.Errorf("[ERROR]Failed to load Genesis Wallet. pvt:%s|pub:%s\n", req.Privatekey, req.Publickey)
	}

	txg := TxGeneration{
		duration: float32(req.Duration),
		txrate:   req.Txrate,
		txmax:    req.Txmax,
		txcount:  0,
		network:  uint8(req.Network),
		wallet:   genesisWallet,
	}

	dur, txc := txg.generate()

	res := &pb.TxGenerateResponse{
		Duration: dur,
		Txcount:  txc,
	}
	return res, nil
}

// Statuses PublishTx answers with. Zero has always meant success and clients
// test for it, so the refusals start at one.
// addressBytes - the length of an address everywhere in the system.
const addressBytes = grape1crypto.AddressLength

// tracePublish - render every published transaction into the log.
//
// Off unless -verbose is given, and read before the argument is built rather
// than left to the logger: Debugf's arguments are evaluated whatever the level
// is, and String() marshals the whole transaction to JSON. At the rates this
// path runs at that is thousands of JSON documents a second, built and thrown
// away. The same mistake on the insert path was the largest single consumer of
// CPU in a profile of a loaded node.
var tracePublish bool

const (
	publishAccepted   = 0
	publishNoTx       = 1 // the request carried no transaction to publish
	publishUnusableTx = 2 // the transaction could not be read
)

// PublishTx - accept a transaction over gRPC and queue it for diffusion.
//
// Two things this used to get wrong, both found while trying to establish what
// the node's throughput actually is.
//
// It read the request's fields through the message rather than through the
// generated getters, by way of Txv1.UnmarshalBinary, and direct field access on
// a nil message is a nil dereference. The server is created with no recovery
// interceptor, so grpc-go let that panic unwind into the process: an empty
// publish request - three bytes on the wire, no credentials, no valid
// transaction - stopped the node. There is a recovery interceptor now as well,
// but a handler should not need one, so the nil case is answered here.
//
// And nothing on this path was counted. TxIngress, TxAccepted and TxRejected
// were wired only into the REST entry point, so a five-minute saturation run
// offering 739,785 transactions over gRPC left grape_tx_accepted_total reading
// zero and the ingress histogram empty. A gauge that stays at zero under load
// is worse than a missing one, because it reads as an answer.
//
// Acceptance here means queued for diffusion, and that is the whole claim: the
// signature is checked by the subscriber and the balance by the DAG, both after
// this returns. The queue blocks rather than drops once it is full, so a client
// that keeps getting acceptances is being held at the node's real drain rate
// rather than being lied to.
func (ps *RoboTraderServer) PublishTx(ctx context.Context, req *pb.TxPublishRequest) (*pb.TxPublishResponse, error) {
	start := time.Now()
	defer stats.Since(stats.TxIngress, start)

	pbtx := req.GetTx()
	if pbtx == nil {
		stats.TxRejected.WithLabelValues("no transaction").Inc()
		return &pb.TxPublishResponse{Status: publishNoTx, Msg: "no transaction in the request"}, nil
	}

	newTx, err := readPublishedTx(pbtx)
	if err != nil {
		stats.TxRejected.WithLabelValues("unusable transaction").Inc()
		logger.Warnf("[%s] Refusing an unusable transaction: %s", PREFIX, err.Error())
		return &pb.TxPublishResponse{Status: publishUnusableTx, Msg: err.Error()}, nil
	}
	// Guarded rather than passed straight to Debugf: the argument would be built
	// whatever the log level is, and String() renders the whole transaction as
	// JSON. At the rates this path runs at that is thousands of throwaway JSON
	// documents a second.
	if tracePublish {
		logger.Debugf("[%s] Request to publish tx:\n%s", PREFIX, newTx.String())
	}

	txqueue.GetPublishQueue().Enqueue(newTx)

	stats.TxAccepted.Inc()
	return &pb.TxPublishResponse{Status: publishAccepted, Msg: "success"}, nil
}

// readPublishedTx turns the wire message into a transaction, converting a panic
// in the unmarshaller into an error.
//
// The recover is here rather than trusted away because UnmarshalBinary reads
// fields off the message directly, and it takes only one more nil-valued nested
// message for a request to reach a dereference the way the empty request did.
// A malformed transaction is a client's problem; it must not be the node's.
func readPublishedTx(pbtx *pb.Txv1) (t *tx.Txv1, err error) {
	defer func() {
		if r := recover(); r != nil {
			t, err = nil, fmt.Errorf("transaction could not be read: %v", r)
		}
	}()
	newTx := tx.NewTxv1(chaintype)
	newTx.UnmarshalBinary(pbtx)
	// The one shape check made here rather than left to the verifier. An address
	// is twenty bytes everywhere else in the system, and several things that
	// handle a transaction downstream - including rendering it for a log line -
	// assume it. Refusing it at the boundary is cheaper than making every one of
	// them defensive, and a transaction with no sender was never going to verify.
	if len(pbtx.GetSender()) != addressBytes {
		return nil, fmt.Errorf("sender address is %d bytes, want %d", len(pbtx.GetSender()), addressBytes)
	}
	return newTx, nil
}

func (rts *RoboTraderServer) GetTxCount(ctx context.Context, req *pb.TxCountRequest) (*pb.TxCountResponse, error) {

	dag.GetDag()

	resp := &pb.TxCountResponse{
		Count:    int64(dag.GetDag().Size()),
		Tps:      float32(dag.GetDag().Tps()),
		Avgdelay: dag.GetDag().AvgDelay(),
	}
	return resp, nil
}

func (rts *RoboTraderServer) GetBalances(ctx context.Context, req *pb.BalanceRequest) (*pb.BalanceResponse, error) {
	pbw := req.GetWallets()
	balances, err := dag.GetPin().GetBalances(pbw)
	resp := &pb.BalanceResponse{
		Balances: balances,
	}
	return resp, err
}

// GetWallets - return exodus wallets to the generator, so that txgen knows the initial set of wallets
// to work with
func (rts *RoboTraderServer) GetWallets(ctx context.Context, req *pb.WalletsRequest) (*pb.WalletsResponse, error) {
	wallets, err := dag.GetPin().GetWallets()
	resp := &pb.WalletsResponse{
		Wallets: wallets[:],
	}
	return resp, err
}

// WatchDog - answer whether this node has joined the network yet, waiting up to
// the caller's timeout for it to happen.
//
// Two nil-and-zero cases handled before anything else, both reachable by anyone
// who can open a connection. The retry count divides the timeout, and an integer
// division by zero panics; and the sync manager is looked up out of the app,
// which the gRPC service can start before, so a WatchDog call arriving early
// dereferenced a nil pointer. Neither had a recovery interceptor in front of it
// until now, so either one stopped the node.
//
// The argument check comes first because it is about the request rather than
// about this node's state: a caller that sent nonsense should be told so whether
// or not the node has joined.
func (rts *RoboTraderServer) WatchDog(ctx context.Context, req *pb.WatchDogRequest) (*pb.WatchDogResponse, error) {
	if req.GetRetries() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "retries must be greater than zero")
	}
	if req.GetTimeout() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "timeout must be greater than zero")
	}

	joined := func() bool {
		a := app.GetApp()
		return a != nil && a.App_dagsyncmgr != nil && a.App_dagsyncmgr.HaveJoined.Load()
	}
	if joined() {
		return &pb.WatchDogResponse{Status: pb.WatchDogResponse_RUNNING}, nil
	}

	ticker := req.GetTimeout() / int64(req.GetRetries())
	logger.Infof("%s [WatchDog] ~ Ticker %f sec; Retries %d", emoji.ServiceDog, time.Duration(ticker).Seconds(), req.Retries)
	t := time.NewTicker(time.Duration(ticker))
	defer t.Stop()
	count := 0
	for {
		// The caller's context is watched alongside the tick so that a client that
		// hangs up does not leave this loop running to its retry limit - which
		// ends in os.Exit, so an abandoned watchdog is not a harmless goroutine.
		select {
		case <-t.C:
		case <-ctx.Done():
			return nil, status.FromContextError(ctx.Err()).Err()
		}
		if joined() {
			return &pb.WatchDogResponse{Status: pb.WatchDogResponse_RUNNING}, nil
		}
		if count == int(req.Retries) {
			// OUT OF RETRIES
			// this routine will be abandoned until the process gets killed
			go func() {
				logger.Warnf("%s  %s  ~ Peer with process %d will be terminated", emoji.Warning, emoji.CrossMark, syscall.Getegid())
				tt := time.NewTimer(time.Second * 5)
				<-tt.C
				os.Exit(-1)
			}()
			return &pb.WatchDogResponse{Status: pb.WatchDogResponse_KILLED}, nil
		}
		count++
	}
}

func RunRoboTraderService(port int) chan<- bool {
	if c := config.GetConfig(); c != nil {
		tracePublish = c.Host.Verbose > 0
	}
	lis, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		logger.Errorf("[%s] Failed to start. err: %s", PREFIX, err.Error())
		return nil
	}
	kb := 1024
	mb := 1024 * kb

	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(10 * mb),
		grpc.MaxSendMsgSize(10 * mb),
		grpc.ChainUnaryInterceptor(recoverUnary),
	}
	srv := grpc.NewServer(opts...)

	pb.RegisterRoboTraderServer(srv, &RoboTraderServer{})

	utils.ColorizeInfo(logger, "[%s] running on %s", PREFIX, lis.Addr().String())

	go func() {
		if err := srv.Serve(lis); err != nil {
			logger.Fatalf("[%s] Failed to listen for gRPC: %s", PREFIX, err.Error())
		}
		logger.Infof("[%s] stopped", PREFIX)
	}()

	stop := make(chan bool)

	go func() {
		<-stop
		//gRPC stop requested; Publisher may be waiting for on a tx channel for new transactions
		//wake him up so that he can terminate
		tx := tx.NewServiceTxv1(chaintype, tx.TX_SERVICE_STOP, dag.GetDag().Wallet())
		txqueue.GetPublishQueue().Enqueue(tx)
		t := time.NewTimer(time.Millisecond * 500) // give it time to deliver the message
		<-t.C
		t.Stop()
		logger.Infof("[%s] Stopping service...", PREFIX)
		srv.Stop()
		logger.Infof("[%s] Closing listener...", PREFIX)
		lis.Close()
	}()

	return stop
}
