package txgen

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/VG-Grape/luna/tx/pb"
	"github.com/VG-Grape/luna/utils"
	"github.com/VG-Grape/luna/wallet"
	"github.com/VG-Grape/luna/crypto"
)

type CommandArgs struct {
	Mode    string
	To      string
	From    string
	Amount  string
	Wallets string
	Timeout int64
	Retries int64
}

type CommandStatus int

const (
	CS_OK CommandStatus = iota
	CS_ER               = 1
)

func (c CommandStatus) String() string {
	return []string{
		"CS_OK",
		"CS_ER",
	}[c]
}

/* ----------------------------------------------------------- */
type Command interface {
	Execute(cltService *pb.RoboTraderClient) any
	Init(args *CommandArgs) error
	ProcessResult(i any) (CommandStatus, error)
	AdviseColor() string
}

/* ----------------------------------------------------------- */
/* WALLET COMMAND OPERATIONS                                   */
type WalletCommand struct {
}
type WalletResponse struct {
	Wallet *luna1crypto.Wallet
}

func (w *WalletCommand) AdviseColor() string {
	return "green"
}

func (w *WalletCommand) ProcessResult(i any) (CommandStatus, error) {
	r, ok := i.(*WalletResponse)
	if !ok {
		return CS_ER, fmt.Errorf("failed to cast to WalletResponse. %t", ok)
	}
	err := utils.SaveWalletKey(r.Wallet.WalletAddress(), r.Wallet.PrivateKeyStr(), r.Wallet.PublicKeyStr())
	if err != nil {
		return CS_ER, fmt.Errorf("save wallet keys. err: %s", err.Error())
	}
	fmt.Println("\nNew Wallet =>")
	fmt.Printf("\tWallet     : %s\n", r.Wallet.WalletAddress())
	fmt.Printf("\tPrivate Key: %s\n", r.Wallet.PrivateKeyStr())
	fmt.Printf("\tPublic Key : %s\n", r.Wallet.PublicKeyStr())
	return CS_OK, nil
}
func (c *WalletCommand) Execute(_ *pb.RoboTraderClient) any {
	return &WalletResponse{
		Wallet: luna1crypto.NewWallet(),
	}
}
func (c *WalletCommand) Init(_ *CommandArgs) error {
	// nothing to init
	return nil
}

/* ----------------------------------------------------------- */
/* PAYMENT COMMAND OPERATIONS                                  */
type PaymentCommand struct {
	From   string
	To     string
	Amount *big.Int
}

func (c *PaymentCommand) AdviseColor() string {
	return "blue"
}
func (c *PaymentCommand) ProcessResult(i any) (CommandStatus, error) {
	return CS_OK, nil
}
func (c *PaymentCommand) Execute(cltService *pb.RoboTraderClient) any {
	return nil
}
func (c *PaymentCommand) Init(args *CommandArgs) error {
	buf := bytes.Buffer{}
	// To wallet address is required
	if len(args.To) == 0 {
		buf.WriteString("the wallet address to send the funds to is missing\n")
	} else {
		if !wallet.ValidateAddress(args.To) {
			buf.WriteString(fmt.Sprintf("the TO wallet address %s is invalid\n", args.To))
		} else {
			c.To = args.To
		}
	}
	if len(args.Amount) == 0 {
		buf.WriteString("the amount to send is missing\n")
	} else {
		var ok bool
		if c.Amount, ok = big.NewInt(0).SetString(args.Amount, 10); !ok {
			buf.WriteString(fmt.Sprintf("the amount value is incorrect: %s", args.Amount))
		}
	}
	if len(args.From) > 0 {
		if !wallet.ValidateAddress(args.From) {
			buf.WriteString(fmt.Sprintf("the FROM wallet address %s is invalid\n", args.To))
		} else {
			c.From = args.From
		}
	}
	if len(buf.String()) > 0 {
		return errors.New(buf.String())
	}
	return nil
}

/* ----------------------------------------------------------- */
/* COUNT COMMAND OPERATIONS                                    */
type CountCommand struct {
}
type CountResponse struct {
	Count    int64
	Tps      float32
	AvgDelay int64
}

func (w *CountCommand) AdviseColor() string {
	return "white"
}
func (c *CountCommand) Execute(cltService *pb.RoboTraderClient) any {
	var err error = nil
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	fmt.Println("\nGet DAG size...")
	tx_count, err := (*cltService).GetTxCount(ctx, &pb.TxCountRequest{})
	if err != nil {
		fmt.Printf("Failed to obtain DAG count. %s\n", err.Error())
		fmt.Println(">>> FAILURE <<<")
		cancel()
		return &CountResponse{Count: -1, Tps: -1, AvgDelay: -1}
	}
	cancel()
	return &CountResponse{Count: tx_count.Count, Tps: tx_count.Tps, AvgDelay: tx_count.Avgdelay}
}
func (c *CountCommand) ProcessResult(i any) (CommandStatus, error) {
	tx_count, ok := i.(*CountResponse)
	if !ok {
		return CS_ER, fmt.Errorf("failed to cast result to CountResponse. %t", ok)
	}
	fmt.Printf("Successfully received DAG count %d: Genesis + %d\n", tx_count.Count, tx_count.Count-1)
	fmt.Printf("Successfully received DAG tps %f tps\n", tx_count.Tps)
	t := time.Microsecond * time.Duration(tx_count.AvgDelay)
	fmt.Printf("Successfully received DAG adv delay %d msec\n", t.Milliseconds())
	return CS_OK, nil
}
func (c *CountCommand) Init(_ *CommandArgs) error {
	// nothing to init
	return nil
}

type GenMode uint8

const (
	GEN_MODE_GENESIS GenMode = iota
	GEN_MODE_TRADER
	GEN_MODE_LOCAL
	GEN_MODE_BALANCE
	GEN_MODE_PAYMENT
	GEN_MODE_WALLET
	GEN_MODE_COUNT
	GEN_MODE_CHECK
	GEN_MODE_WATCHDOG
)

func (gm GenMode) Type() string {
	return []string{
		"genesis",
		"trader",
		"local",
		"balance",
		"payment",
		"wallet",
		"count",
		"check",
		"watchdog"}[gm]
}

func Must(gm GenMode, ok bool) GenMode {
	if !ok {
		panic("Must parse")
	}
	return gm
}

func ModeMapper(k string) (GenMode, bool) {
	v, ok := map[string]GenMode{
		GEN_MODE_GENESIS.Type():  GEN_MODE_GENESIS,
		GEN_MODE_TRADER.Type():   GEN_MODE_TRADER,
		GEN_MODE_LOCAL.Type():    GEN_MODE_LOCAL,
		GEN_MODE_BALANCE.Type():  GEN_MODE_BALANCE,
		GEN_MODE_PAYMENT.Type():  GEN_MODE_PAYMENT,
		GEN_MODE_WALLET.Type():   GEN_MODE_WALLET,
		GEN_MODE_COUNT.Type():    GEN_MODE_COUNT,
		GEN_MODE_CHECK.Type():    GEN_MODE_CHECK,
		GEN_MODE_WATCHDOG.Type(): GEN_MODE_WATCHDOG,
	}[k]
	return v, ok
}

// command factory

type CommandFactory interface {
	Create(mode GenMode) Command
}

type CmdArgFactory struct {
}

func (f *CmdArgFactory) Create(mode GenMode) Command {
	switch mode {
	case GEN_MODE_PAYMENT:
		return &PaymentCommand{}
	case GEN_MODE_WALLET:
		return &WalletCommand{}
	case GEN_MODE_COUNT:
		return &CountCommand{}
	case GEN_MODE_WATCHDOG:
		return &WatchDogCommand{}
	}
	return nil
}

func GeDefaultCmdFactory() CommandFactory {
	return &CmdArgFactory{}
}
