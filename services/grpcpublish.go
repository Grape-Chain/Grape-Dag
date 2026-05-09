package services

import (
	"context"
	"fmt"
	"math/big"
	"net"
	"os"
	"sync/atomic"
	"syscall"
	"time" // fmt.Printf("\n[Transact] From %s $%d to %s move $%d\n", wsb.WalletAddress(), lastKnownBalance.Uint64(), wrb.WalletAddress(), delta.Uint64())

	"github.com/VG-Grape/luna/app"
	"github.com/VG-Grape/luna/config"
	"github.com/VG-Grape/luna/dag"
	txqueue "github.com/VG-Grape/luna/queues"
	"github.com/VG-Grape/luna/tx"
	"github.com/VG-Grape/luna/tx/pb"
	"github.com/VG-Grape/luna/utils"
	"github.com/VG-Grape/luna/wallet"
	"github.com/VG-Grape/luna/crypto"
	"github.com/enescakir/emoji"
	golog "github.com/ipfs/go-log/v2"
	"google.golang.org/grpc"
)

const PREFIX = "grpc-trader"

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
	wallet   *luna1crypto.Wallet
	network  uint8
}

func (g *TxGeneration) Trade() (int64, error) {
	tx := tx.NewTxv1(tx.ChainType(g.network))

	var wrb *luna1crypto.Wallet = luna1crypto.NewWallet()

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

	genesisWallet := luna1crypto.LoadWallet(req.Publickey, req.Privatekey)
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

func (ps *RoboTraderServer) PublishTx(ctx context.Context, req *pb.TxPublishRequest) (*pb.TxPublishResponse, error) {
	pbtx := req.GetTx()

	tx := tx.NewTxv1(chaintype)

	tx.UnmarshalBinary(pbtx)
	logger.Debugf("[%s] Request to publish tx:\n%s", PREFIX, tx.String())

	txqueue.GetPublishQueue().Enqueue(tx)

	res := &pb.TxPublishResponse{Status: 0, Msg: "success"}
	return res, nil
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

func (rts *RoboTraderServer) WatchDog(ctx context.Context, req *pb.WatchDogRequest) (*pb.WatchDogResponse, error) {

	if app.GetApp().App_dagsyncmgr.HaveJoined.Load() {
		return &pb.WatchDogResponse{Status: pb.WatchDogResponse_RUNNING}, nil
	}

	ticker := req.Timeout / int64(req.Retries)
	logger.Infof("%s [WatchDog] ~ Ticker %f sec; Retries %d", emoji.ServiceDog, time.Duration(ticker).Seconds(), req.Retries)
	t := time.NewTicker(time.Duration(ticker))
	defer t.Stop()
	count := 0
	for {
		<-t.C
		have_joined := app.GetApp().App_dagsyncmgr.HaveJoined.Load()
		if have_joined {
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
