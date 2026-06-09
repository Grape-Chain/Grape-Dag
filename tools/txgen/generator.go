package txgen

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Grape-Chain/Grape-Dag/config"
	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"github.com/Grape-Chain/Grape-Dag/utils"
	"github.com/Grape-Chain/Grape-Dag/wallet"
	"github.com/Grape-Chain/Grape-Dag/crypto"
	"github.com/ledongthuc/goterators"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type GenConfiguration struct {
	Txrate      uint64 // number of transactions to generate per second
	Txmax       uint64 // total number of transactions to generate, 0 - infinite
	Duration    uint64 // tx generation duration in seconds; duration=600 will generate for 600sec at rate Txrate
	Txalgo      string // probability distribution algorithm: normal, uniform
	Txinitdelay uint64 // initial delay before the first transaction is generated
	Txtypes     string // comma separated types of transactions: e.g.: commission,smart,service
	Nodeip      string // ip address of the peer node to connect to and publish transactions
	Nodeport    uint16 // port number of the peer node to connect to and publish transactions
	Network     uint8
	Width       uint8  // When generating a true Dag, define the init width, to generate exodus wallets
	Corrupt     bool   // generate invalid transactions
	Trader      bool   // act as a coin trader
	Mode        string // support genesis; trader
	Wallet      string
	Ico         string // Initial coin offering
	Privatekey  string
	Publickey   string
	Wait        uint8 // wait before querying results - this may allow the balances to settle properly
}

type TxGenerator struct {
	Tx        config.TxConfiguration
	Generator GenConfiguration
	// Note: RoboTrader manages all wallets it transacts on
	Wallets  []*grape1crypto.Wallet
	Balances map[string]*big.Int
	Wallet   *grape1crypto.Wallet
}

func (gc TxGenerator) String() string {
	var buffer bytes.Buffer
	buffer.WriteString("GenConfiguration:\n")
	buffer.WriteString(fmt.Sprintf("\tMaxfuellimit: %d\n", gc.Tx.Maxfuellimit))
	buffer.WriteString(fmt.Sprintf("\tMaxfuelprice: %d\n", gc.Tx.Maxfuelprice))
	buffer.WriteString(fmt.Sprintf("\tNeutrino: %08f\n", gc.Tx.Neutrino))
	buffer.WriteString(fmt.Sprintf("\tTxrate: %d\n", gc.Generator.Txrate))
	buffer.WriteString(fmt.Sprintf("\tTxmax: %d\n", gc.Generator.Txmax))
	buffer.WriteString(fmt.Sprintf("\tDuration: %dsec\n", gc.Generator.Duration))
	buffer.WriteString(fmt.Sprintf("\tTxalgo: %s\n", gc.Generator.Txalgo))
	buffer.WriteString(fmt.Sprintf("\tTxinitdelay: %d\n", gc.Generator.Txinitdelay))
	buffer.WriteString(fmt.Sprintf("\tTxtypes: %s\n", gc.Generator.Txtypes))
	buffer.WriteString(fmt.Sprintf("\tNodeip: %s\n", gc.Generator.Nodeip))
	buffer.WriteString(fmt.Sprintf("\tNodeport: %d\n", gc.Generator.Nodeport))
	buffer.WriteString(fmt.Sprintf("\tNetwork: %d\n", gc.Generator.Network))
	buffer.WriteString(fmt.Sprintf("\tWidth: %d\n", gc.Generator.Width))
	buffer.WriteString(fmt.Sprintf("\tWait: %dsec\n", gc.Generator.Wait))
	buffer.WriteString(fmt.Sprintf("\tCorrupt: %t\n", gc.Generator.Corrupt))
	buffer.WriteString(fmt.Sprintf("\tTrader: %t\n", gc.Generator.Trader))
	buffer.WriteString(fmt.Sprintf("\tGenesis Wallet: %s\n", gc.Generator.Wallet))
	buffer.WriteString(fmt.Sprintf("\tICO: %s\n", gc.Generator.Ico))
	return buffer.String()
}

func (g *TxGenerator) CreategRpcClient() (pb.RoboTraderClient, *grpc.ClientConn) {
	grpcsrv_url := fmt.Sprintf("%s:%d", g.Generator.Nodeip, g.Generator.Nodeport)
	fmt.Printf("[TxGen] Dialing gRpc Service at %s\n", grpcsrv_url)

	opts := []grpc.DialOption{
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(10 * config.MB)),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithMaxMsgSize(10 * config.MB),
		grpc.WithInsecure(),
		grpc.WithBlock(),
	}

	conn, err := grpc.Dial(grpcsrv_url, opts...)
	if err != nil {
		fmt.Printf("[TxGen] Error dialing. %v+", err)
		return nil, nil
	}
	fmt.Println("[TxGen] Create gRPC Client")
	cltService := pb.NewRoboTraderClient(conn)
	return cltService, conn
}

func (g *TxGenerator) getTickerDuration() time.Duration {
	var ticker int64
	switch g.Generator.Txalgo {
	case "normal":
		ticker = TxnormalTime(g.Generator.Txrate)
	case "uniform":
		ticker = TxuniformTime(g.Generator.Txrate)
	default:
		ticker = TxdefaultTime(g.Generator.Txrate)
	}
	return time.Millisecond * time.Duration(ticker)
}

func (g *TxGenerator) runTx(cltService *pb.RoboTraderClient) error {
	ticker := g.getTickerDuration()
	r := rand.New(rand.NewSource(time.Now().UnixMilli()))
	t := time.NewTimer(time.Duration(int64(ticker)))
	<-t.C
	tx := tx.NewTxv1(tx.ChainType(g.Generator.Network))
	tx.GenerateRandom(g.Tx.Maxfuellimit, g.Tx.Maxfuelprice, wallet.GenRanWallet(), g.Generator.Network)
	if g.Generator.Corrupt {
		if rand.NormFloat64()*0.1+0.5 < 0.3 {
			// Corrupt the content so the hash on validation will be different
			tx.Amount = big.NewInt(r.Int63()).Bytes()
			tx.Nonce = utils.RandomUint64()
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	_, err := (*cltService).PublishTx(ctx, &pb.TxPublishRequest{
		Tx: tx.MarshalBinary(),
	})
	if err != nil {
		fmt.Printf("[TxGen] Failed to publish: %v+\n", err)
		defer os.Exit(1)
		return err
	}
	//fmt.Println("Successfully published a transaction")
	return nil
}

func (g *TxGenerator) Trade0(cltService *pb.RoboTraderClient) error {
	tx := tx.NewTxv1(tx.ChainType(g.Generator.Network))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	// need to init with the genesis wallet
	wsb := g.Wallets[0]
	var balance_response *pb.BalanceResponse
	var err error
	// get the current balance for the genesis wallet
	balance_response, err = (*cltService).GetBalances(ctx, &pb.BalanceRequest{
		Wallets: [][]byte{grape1crypto.AddressToBytes(wsb.WalletAddress())},
	})
	cancel()
	if err != nil {
		return fmt.Errorf("[$] 0 Get balance for %s. err: %s", wsb.WalletAddress(), err.Error())
	}
	// we are generating exodus tx - get a new wallet address
	var wrb *grape1crypto.Wallet = grape1crypto.NewWallet()
	g.Wallets = append(g.Wallets, wrb)
	lastKnownBalance := big.NewInt(0).SetBytes(balance_response.Balances[0])
	x := math.Log10(float64(lastKnownBalance.Uint64()))
	var delta *big.Int = big.NewInt(lastKnownBalance.Int64())
	delta.Div(delta, big.NewInt(int64(math.Pow10(int(x)-1))))
	// fmt.Printf("\n[Transact] 0 From %s $%d to %s move $%d\n", wsb.WalletAddress(), lastKnownBalance.Uint64(), wrb.WalletAddress(), delta.Uint64())
	tx.GeneratePayment(
		wallet.GenPaymentTransaction(wsb, wrb, delta),
		uint8(g.Generator.Network),
	)
	tx.Sign(wsb.PrivateKey())

	if err := tx.Verify(); err != nil {
		fmt.Printf("Failed to verify newly generated tx: %s\n", err.Error())
		return err
	}

	ctx, cancel = context.WithTimeout(context.Background(), time.Second*30)
	_, err = (*cltService).PublishTx(ctx, &pb.TxPublishRequest{
		Tx: tx.MarshalBinary(),
	})
	cancel()
	if err != nil {
		fmt.Printf("[TxGen] Failed to publish: %s+\n", err.Error())
		defer os.Exit(1)
		return err
	}
	//fmt.Println("Successfully published a transaction")
	return nil
}

func (g *TxGenerator) Trade1(cltService *pb.RoboTraderClient) error {
	//r := rand.New(rand.NewSource(time.Now().UnixMilli()))
	tx := tx.NewTxv1(tx.ChainType(g.Generator.Network))
	// Request known wallets from node
	// From each known wallet obtain their balances
	// Create new wallets and move funds to these new wallets,
	// and add these wallets to cache
	// Get random wallet from slice of wallets, and get its balance,
	// if balance is sufficient, move 1/10 of the funds to a new wallet
	var wsb *grape1crypto.Wallet
	var err error
	wsb = g.Wallets[0]
	var wrb *grape1crypto.Wallet = grape1crypto.NewWallet()
	// for {
	// 	wrb = g.Wallets[r.Intn(len(g.Wallets))]
	// 	if wsb.WalletAddress() != wrb.WalletAddress() {
	// 		break
	// 	}
	// }

	var delta *big.Int = big.NewInt(1)
	// fmt.Printf("\n[Transact] From %s $%d to %s move $%d\n", wsb.WalletAddress(), lastKnownBalance.Uint64(), wrb.WalletAddress(), delta.Uint64())
	tx.GeneratePayment(
		wallet.GenPaymentTransaction(wsb, wrb, delta),
		uint8(g.Generator.Network),
	)
	tx.Sign(wsb.PrivateKey())

	if err := tx.Verify(); err != nil {
		fmt.Printf("Failed to verify newly generated tx: %s", err.Error())
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	_, err = (*cltService).PublishTx(ctx, &pb.TxPublishRequest{
		Tx: tx.MarshalBinary(),
	})
	cancel()
	if err != nil {
		fmt.Printf("[TxGen] Failed to publish: %v+\n", err)
		defer os.Exit(1)
		return err
	}
	//fmt.Println("Successfully published a transaction")
	return nil
}
func (g *TxGenerator) Count(cltService *pb.RoboTraderClient) error {
	var err error = nil
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	fmt.Println("\nGet DAG size...")
	tx_count, err := (*cltService).GetTxCount(ctx, &pb.TxCountRequest{})
	if err != nil {
		fmt.Printf("Failed to obtain DAG count. %s\n", err.Error())
		fmt.Println(">>> FAILURE <<<")
	} else {
		fmt.Printf("Successfully received DAG count %d: Genesis + %d\n", tx_count.Count, tx_count.Count-1)
		fmt.Printf("Successfully received DAG tps %f tps\n", tx_count.Tps)
		t := time.Microsecond * time.Duration(tx_count.Avgdelay)
		fmt.Printf("Successfully received DAG adv delay %d msec\n", t.Milliseconds())
	}
	cancel()
	os.Exit(1)
	return err
}

func (g *TxGenerator) Balance(cltService *pb.RoboTraderClient) error {
	var err error = nil
	for {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		fmt.Println("\nGet all known to DAG [$]wallets[$]...")
		wallets, err := (*cltService).GetWallets(ctx, &pb.WalletsRequest{})
		if err != nil {
			fmt.Printf("Failed to obtain all known wallets. %s\n", err.Error())
			fmt.Println(">>> FAILURE <<<")
			break
		}
		fmt.Printf("Successfully received %d wallets\n", len(wallets.Wallets))
		fmt.Printf("Wallets %d\n", len(wallets.Wallets))
		ctx, cancel = context.WithTimeout(context.Background(), time.Second*30)
		fmt.Println("Query final balances for all the wallets...")
		balance_response, err := (*cltService).GetBalances(ctx, &pb.BalanceRequest{
			Wallets: wallets.Wallets,
		})
		cancel()
		totalBalance := big.NewInt(0)
		if err != nil {
			fmt.Printf("Failed to obtain balances. %s\n", err.Error())
		} else {
			goterators.ForEach(balance_response.Balances, func(b []byte) {
				totalBalance.Add(totalBalance, big.NewInt(0).SetBytes(b))
			})
		}
		fmt.Printf("Total balances for all known wallets: $%s\n", totalBalance.String())
		fmt.Printf("Initial coin offering: $%s, total balance for all wallets: $%s\n", g.Generator.Ico, totalBalance.String())
		genesisAmount, _ := big.NewInt(0).SetString(g.Generator.Ico, 10)
		if g.Generator.Ico == totalBalance.String() {
			fmt.Printf("Successfully confirmed all tx with the total balance %s\n", totalBalance.String())
			fmt.Println("<<< SUCCESS >>>")
		} else {
			diff := big.NewInt(0).Sub(genesisAmount, totalBalance)
			fmt.Printf("Failed to confirm all tx. Difference is: $%s\n", diff.String())
			fmt.Println(">>> FAILURE <<<")
		}
		break
	}
	os.Exit(1)
	return err
}

func (g *TxGenerator) TradeLocal(cltService *pb.RoboTraderClient, wg *sync.WaitGroup) error {
	defer wg.Done()
	req := pb.TxGenerateRequest{
		Duration:   uint32(g.Generator.Duration),
		Txrate:     uint32(g.Generator.Txrate),
		Txmax:      uint32(g.Generator.Txmax),
		Publickey:  g.Generator.Publickey,
		Privatekey: g.Generator.Privatekey,
		Network:    uint32(g.Generator.Network),
	}
	res, err := (*cltService).GenerateTx(context.Background(), &req)
	if err != nil {
		return err
	}
	t := time.NewTimer(time.Duration(5) * time.Second)
	<-t.C
	ctx, csl := context.WithTimeout(context.Background(), time.Duration(5)*time.Second)
	crsp, err := (*cltService).GetTxCount(ctx, &pb.TxCountRequest{})
	csl()

	defer os.Exit(1)
	if err != nil {
		fmt.Printf("Failed to get DAG count. err: %s", err.Error())
	}
	ft := time.Now().Local().String()
	fmt.Printf("\n At %s the stats are:\n", ft)
	if crsp != nil {
		fmt.Printf("* DAG count is %d (Genesis+%d) ---\n", crsp.Count, crsp.Count-1)
		fmt.Printf("* DAG tps   is %f {from the time Genesis created}\n", crsp.Tps)
	}

	const M1 = 1000000.
	fmt.Printf("* Generated %d txs in %06f sec - %f tps\n", res.Txcount, res.Duration/M1, M1*float32(res.Txcount)/res.Duration)
	return nil
}

func (g *TxGenerator) Trade(cltService *pb.RoboTraderClient) error {
	r := rand.New(rand.NewSource(time.Now().UnixMilli()))
	tx := tx.NewTxv1(tx.ChainType(g.Generator.Network))
	// Request known wallets from node
	// From each known wallet obtain their balances
	// Create new wallets and move funds to these new wallets,
	// and add these wallets to cache
	// Get random wallet from slice of wallets, and get its balance,
	// if balance is sufficient, move 1/10 of the funds to a new wallet
	var wsb *grape1crypto.Wallet
	var balance_response *pb.BalanceResponse
	var err error
	for {
		// make sure that the wallet we pick randomly is known to the peer
		wsb = g.Wallets[r.Intn(len(g.Wallets))]
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		balance_response, err = (*cltService).GetBalances(ctx, &pb.BalanceRequest{
			Wallets: [][]byte{grape1crypto.AddressToBytes(wsb.WalletAddress())},
		})
		cancel()
		// sender must have positive balance
		if err == nil && big.NewInt(0).SetBytes(balance_response.Balances[0]).Cmp(big.NewInt(0)) > 0 {
			break
		}
	}
	var wrb *grape1crypto.Wallet
	if len(g.Wallets) > int(g.Generator.Width) && rand.Int31n(10) <= 5 {
		for {
			wrb = g.Wallets[r.Intn(len(g.Wallets))]
			if wsb.WalletAddress() != wrb.WalletAddress() {
				break
			}
		}
	} else {
		// need to generate a new wallet
		wrb = grape1crypto.NewWallet()
	}

	lastKnownBalance := big.NewInt(0).SetBytes(balance_response.Balances[0])
	x := math.Log10(float64(lastKnownBalance.Uint64()))
	var delta *big.Int = big.NewInt(lastKnownBalance.Int64())
	delta.Div(delta, big.NewInt(int64(math.Pow10(int(x)-1))))
	// fmt.Printf("\n[Transact] From %s $%d to %s move $%d\n", wsb.WalletAddress(), lastKnownBalance.Uint64(), wrb.WalletAddress(), delta.Uint64())
	tx.GeneratePayment(
		wallet.GenPaymentTransaction(wsb, wrb, delta),
		uint8(g.Generator.Network),
	)
	tx.Sign(wsb.PrivateKey())

	if err := tx.Verify(); err != nil {
		fmt.Printf("Failed to verify newly generated tx: %s", err.Error())
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	_, err = (*cltService).PublishTx(ctx, &pb.TxPublishRequest{
		Tx: tx.MarshalBinary(),
	})
	cancel()
	if err != nil {
		fmt.Printf("[TxGen] Failed to publish: %v+\n", err)
		defer os.Exit(1)
		return err
	}
	//fmt.Println("Successfully published a transaction")
	return nil
}

func (g *TxGenerator) make_payment(cltService *pb.RoboTraderClient, walletto string, amount *big.Int) error {
	tx := tx.NewTxv1(tx.ChainType(g.Generator.Network))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	// need to init with the genesis wallet
	wsb := g.Wallets[0]
	var balance_response *pb.BalanceResponse
	var err error
	// get the current balance for the genesis wallet
	balance_response, err = (*cltService).GetBalances(ctx, &pb.BalanceRequest{
		Wallets: [][]byte{grape1crypto.AddressToBytes(wsb.WalletAddress())},
	})
	cancel()
	if err != nil {
		return fmt.Errorf("[$] 0 Get balance for %s. err: %s", wsb.WalletAddress(), err.Error())
	}
	// we are generating exodus tx - get a new wallet address
	lastKnownBalance := big.NewInt(0).SetBytes(balance_response.Balances[0])
	// fmt.Printf("\n[Transact] 0 From %s $%d to %s move $%d\n", wsb.WalletAddress(), lastKnownBalance.Uint64(), wrb.WalletAddress(), delta.Uint64())
	if lastKnownBalance.Cmp(amount) < 0 {
		return fmt.Errorf("Tx for wallet %s exceeds the remaining balance for wallet %s", walletto, wsb.WalletAddress())
	}
	fmt.Printf("[$] Last known balance for genesis %s is %s\n", wsb.WalletAddress(), lastKnownBalance.String())
	tx.GeneratePayment(
		wallet.GenPaymentEx(wsb, walletto, amount),
		uint8(g.Generator.Network),
	)
	tx.Sign(wsb.PrivateKey())

	if err := tx.Verify(); err != nil {
		fmt.Printf("Failed to verify newly generated tx: %s\n", err.Error())
		return err
	}

	ctx, cancel = context.WithTimeout(context.Background(), time.Second*30)
	_, err = (*cltService).PublishTx(ctx, &pb.TxPublishRequest{
		Tx: tx.MarshalBinary(),
	})
	cancel()
	if err != nil {
		fmt.Printf("[TxGen] Failed to publish: %s+\n", err.Error())
		defer os.Exit(1)
		return err
	}
	if g.Generator.Wait > 0 {
		time.Sleep(time.Duration(g.Generator.Wait) * time.Second)
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Second*30)
	balance_response, err = (*cltService).GetBalances(ctx, &pb.BalanceRequest{
		Wallets: [][]byte{grape1crypto.AddressToBytes(walletto)},
	})
	cancel()
	if err != nil {
		return fmt.Errorf("Error querying new balance. err: %s", err.Error())
	}

	newBalance := big.NewInt(0).SetBytes(balance_response.Balances[0])
	fmt.Printf("[$] New balance for %s is %s\n", walletto, newBalance.String())

	ctx, cancel = context.WithTimeout(context.Background(), time.Second*30)
	balance_response, err = (*cltService).GetBalances(ctx, &pb.BalanceRequest{
		Wallets: [][]byte{grape1crypto.AddressToBytes(wsb.WalletAddress())},
	})
	cancel()
	if err != nil {
		return fmt.Errorf("[$] 0 Get balance for %s. err: %s", wsb.WalletAddress(), err.Error())
	}

	lastKnownBalance = big.NewInt(0).SetBytes(balance_response.Balances[0])
	fmt.Printf("[$] New balance for genesis %s is %s\n", wsb.WalletAddress(), lastKnownBalance.String())

	return nil
}

func (g *TxGenerator) GenWallet() {
	defer os.Exit(1)
	w := grape1crypto.NewWallet()
	utils.SaveWalletKey(w.WalletAddress(), w.PrivateKeyStr(), w.PublicKeyStr())
	// t := time.NewTicker(time.Second * 2)
	// <-t.C
	fmt.Println("\nNew Wallet =>")
	fmt.Printf("\tWallet     : %s\n", w.WalletAddress())
	fmt.Printf("\tPrivate Key: %s\n", w.PrivateKeyStr())
	fmt.Printf("\tPublic Key : %s\n", w.PublicKeyStr())
}

func (g *TxGenerator) check_balance(cltService *pb.RoboTraderClient, wallet string) error {

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	// need to init with the genesis wallet
	var balance_response *pb.BalanceResponse
	var err error
	// get the current balance for the genesis wallet
	balance_response, err = (*cltService).GetBalances(ctx, &pb.BalanceRequest{
		Wallets: [][]byte{grape1crypto.AddressToBytes(wallet)},
	})
	cancel()
	if err != nil {
		return fmt.Errorf("[$] 0 Get balance for %s. err: %s", wallet, err.Error())
	}
	// we are generating exodus tx - get a new wallet address
	lastKnownBalance := big.NewInt(0).SetBytes(balance_response.Balances[0])
	// fmt.Printf("\n[Transact] 0 From %s $%d to %s move $%d\n", wsb.WalletAddress(), lastKnownBalance.Uint64(), wrb.WalletAddress(), delta.Uint64())
	fmt.Printf("[$] Last known balance for wallet %s is %s\n", wallet, lastKnownBalance.String())

	return nil
}

func (g *TxGenerator) Check(cltService *pb.RoboTraderClient, wallets []string) {
	defer os.Exit(1)
	goterators.ForEach(wallets, func(wallet string) {
		err := g.check_balance(cltService, wallet)
		if err != nil {
			fmt.Printf("[TxG] Balance check error: %s", err.Error())
		}
	})

}

func (g *TxGenerator) Payment(cltService *pb.RoboTraderClient, wallets []string, amount *big.Int) {
	defer os.Exit(1)
	goterators.ForEach(wallets, func(wallet string) {
		err := g.make_payment(cltService, wallet, amount)
		if err != nil {
			fmt.Printf("[TxG] Payment error: %s", err.Error())
		}
	})
	// t := time.NewTicker(time.Second * 2)
	// <-t.C
}

func (g *TxGenerator) Generate(cltService *pb.RoboTraderClient, stop <-chan bool, mode GenMode, ch chan<- bool) {
	if g.Generator.Txinitdelay > 0 {
		t := time.NewTimer(time.Duration(time.Second * time.Duration(g.Generator.Txinitdelay)))
		<-t.C
		t.Stop()
	}
	count := g.Generator.Txmax
	infinite := count <= 0 || (g.Generator.Duration > 0 && mode == GEN_MODE_TRADER)

	if mode == GEN_MODE_GENESIS {
		count = uint64(g.Generator.Width)
	}
	// var initCount uint8 = 0
	// initWidth := g.Generator.Width

	var txCounter uint64 = 0

	tps := 1000. / float64(g.Generator.Txrate)
	var t *time.Ticker
	if mode == GEN_MODE_GENESIS {
		t = time.NewTicker(time.Duration(200 * float64(time.Millisecond)))
	} else if mode == GEN_MODE_TRADER {
		t = time.NewTicker(time.Duration(tps * float64(time.Millisecond)))
	}

	var flag atomic.Bool = atomic.Bool{}

	var durTm *time.Timer = nil
	if mode == GEN_MODE_TRADER {
		durTm = time.NewTimer(time.Second * time.Duration(g.Generator.Duration))
		go func() {
			defer durTm.Stop()
			<-durTm.C
			if infinite {
				fmt.Printf("\n[!] Duration timer is up after %d sec\n", g.Generator.Duration)
				os.Exit(0)
				durTm = nil
			}
		}()
	}
	time1 := time.Now()
	skips := 0
	for !flag.Load() {
		select {
		case v := <-stop:
			flag.Store(v)
			fmt.Println("Stopping generate go routine")
			break
		case <-t.C:
			txCounter++
			if txCounter%1000 == 0 {
				nt := time.NewTimer(time.Second)
				<-nt.C
				nt.Stop()
				skips++
				t.Reset(time.Duration(tps * float64(time.Millisecond)))
				continue
			}
			if !g.Generator.Trader {
				if err := g.runTx(cltService); err != nil {
					fmt.Printf("[%d] Failed to publish tx: %s\n", count, err.Error())
					flag.Store(true)
					break
				}
			} else {
				if mode == GEN_MODE_GENESIS {
					if err := g.Trade0(cltService); err != nil {
						fmt.Printf("[%d] Failed to trade tx: %s\n", count, err.Error())
						flag.Store(true)
						break
					}
				} else if mode == GEN_MODE_TRADER {
					if err := g.Trade1(cltService); err != nil {
						fmt.Printf("[%d] Failed to trade tx: %s\n", count, err.Error())
						flag.Store(true)
						break
					}
				}
			}
			if !infinite {
				count--
				if count == 0 {
					os.Exit(0)
				}
			}
			break
		}
	}
	time2 := time.Now()
	runTime := time2.Sub(time1)
	runTime = runTime - time.Duration(skips)*time.Second
	if durTm != nil {
		durTm.Stop()
		fmt.Println("Stopping duration timer...")
	}
	for {
		// give dag time to settle
		var waitTime uint8 = func() uint8 {
			if mode != GEN_MODE_TRADER {
				return 5
			} else {
				return g.Generator.Wait
			}
		}()
		t := time.NewTimer(time.Second * time.Duration(waitTime))
		<-t.C
		t.Stop()
		totalBalance := big.NewInt(0)
		// walletSet := set.New()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		fmt.Println("Get all known to DAG wallets...")
		wallets, err := (*cltService).GetWallets(ctx, &pb.WalletsRequest{})
		if err != nil {
			fmt.Printf("Failed to obtain all known wallets. %s\n", err.Error())
			fmt.Println(">>> FAILURE <<<")
			break
		}
		fmt.Printf("Successfully received %d wallets\n", len(wallets.Wallets))
		//		for _, w := range wallets.Wallets {
		fmt.Printf("Wallets %d\n", len(wallets.Wallets))
		//		}
		// goterators.ForEach(g.Wallets, func(i *grape1crypto.Wallet) {
		// 	walletSet.Insert(i.WalletAddress())
		// })
		// balanceSet := [][]byte{}
		// walletSet.Do(func(i interface{}) {
		// 	walletAddress := i.(string)
		// 	balanceSet = append(balanceSet, []byte(walletAddress))
		// })
		ctx, cancel = context.WithTimeout(context.Background(), time.Second*30)
		fmt.Println("Query final balances for all the wallets...")
		balance_response, err := (*cltService).GetBalances(ctx, &pb.BalanceRequest{
			Wallets: wallets.Wallets,
		})
		cancel()
		if err != nil {
			fmt.Printf("Failed to obtain balances. %s\n", err.Error())
		} else {
			goterators.ForEach(balance_response.Balances, func(b []byte) {
				totalBalance.Add(totalBalance, big.NewInt(0).SetBytes(b))
			})
		}
		fmt.Printf("Total balances for all known wallets: $%s\n", totalBalance.String())
		fmt.Printf("Initial coin offering: $%s, total balance for all wallets: $%s\n", g.Generator.Ico, totalBalance.String())
		genesisAmount, _ := big.NewInt(0).SetString(g.Generator.Ico, 10)
		if g.Generator.Ico == totalBalance.String() {
			fmt.Printf("Successfully confirmed all tx with the total balance %s\n", totalBalance.String())
			fmt.Println("<<< SUCCESS >>>")
		} else {
			diff := big.NewInt(0).Sub(genesisAmount, totalBalance)
			fmt.Printf("Failed to confirm all tx. Difference is: $%s\n", diff.String())
			fmt.Println(">>> FAILURE <<<")
		}
		if mode == GEN_MODE_TRADER {
			fmt.Printf("--- Generated %d txs in %f sec - %f tps ---\n", txCounter, runTime.Seconds(), float64(txCounter)/runTime.Seconds())
		}
		break
	}
	ch <- true
}

func TxGeneratorFromConfig() *TxGenerator {

	// Set the path to look for the configurations file
	hd, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Error: %v+", err)
		return nil
	}

	configPath := filepath.Join(hd, config.GRAPEONE_CFG_PATH)

	fd, err := os.Open(filepath.Join(hd, config.GRAPEONE_CFG_PATH, config.GENERATOR_FILE))
	if err != nil {
		if os.IsNotExist(err) {
			// Configuration file does not exist
			fmt.Printf("%v+", err)
			return nil
		}
	}
	fd.Close()

	viper.SetConfigName(config.GENERATOR_NAME)
	viper.AddConfigPath(configPath)

	// Enable VIPER to read Environment Variables
	viper.AutomaticEnv()

	viper.SetConfigType(config.GENERATOR_EXT)

	var txConfig *TxGenerator = &TxGenerator{
		Balances: make(map[string]*big.Int),
	}

	if err := viper.ReadInConfig(); err != nil {
		fmt.Printf("Error reading %s file, %v", config.GENERATOR_FILE, err)
		return nil
	}

	// Set undefined variables
	viper.SetDefault("generator.maxfuellimit", 999)
	viper.SetDefault("generator.maxfuelprice", 10000000)
	viper.SetDefault("generator.neutrino", 0.00000001)
	viper.SetDefault("generator.txrate", 1)
	viper.SetDefault("generator.txmax", 0)
	viper.SetDefault("generator.duration", 0)
	viper.SetDefault("generator.txalgo", "normal")
	viper.SetDefault("generator.txinitdelay", 10)
	viper.SetDefault("generator.txtypes", "commission,smart,service")
	viper.SetDefault("generator.nodeip", "localhost")
	viper.SetDefault("generator.nodeport", 50333)
	viper.SetDefault("generator.corrupt", false)
	viper.SetDefault("generator.trader", false)
	viper.SetDefault("generator.width", 10)
	viper.SetDefault("generator.ico", 1000000000)
	viper.SetDefault("generator.wait", 5)

	err = viper.Unmarshal(&txConfig)
	if err != nil {
		fmt.Printf("Unable to decode into Grapepeer, %v", err)
		return nil
	}
	fmt.Printf("%s", txConfig)
	// Overwrite the num of approved tx - this is the optimal value already
	return txConfig
}
