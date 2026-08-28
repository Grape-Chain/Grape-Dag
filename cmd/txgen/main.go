package main

import (
	"context"
	"flag"
	"fmt"
	"math/big"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	txgen "github.com/Grape-Chain/Grape-Dag/tools/txgen"
	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"github.com/Grape-Chain/Grape-Dag/utils"
	"github.com/Grape-Chain/Grape-Dag/crypto"
	"github.com/briandowns/spinner"
	"github.com/enescakir/emoji"
)

const banner = `
   ______                         ______     ______         
  / ____/________ _____  ___     /_  __/  __/ ____/__  ____ 
 / / __/ ___/ __ '/ __ \/ _ \     / / | |/_/ / __/ _ \/ __ \
/ /_/ / /  / /_/ / /_/ /  __/    / / _>  </ /_/ /  __/ / / /
\____/_/   \__,_/ .___/\___/    /_/ /_/|_|\____/\___/_/ /_/ 
			   /_/                                          
`

var grpc_port int = 0

// benchOpts is filled by the bench-mode flags. They are declared by the txgen
// package so that a new bench setting does not mean editing this file.
var benchOpts *txgen.BenchOptions

func ParseCmdArgs() (*txgen.CommandArgs, error) {
	conf := &txgen.CommandArgs{}

	flag.StringVar(&conf.Mode, "mode", "trader", "TxGen mode: genesis, trader, local, balance, payment, count, check, wallet, watchdog, bench")
	benchOpts = txgen.RegisterBenchFlags(flag.CommandLine)
	flag.IntVar(&grpc_port, "grpc_port", 0, "GRPC Port to use when publishing tx")
	flag.StringVar(&conf.To, "to", "", "Wallet address(es) to send coins to: e.g. -to 0x12..,0x34...,0x56...")
	flag.StringVar(&conf.To, "from", "", "Wallet address(es) to send coins to: e.g. -to 0x12..,0x34...,0x56...")
	flag.StringVar(&conf.Amount, "amount", "0", "Amount to send from Genesis wallet")
	flag.StringVar(&conf.Wallets, "wallets", "", "Check wallent current balance")
	flag.Int64Var(&conf.Timeout, "timeout", 0, "In mode watchdog total timeout before peer is terminated")
	flag.Int64Var(&conf.Retries, "retries", 0, "In mode watchdot number of retries after which peer is terminated")

	flag.Parse()

	conf.Mode = strings.Trim(conf.Mode, "\n\t ")
	conf.Mode = strings.ToLower(conf.Mode)

	_, ok := txgen.ModeMapper(conf.Mode)
	if !ok {
		return nil, fmt.Errorf("Invalid value for mode. Expect: genesis, trader, local, balance, payment, count, check. You provided: %s", conf.Mode)
	}

	if (grpc_port > 0 && grpc_port < 49152) || grpc_port > 64353 {
		return nil, fmt.Errorf("Invalid value for port %d. Valid range 49152-64353", grpc_port)
	}

	return conf, nil
}

var ch chan bool = make(chan bool)

func main() {
	fmt.Print(banner)
	fmt.Printf("%s Grape TxGen %s  ~ Analyzing configuration %s...\n", emoji.Grapes, emoji.Grapes, emoji.MagnifyingGlassTiltedRight)

	cfg, err := ParseCmdArgs()
	if err != nil {
		fmt.Printf("Error: %s", err.Error())
		return
	}

	txGenerator := txgen.TxGeneratorFromConfig()
	if txGenerator == nil {
		return
	}

	if grpc_port != 0 {
		txGenerator.Generator.Nodeport = uint16(grpc_port)
	}

	wallets := []string{}
	amount := big.NewInt(0)
	ok := true
	m := txgen.Must(txgen.ModeMapper(cfg.Mode))
	if m == txgen.GEN_MODE_PAYMENT {
		if len(cfg.To) == 0 {
			fmt.Println("Please provide a list of wallets")
			return
		}
		if amount, ok = big.NewInt(0).SetString(cfg.Amount, 10); !ok {
			fmt.Printf("Invalid amount value %s\n", cfg.Amount)
			return
		}
		wallets = strings.Split(cfg.To, ",")
	} else if m == txgen.GEN_MODE_CHECK {
		if len(cfg.Wallets) == 0 {
			fmt.Println("Please provide a list of wallets")
			return
		}
		wallets = strings.Split(cfg.Wallets, ",")
	} else if m == txgen.GEN_MODE_WATCHDOG {
		if cfg.Timeout <= 0 {
			fmt.Printf("%s  Mode WATCHDOG required a valid timeout value in seconds\n", emoji.NoEntry)
			return
		}
		if cfg.Retries <= 0 {
			fmt.Printf("%s  Mode WATCHDOG required a valid number of retries\n", emoji.NoEntry)
			return
		}
	}

	g := txGenerator.Generator

	cltService, conn := txGenerator.CreategRpcClient()
	if conn == nil || cltService == nil {
		panic("Failed to create gRpc client")
	}
	defer conn.Close()

	if txgen.GenesisWalletMode(m) {
		// Need to load the genesis wallet object to bootstrap the process
		genesisWallet := grape1crypto.LoadWallet(g.Publickey, g.Privatekey)
		if genesisWallet == nil {
			fmt.Printf("[ERROR]Failed to load Genesis Wallet. pvt:%s|pub:%s\n", g.Privatekey, g.Publickey)
			return
		}
		txGenerator.Wallets = append(txGenerator.Wallets, genesisWallet)

		ctx, cancel := context.WithCancel(context.Background())
		genesisBalance, err := cltService.GetBalances(ctx, &pb.BalanceRequest{
			Wallets: [][]byte{grape1crypto.AddressToBytes(genesisWallet.WalletAddress())},
		})
		if err != nil {
			fmt.Printf("[$] 0 Get balance for %s. err: %s", genesisWallet.WalletAddress(), err.Error())
			return
		}
		cancel()
		txGenerator.Balances[genesisWallet.WalletAddress()] = big.NewInt(0).SetBytes(genesisBalance.Balances[0])

	}
	if m == txgen.GEN_MODE_BENCH {
		// Bench mode runs in the foreground and handles its own signals, so that
		// <Ctrl-C> still prints the report. The tail the other modes run after
		// stopping - settle, fetch every wallet, reconcile balances - would add
		// minutes to a measurement that is already finished.
		if err := txGenerator.Bench(&cltService, benchOpts); err != nil {
			fmt.Printf("%s  ~ %s\n", emoji.Warning, err.Error())
			os.Exit(1)
		}
		os.Exit(0)
	}

	var (
		stop chan bool
		s    *spinner.Spinner = spinner.New(spinner.CharSets[36], 100*time.Millisecond)
		wg   sync.WaitGroup
	)
	// for now only wallet and count commands are properly implemented using the Command Design Pattern
	var status txgen.CommandStatus
	if txgen.IsProcessCommand(m) {
		go func() {
			c := txgen.GeDefaultCmdFactory().Create(m)
			if err := c.Init(cfg); err != nil {
				panic(err.Error())
			}
			s.Color(c.AdviseColor())
			status, err = c.ProcessResult(c.Execute(&cltService))
			if err != nil {
				fmt.Printf("%s  ~ Error: %s\n", emoji.Warning, err.Error())
			}
			os.Exit(1)
		}()
	} else {
		// @TODO: Migrate all commands to the command design pattern implementation
		//channel and pass to go routine
		stop := make(chan bool)
		defer close(stop)
		fmt.Printf("[TxGen] Run tx generator in mode [%s]\n", strings.ToUpper(cfg.Mode))
		wg := sync.WaitGroup{}
		switch m {
		case txgen.GEN_MODE_GENESIS:
			go txGenerator.Generate(&cltService, stop, m, ch)
			s.Color("cyan")
		case txgen.GEN_MODE_TRADER:
			go txGenerator.Generate(&cltService, stop, m, ch)
			s.Color("red")
		case txgen.GEN_MODE_LOCAL:
			wg.Add(1)
			go txGenerator.TradeLocal(&cltService, &wg)
			s.Color("yellow")
		case txgen.GEN_MODE_BALANCE:
			go txGenerator.Balance(&cltService)
			s.Color("green")
		case txgen.GEN_MODE_PAYMENT:
			go txGenerator.Payment(&cltService, wallets, amount)
			s.Color("magenta")
		case txgen.GEN_MODE_WALLET:
			go txGenerator.GenWallet()
			s.Color("blue")
		case txgen.GEN_MODE_CHECK:
			go txGenerator.Check(&cltService, wallets)
		case txgen.GEN_MODE_COUNT:
			go txGenerator.Count(&cltService)
			s.Color("red")
		}
	}
	//
	fmt.Printf("%s TxGen is running. <Ctrl-C> to stop", emoji.Grapes)
	if txgen.SpinMode(m) {
		if s != nil {
			s.Start()
		}
	}
	utils.WaitOnSignal([]os.Signal{syscall.SIGINT, syscall.SIGTERM})
	if txgen.SpinMode(m) {
		if s != nil {
			s.Stop()
		}
	}
	fmt.Printf("%s Stopping TxGen...\n", emoji.Grapes)
	if m == txgen.GEN_MODE_TRADER {
		// call channel to stop the routine
		stop <- true
		<-ch
		t := time.NewTimer(time.Second * 3)
		<-t.C
	} else if m == txgen.GEN_MODE_LOCAL {
		fmt.Println("*")
		wg.Wait()
	}
	fmt.Printf("%s TxGen stopped [%s]\n", emoji.Grapes, status)
	os.Exit(int(status))
}
