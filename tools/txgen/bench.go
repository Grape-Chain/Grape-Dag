package txgen

import (
	"context"
	"flag"
	"fmt"
	"math/big"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Grape-Chain/Grape-Dag/config"
	grape1crypto "github.com/Grape-Chain/Grape-Dag/crypto"
	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"github.com/Grape-Chain/Grape-Dag/wallet"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

// Bench mode measures the maximum sustained publish throughput of one node.
//
// It differs from trader mode in four ways, each of which was a measured ceiling
// in that mode rather than a preference:
//
//   - Senders run in parallel, each on its own connection. gRPC multiplexes every
//     call on a ClientConn onto one HTTP/2 connection, so a single client makes
//     the connection, not the node, the limit.
//   - There is one round trip per transaction. Trader mode called GetBalances
//     before every PublishTx, and two serial round trips per transaction caps
//     throughput at a few hundred per second however many senders there are.
//     Balances are tracked locally here instead.
//   - Signing happens before the timed window. ed25519 over a SHA-256 of the
//     marshalled transaction is tens of microseconds, so signing inside the send
//     loop charges the node's score for the generator's own CPU.
//   - There are no sleeps in the send path. Trader mode slept a full second every
//     thousand transactions, which alone capped the average rate.
const (
	benchDefaultWorkers  = 32
	benchDefaultDuration = 30 * time.Second
	benchDefaultTimeout  = 5 * time.Second
	benchDefaultReport   = 5 * time.Second
	benchDefaultSettle   = 30 * time.Second

	// benchDefaultPrepare is the pre-signed pool size when there is no rate and
	// no transaction cap to derive one from, which is the case for a -bench_max
	// run. At roughly 0.6 KB per pre-signed transaction this is a few hundred
	// megabytes of held load; raise it with -bench_prepare for a longer run and
	// watch the generator's own memory when you do.
	benchDefaultPrepare = 200000

	// benchRecipientCount is the number of receive-only addresses payments are
	// spread over. A handful is enough to keep the transactions distinct without
	// generating a keypair per transaction during setup.
	benchRecipientCount = 64

	// benchFundHeadroom multiplies what a sender wallet is asked to spend when
	// deciding what the faucet sends it, so a run is not cut short by a rounding
	// difference or by a fee the node charges that the generator does not model.
	benchFundHeadroom = 4

	// benchFundTimeout is the deadline on one faucet payment. Generous, because
	// this is sequential setup and a slow faucet publish is not a measurement.
	benchFundTimeout = 30 * time.Second

	// benchDialTimeout is how long a worker's connection has to reach Ready. A
	// dial that fails should fail as a setup error rather than as thousands of
	// failed publishes inside the window.
	benchDialTimeout = 15 * time.Second

	benchSettlePoll = 500 * time.Millisecond

	// benchShutdownFailure names the publishes that were in flight when the
	// window closed, so they are not read as the node refusing load.
	benchShutdownFailure = "cut short by shutdown"
)

// BenchOptions holds the bench-mode settings that come from the command line.
type BenchOptions struct {
	Workers  int
	Rate     uint64
	Max      bool
	Duration time.Duration
	Txmax    uint64
	Wallets  int
	Prepare  uint64
	Amount   uint64
	Fund     uint64
	Timeout  time.Duration
	Report   time.Duration
	Settle   time.Duration
}

// RegisterBenchFlags declares the bench-mode flags on fs. It lives here rather
// than in cmd/txgen so that adding a bench setting does not mean editing the
// command line plumbing shared with every other mode.
func RegisterBenchFlags(fs *flag.FlagSet) *BenchOptions {
	o := &BenchOptions{}
	fs.IntVar(&o.Workers, "bench_workers", benchDefaultWorkers,
		"bench mode: parallel senders, each with its own gRPC connection")
	fs.Uint64Var(&o.Rate, "bench_rate", 0,
		"bench mode: offered transactions per second across all workers; 0 offers as fast as the node accepts")
	fs.BoolVar(&o.Max, "bench_max", false,
		"bench mode: offer as fast as the node accepts, ignoring -bench_rate")
	fs.DurationVar(&o.Duration, "bench_duration", benchDefaultDuration,
		"bench mode: length of the timed window")
	fs.Uint64Var(&o.Txmax, "bench_txmax", 0,
		"bench mode: stop after this many transactions; 0 runs for -bench_duration")
	fs.IntVar(&o.Wallets, "bench_wallets", 0,
		"bench mode: faucet-funded sender wallets; 0 means one per worker")
	fs.Uint64Var(&o.Prepare, "bench_prepare", 0,
		"bench mode: transactions to pre-sign before the timed window; 0 derives it from the rate and duration")
	fs.Uint64Var(&o.Amount, "bench_amount", 1,
		"bench mode: amount each pre-signed payment moves")
	fs.Uint64Var(&o.Fund, "bench_fund", 0,
		"bench mode: faucet amount per sender wallet; 0 derives it from -bench_prepare and -bench_amount")
	fs.DurationVar(&o.Timeout, "bench_timeout", benchDefaultTimeout,
		"bench mode: deadline for one PublishTx call")
	fs.DurationVar(&o.Report, "bench_report", benchDefaultReport,
		"bench mode: interval between progress reports; 0 reports only at the end")
	fs.DurationVar(&o.Settle, "bench_settle", benchDefaultSettle,
		"bench mode: how long to wait for faucet funding to show up in balances; 0 does not wait or check")
	return o
}

func (o *BenchOptions) validate() error {
	switch {
	case o.Workers < 1:
		return fmt.Errorf("bench: -bench_workers must be at least 1, got %d", o.Workers)
	case o.Amount == 0:
		return fmt.Errorf("bench: -bench_amount must be greater than 0")
	case o.Timeout <= 0:
		return fmt.Errorf("bench: -bench_timeout must be positive, got %s", o.Timeout)
	case o.Duration <= 0 && o.Txmax == 0:
		return fmt.Errorf("bench: nothing bounds the run - set -bench_duration or -bench_txmax")
	case o.Wallets < 0:
		return fmt.Errorf("bench: -bench_wallets cannot be negative, got %d", o.Wallets)
	}
	return nil
}

// offeredRate - the pacing rate in transactions per second, 0 meaning unpaced.
func (o *BenchOptions) offeredRate() float64 {
	if o.Max {
		return 0
	}
	return float64(o.Rate)
}

// walletCount - how many sender wallets the faucet has to fund. Every worker
// needs at least one of its own: two workers spending one wallet would race on
// the locally tracked balance and could pre-sign more than the faucet funded.
func (o *BenchOptions) walletCount() int {
	if o.Wallets < o.Workers {
		return o.Workers
	}
	return o.Wallets
}

// prepareCount - how many transactions to pre-sign. An explicit -bench_prepare
// wins; otherwise a bounded run pre-signs exactly what it will send, a paced run
// pre-signs what the rate implies plus a tenth so that pacing drift does not end
// the run early, and an unpaced run has nothing to derive from and takes the
// default.
func (o *BenchOptions) prepareCount() uint64 {
	switch {
	case o.Prepare > 0:
		return o.Prepare
	case o.Txmax > 0:
		return o.Txmax
	case o.offeredRate() > 0 && o.Duration > 0:
		return uint64(o.offeredRate()*o.Duration.Seconds()*1.1) + uint64(o.Workers)
	default:
		return benchDefaultPrepare
	}
}

// fundAmount - what the faucet sends each sender wallet.
func (o *BenchOptions) fundAmount(perWallet uint64) *big.Int {
	if o.Fund > 0 {
		return new(big.Int).SetUint64(o.Fund)
	}
	spend := new(big.Int).SetUint64(perWallet)
	spend.Mul(spend, new(big.Int).SetUint64(o.Amount))
	spend.Mul(spend, big.NewInt(benchFundHeadroom))
	if spend.Sign() == 0 {
		spend.SetUint64(o.Amount * benchFundHeadroom)
	}
	return spend
}

// benchWallet is a faucet-funded sender. The balance is tracked here rather than
// asked of the node, which is what removes the GetBalances round trip that
// halved trader mode's throughput. The nonce is ours too, so that every
// pre-signed transaction hashes differently.
type benchWallet struct {
	w       *grape1crypto.Wallet
	balance *big.Int
	nonce   uint64
}

func (bw *benchWallet) canSpend(amount *big.Int) bool {
	return bw.balance.Cmp(amount) >= 0
}

// benchWorker owns everything one sender goroutine touches: its own connection,
// its own funded wallets and its own pre-signed transactions. Nothing here is
// shared, so the timed window has no contention beyond the counters and the
// token bucket.
type benchWorker struct {
	id      int
	conn    *grpc.ClientConn
	client  pb.RoboTraderClient
	senders []*benchWallet
	load    []*pb.Txv1
	next    int
}

// take hands out the next pre-signed transaction, or nil when the pool is spent.
// Called from one goroutine only, so no synchronisation.
func (w *benchWorker) take() *pb.Txv1 {
	if w.next >= len(w.load) {
		return nil
	}
	t := w.load[w.next]
	w.next++
	return t
}

// benchPlan is everything the setup phase produced for the timed window.
type benchPlan struct {
	opts       *BenchOptions
	amount     *big.Int
	fund       *big.Int
	network    uint8
	wallets    []*benchWallet
	recipients []string
	perWorker  int
	target     string
}

// Bench runs a saturation measurement against the configured node and prints the
// report. cltService is the connection main already opened; it is used for the
// sequential setup only, never inside the timed window.
func (g *TxGenerator) Bench(cltService *pb.RoboTraderClient, opts *BenchOptions) error {
	if opts == nil {
		return fmt.Errorf("bench: no options given")
	}
	if err := opts.validate(); err != nil {
		return err
	}
	if len(g.Wallets) == 0 {
		return fmt.Errorf("bench: no faucet wallet loaded - generator.publickey and generator.privatekey must name the genesis wallet")
	}

	// SIGINT cancels the window and falls through to the report. A load test that
	// throws away its numbers when interrupted is a load test nobody interrupts,
	// so they let a bad run finish instead.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	plan := g.newBenchPlan(opts)
	fmt.Printf("[bench] %d workers, %d sender wallets, %s per transaction, pre-signing %d transactions\n",
		opts.Workers, len(plan.wallets), plan.amount.String(), opts.prepareCount())

	if err := g.fundBenchWallets(ctx, cltService, plan); err != nil {
		return err
	}
	if err := g.awaitBenchFunding(ctx, cltService, plan); err != nil {
		return err
	}

	workers, err := dialBenchWorkers(ctx, plan)
	if err != nil {
		return err
	}
	defer closeBenchWorkers(workers)

	presignBenchLoad(plan, workers)
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("bench: interrupted during setup")
	}

	report := runBench(ctx, opts, workers)
	fmt.Print(report.String())
	return nil
}

func (g *TxGenerator) newBenchPlan(opts *BenchOptions) *benchPlan {
	count := opts.walletCount()
	prepare := opts.prepareCount()

	// Round the per-worker share up, so the pool is never short of the requested
	// total because of integer division.
	perWorker := int((prepare + uint64(opts.Workers) - 1) / uint64(opts.Workers))
	perWallet := uint64(perWorker*opts.Workers/count) + 1

	plan := &benchPlan{
		opts:      opts,
		amount:    new(big.Int).SetUint64(opts.Amount),
		network:   g.Generator.Network,
		perWorker: perWorker,
		target:    fmt.Sprintf("%s:%d", g.Generator.Nodeip, g.Generator.Nodeport),
	}
	plan.fund = opts.fundAmount(perWallet)

	plan.wallets = make([]*benchWallet, count)
	for i := range plan.wallets {
		plan.wallets[i] = &benchWallet{
			w:       grape1crypto.NewWallet(),
			balance: big.NewInt(0),
		}
	}
	plan.recipients = make([]string, benchRecipientCount)
	for i := range plan.recipients {
		plan.recipients[i] = grape1crypto.NewWallet().WalletAddress()
	}
	return plan
}

// fundBenchWallets moves faucet coin out to the sender pool, one payment at a
// time, before the window opens. Sequential on purpose: this is setup, and a
// faucet publishing concurrently with itself hands the node several payments
// drawn from one balance at once.
func (g *TxGenerator) fundBenchWallets(ctx context.Context, cltService *pb.RoboTraderClient, plan *benchPlan) error {
	faucet := g.Wallets[0]
	needed := new(big.Int).Mul(plan.fund, big.NewInt(int64(len(plan.wallets))))
	if have, ok := g.Balances[faucet.WalletAddress()]; ok && have.Cmp(needed) < 0 {
		return fmt.Errorf("bench: faucet %s holds %s but funding %d wallets with %s each needs %s - lower -bench_prepare or set -bench_fund",
			faucet.WalletAddress(), have.String(), len(plan.wallets), plan.fund.String(), needed.String())
	}
	fmt.Printf("[bench] funding %d wallets with %s each from faucet %s\n",
		len(plan.wallets), plan.fund.String(), faucet.WalletAddress())

	var nonce uint64
	for i, bw := range plan.wallets {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("bench: interrupted while funding wallet %d of %d", i, len(plan.wallets))
		}
		txv := tx.NewTxv1(tx.ChainType(plan.network))
		txv.GeneratePayment(wallet.GenPaymentEx(faucet, bw.w.WalletAddress(), plan.fund), plan.network)
		txv.Nonce = nonce
		nonce++
		txv.Sign(faucet.PrivateKey())

		callCtx, cancel := context.WithTimeout(ctx, benchFundTimeout)
		_, err := (*cltService).PublishTx(callCtx, &pb.TxPublishRequest{Tx: txv.MarshalBinary()})
		cancel()
		if err != nil {
			return fmt.Errorf("bench: funding wallet %d of %d failed: %s", i+1, len(plan.wallets), err.Error())
		}
		// Optimistic until awaitBenchFunding reconciles against the node. A
		// balance the node never credited shows up there, not as a silent
		// under-funded run.
		bw.balance.Set(plan.fund)
	}
	return nil
}

// awaitBenchFunding waits until the node reports the funding it was sent, and
// seeds the local balances from what it actually reports. Without this the run
// would start while the faucet payments are still in the node's queue, and every
// sender would be spending coin the node does not think it has.
func (g *TxGenerator) awaitBenchFunding(ctx context.Context, cltService *pb.RoboTraderClient, plan *benchPlan) error {
	if plan.opts.Settle <= 0 {
		fmt.Println("[bench] -bench_settle 0: not waiting for funding to settle")
		return nil
	}
	addresses := make([][]byte, len(plan.wallets))
	for i, bw := range plan.wallets {
		addresses[i] = grape1crypto.AddressToBytes(bw.w.WalletAddress())
	}

	deadline := time.Now().Add(plan.opts.Settle)
	for {
		callCtx, cancel := context.WithTimeout(ctx, benchFundTimeout)
		res, err := (*cltService).GetBalances(callCtx, &pb.BalanceRequest{Wallets: addresses})
		cancel()
		if err == nil && len(res.Balances) == len(plan.wallets) {
			funded := 0
			for i, b := range res.Balances {
				balance := new(big.Int).SetBytes(b)
				if balance.Cmp(plan.fund) >= 0 {
					funded++
				}
				plan.wallets[i].balance.Set(balance)
			}
			if funded == len(plan.wallets) {
				fmt.Printf("[bench] all %d sender wallets funded\n", funded)
				return nil
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("bench: only %d of %d sender wallets were funded within %s - raise -bench_settle, or lower -bench_prepare so the faucet sends less",
					funded, len(plan.wallets), plan.opts.Settle)
			}
		} else if time.Now().After(deadline) {
			return fmt.Errorf("bench: could not confirm funding within %s: %v", plan.opts.Settle, err)
		}
		if !sleepCtx(ctx, benchSettlePoll) {
			return fmt.Errorf("bench: interrupted while waiting for funding to settle")
		}
	}
}

// benchDial opens one connection for one worker. Each worker gets its own,
// because gRPC puts every call on a ClientConn onto a single HTTP/2 connection:
// sharing one serialises the workers behind that connection's stream limit and
// flow control window, and that - not the node - was what the old single-client
// loop was measuring.
func benchDial(ctx context.Context, target string) (*grpc.ClientConn, error) {
	conn, err := grpc.NewClient(
		// passthrough, so the target is used as the host:port it already is and
		// no name resolution happens per worker.
		"passthrough:///"+target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(10*config.MB),
			grpc.MaxCallSendMsgSize(10*config.MB),
		),
	)
	if err != nil {
		return nil, err
	}
	conn.Connect()
	if err := awaitBenchReady(ctx, conn, benchDialTimeout); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func awaitBenchReady(ctx context.Context, conn *grpc.ClientConn, timeout time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		state := conn.GetState()
		if state == connectivity.Ready {
			return nil
		}
		if !conn.WaitForStateChange(waitCtx, state) {
			return fmt.Errorf("bench: connection to %s stuck in state %s after %s", conn.Target(), state, timeout)
		}
	}
}

func dialBenchWorkers(ctx context.Context, plan *benchPlan) ([]*benchWorker, error) {
	workers := make([]*benchWorker, 0, plan.opts.Workers)
	for i := 0; i < plan.opts.Workers; i++ {
		conn, err := benchDial(ctx, plan.target)
		if err != nil {
			closeBenchWorkers(workers)
			return nil, err
		}
		w := &benchWorker{
			id:     i,
			conn:   conn,
			client: pb.NewRoboTraderClient(conn),
		}
		// Wallets are dealt out in strides so that each worker owns a disjoint
		// set. Sharing one would mean two goroutines pre-signing against the same
		// tracked balance.
		for j := i; j < len(plan.wallets); j += plan.opts.Workers {
			w.senders = append(w.senders, plan.wallets[j])
		}
		workers = append(workers, w)
	}
	fmt.Printf("[bench] %d connections to %s ready\n", len(workers), plan.target)
	return workers, nil
}

func closeBenchWorkers(workers []*benchWorker) {
	for _, w := range workers {
		if w.conn != nil {
			w.conn.Close()
		}
	}
}

// presignBenchLoad generates and signs the whole offered load before the window
// opens, in parallel across workers because each one only touches its own
// wallets. This is the step that keeps ed25519 out of the measurement.
func presignBenchLoad(plan *benchPlan, workers []*benchWorker) {
	started := time.Now()
	var wg sync.WaitGroup
	for _, w := range workers {
		wg.Add(1)
		go func(w *benchWorker) {
			defer wg.Done()
			presignWorkerLoad(plan, w)
		}(w)
	}
	wg.Wait()

	total := 0
	for _, w := range workers {
		total += len(w.load)
	}
	fmt.Printf("[bench] pre-signed %d transactions in %s\n", total, time.Since(started).Round(time.Millisecond))
}

func presignWorkerLoad(plan *benchPlan, w *benchWorker) {
	w.load = make([]*pb.Txv1, 0, plan.perWorker)
	for i := 0; i < plan.perWorker; i++ {
		sender := pickBenchSender(w.senders, plan.amount)
		if sender == nil {
			// Every wallet this worker owns is spent. Funding more would need
			// another faucet round and a second settle wait, so the worker takes
			// the shorter pool and the report shows the shorter window.
			return
		}
		recipient := plan.recipients[(i+w.id)%len(plan.recipients)]

		txv := tx.NewTxv1(tx.ChainType(plan.network))
		txv.GeneratePayment(wallet.GenPaymentEx(sender.w, recipient, plan.amount), plan.network)
		// A nonce of our own, in place of the random one GeneratePayment picks, so
		// that no two pre-signed transactions can hash alike. Duplicates would be
		// counted as offered here and then deduplicated by the node, which reads
		// as throughput the node never had.
		txv.Nonce = sender.nonce
		sender.nonce++
		txv.Sign(sender.w.PrivateKey())

		sender.balance.Sub(sender.balance, plan.amount)
		w.load = append(w.load, txv.MarshalBinary())
	}
}

// pickBenchSender returns the first wallet that can still cover amount, or nil
// when none can.
func pickBenchSender(senders []*benchWallet, amount *big.Int) *benchWallet {
	for _, s := range senders {
		if s.canSpend(amount) {
			return s
		}
	}
	return nil
}

// runBench opens the timed window: it starts the workers, bounds the run by
// duration, transaction count, pool exhaustion or SIGINT, and returns the final
// snapshot.
func runBench(ctx context.Context, opts *BenchOptions, workers []*benchWorker) benchReport {
	rate := opts.offeredRate()
	capacity, batch := benchBurst(rate, len(workers))
	bucket := newTokenBucket(rate, capacity)
	budget := newTxBudget(opts.Txmax)
	stats := newBenchStats()

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if opts.Duration > 0 {
		stopAt := time.AfterFunc(opts.Duration, cancel)
		defer stopAt.Stop()
	}

	if rate > 0 {
		fmt.Printf("[bench] offering %.0f tx/s (bucket capacity %.0f, %d per worker draw), %s\n",
			rate, capacity, batch, describeBenchBound(opts))
	} else {
		fmt.Printf("[bench] offering as fast as the node accepts, %s\n", describeBenchBound(opts))
	}

	started := time.Now()
	reporterDone := startBenchReporter(runCtx, stats, started, opts.Report)

	var wg sync.WaitGroup
	for _, w := range workers {
		wg.Add(1)
		go func(w *benchWorker) {
			defer wg.Done()
			benchSend(runCtx, w, bucket, batch, budget, stats, opts.Timeout)
		}(w)
	}
	wg.Wait()
	elapsed := time.Since(started)

	cancel()
	<-reporterDone
	return stats.snapshot(elapsed)
}

// describeBenchBound says what will end the run, so the opening line does not
// print a window of 0s for a run bounded only by -bench_txmax.
func describeBenchBound(opts *BenchOptions) string {
	switch {
	case opts.Duration > 0 && opts.Txmax > 0:
		return fmt.Sprintf("for %s or %d transactions, whichever comes first", opts.Duration, opts.Txmax)
	case opts.Txmax > 0:
		return fmt.Sprintf("until %d transactions have been sent", opts.Txmax)
	default:
		return fmt.Sprintf("for %s", opts.Duration)
	}
}

// benchSend is one worker's send loop. No sleeps of its own: it either has a
// permit and publishes, or it is waiting on the shared bucket.
func benchSend(ctx context.Context, w *benchWorker, bucket *tokenBucket, batch int, budget *txBudget, stats *benchStats, timeout time.Duration) {
	for ctx.Err() == nil {
		granted := bucket.acquire(ctx, batch)
		if granted == 0 {
			return // context finished while waiting for a permit
		}
		granted = budget.claim(granted)
		if granted == 0 {
			return // -bench_txmax reached
		}
		for i := 0; i < granted; i++ {
			txv := w.take()
			if txv == nil {
				return // pre-signed pool spent
			}
			publishBenchTx(ctx, w, txv, stats, timeout)
		}
	}
}

// publishBenchTx is the whole of the measured path: one round trip, no signing,
// no balance query, no sleep.
func publishBenchTx(ctx context.Context, w *benchWorker, txv *pb.Txv1, stats *benchStats, timeout time.Duration) {
	stats.offered.Add(1)
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	started := time.Now()
	res, err := w.client.PublishTx(callCtx, &pb.TxPublishRequest{Tx: txv})
	latency := time.Since(started)
	cancel()

	switch {
	case err != nil && ctx.Err() != nil:
		// The run ended while this call was in flight. That is our own shutdown
		// rather than the node refusing work, so it is named as such instead of
		// being counted against the node. Expect up to one per worker at the end
		// of every run.
		stats.recordFailure(benchShutdownFailure)
	case err != nil:
		stats.recordFailure(classifyPublishError(err))
	case res.GetStatus() != 0:
		stats.recordFailure(fmt.Sprintf("node/status=%d", res.GetStatus()))
	default:
		stats.recordAccepted(latency)
	}
}

// startBenchReporter prints a progress line every interval and returns a channel
// closed when it has stopped, so the final report never interleaves with a
// progress line.
func startBenchReporter(ctx context.Context, stats *benchStats, started time.Time, every time.Duration) <-chan struct{} {
	done := make(chan struct{})
	if every <= 0 {
		close(done)
		return done
	}
	go func() {
		defer close(done)
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		previous := stats.snapshot(0)
		for {
			select {
			case <-ctx.Done():
				return
			case at := <-ticker.C:
				current := stats.snapshot(at.Sub(started))
				fmt.Println(current.progress(previous))
				previous = current
			}
		}
	}()
	return done
}
