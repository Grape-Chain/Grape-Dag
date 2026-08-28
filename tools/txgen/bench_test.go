package txgen

import (
	"context"
	"flag"
	"math/big"
	"sync/atomic"
	"testing"
	"time"

	grape1crypto "github.com/Grape-Chain/Grape-Dag/crypto"
	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// stubTrader stands in for a node so the send path can be measured without one.
// The embedded interface satisfies the methods bench mode never calls; touching
// one of those would panic, which is the behaviour a test wants.
type stubTrader struct {
	pb.RoboTraderClient
	published atomic.Uint64
	failWith  error
}

func (s *stubTrader) PublishTx(_ context.Context, _ *pb.TxPublishRequest, _ ...grpc.CallOption) (*pb.TxPublishResponse, error) {
	s.published.Add(1)
	if s.failWith != nil {
		return nil, s.failWith
	}
	return &pb.TxPublishResponse{Status: 0, Msg: "success"}, nil
}

// stubWorkers builds workers whose pre-signed pool is filler: runBench never
// looks inside a transaction, so an empty one exercises the same path.
func stubWorkers(count, load int, stub *stubTrader) []*benchWorker {
	workers := make([]*benchWorker, count)
	for i := range workers {
		w := &benchWorker{id: i, client: stub, load: make([]*pb.Txv1, load)}
		for j := range w.load {
			w.load[j] = &pb.Txv1{}
		}
		workers[i] = w
	}
	return workers
}

func TestBenchFlagsHaveTheDocumentedDefaults(t *testing.T) {
	fs := flag.NewFlagSet("txgen", flag.ContinueOnError)
	o := RegisterBenchFlags(fs)
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parsing an empty command line: %s", err)
	}
	cases := []struct {
		flag string
		got  any
		want any
	}{
		{"bench_workers", o.Workers, benchDefaultWorkers},
		{"bench_rate", o.Rate, uint64(0)},
		{"bench_max", o.Max, false},
		{"bench_duration", o.Duration, benchDefaultDuration},
		{"bench_txmax", o.Txmax, uint64(0)},
		{"bench_wallets", o.Wallets, 0},
		{"bench_prepare", o.Prepare, uint64(0)},
		{"bench_amount", o.Amount, uint64(1)},
		{"bench_timeout", o.Timeout, benchDefaultTimeout},
		{"bench_report", o.Report, benchDefaultReport},
		{"bench_settle", o.Settle, benchDefaultSettle},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("-%s defaults to %v, want %v", c.flag, c.got, c.want)
		}
	}
	if err := o.validate(); err != nil {
		t.Errorf("the defaults should be a runnable configuration, but: %s", err)
	}
	if o.offeredRate() != 0 {
		t.Errorf("the default offered rate is %.0f, want 0 - an unpaced run is what finds the ceiling", o.offeredRate())
	}
}

func TestBenchMaxOverridesAnExplicitRate(t *testing.T) {
	fs := flag.NewFlagSet("txgen", flag.ContinueOnError)
	o := RegisterBenchFlags(fs)
	if err := fs.Parse([]string{"-bench_rate", "5000", "-bench_max"}); err != nil {
		t.Fatalf("parsing: %s", err)
	}
	if o.offeredRate() != 0 {
		t.Errorf("-bench_max with -bench_rate 5000 offers at %.0f/s, want unpaced", o.offeredRate())
	}
}

func TestBenchOptionsRejectSettingsThatCannotBeMeasured(t *testing.T) {
	base := func() *BenchOptions {
		return &BenchOptions{Workers: 4, Amount: 1, Timeout: time.Second, Duration: time.Second}
	}
	cases := []struct {
		name  string
		mutot func(*BenchOptions)
	}{
		{"no workers", func(o *BenchOptions) { o.Workers = 0 }},
		{"negative workers", func(o *BenchOptions) { o.Workers = -1 }},
		{"payments of nothing", func(o *BenchOptions) { o.Amount = 0 }},
		{"no publish deadline", func(o *BenchOptions) { o.Timeout = 0 }},
		{"unbounded run", func(o *BenchOptions) { o.Duration = 0; o.Txmax = 0 }},
		{"negative wallet pool", func(o *BenchOptions) { o.Wallets = -1 }},
	}
	for _, c := range cases {
		o := base()
		c.mutot(o)
		if err := o.validate(); err == nil {
			t.Errorf("%s: validate accepted %+v, want an error", c.name, o)
		}
	}
	// A transaction cap alone bounds the run, so no duration is needed.
	o := base()
	o.Duration = 0
	o.Txmax = 100
	if err := o.validate(); err != nil {
		t.Errorf("a run bounded by -bench_txmax alone should be valid, but: %s", err)
	}
}

func TestPrepareCountFollowsTheRateAndDuration(t *testing.T) {
	cases := []struct {
		name string
		opts BenchOptions
		want uint64
	}{
		{"explicit wins", BenchOptions{Prepare: 7, Rate: 1000, Duration: time.Minute, Workers: 4}, 7},
		{"a capped run pre-signs exactly its cap", BenchOptions{Txmax: 5000, Workers: 4}, 5000},
		{"a paced run pre-signs the rate times the window plus a tenth",
			BenchOptions{Rate: 1000, Duration: 10 * time.Second, Workers: 8}, 11008},
		{"an unpaced run has nothing to derive from",
			BenchOptions{Duration: 30 * time.Second, Max: true, Workers: 32}, benchDefaultPrepare},
		{"a rate of zero is unpaced too",
			BenchOptions{Rate: 0, Duration: 30 * time.Second, Workers: 32}, benchDefaultPrepare},
	}
	for _, c := range cases {
		if got := c.opts.prepareCount(); got != c.want {
			t.Errorf("%s: prepareCount() = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestEveryWorkerGetsASenderWalletOfItsOwn(t *testing.T) {
	cases := []struct {
		workers, wallets, want int
	}{
		{32, 0, 32},  // the default: one per worker
		{32, 8, 32},  // fewer than the workers would mean two workers on one wallet
		{32, 64, 64}, // more is allowed, they are dealt out in strides
		{1, 1, 1},
	}
	for _, c := range cases {
		o := &BenchOptions{Workers: c.workers, Wallets: c.wallets}
		if got := o.walletCount(); got != c.want {
			t.Errorf("walletCount() with %d workers and -bench_wallets %d = %d, want %d",
				c.workers, c.wallets, got, c.want)
		}
	}
}

func TestFundingCoversWhatAWalletIsAskedToSpend(t *testing.T) {
	o := &BenchOptions{Amount: 10}
	got := o.fundAmount(100)
	want := big.NewInt(10 * 100 * benchFundHeadroom)
	if got.Cmp(want) != 0 {
		t.Errorf("fundAmount(100) = %s, want %s (spend times headroom)", got, want)
	}
	explicit := &BenchOptions{Amount: 10, Fund: 42}
	if got := explicit.fundAmount(100); got.Cmp(big.NewInt(42)) != 0 {
		t.Errorf("-bench_fund 42 gave %s, want 42", got)
	}
}

// TestPreSignedTransactionsAreDistinctAndVerify is the property the timed window
// depends on: the load is fully signed and valid before the window opens, and no
// two transactions are alike, so the node cannot deduplicate the load away and
// leave a throughput figure that never happened.
func TestPreSignedTransactionsAreDistinctAndVerify(t *testing.T) {
	const count = 120
	sender := &benchWallet{w: grape1crypto.NewWallet(), balance: big.NewInt(10000)}
	plan := &benchPlan{
		opts:       &BenchOptions{Workers: 1, Amount: 1},
		amount:     big.NewInt(1),
		network:    0,
		recipients: []string{grape1crypto.NewWallet().WalletAddress(), grape1crypto.NewWallet().WalletAddress()},
		perWorker:  count,
	}
	w := &benchWorker{id: 0, senders: []*benchWallet{sender}}

	presignWorkerLoad(plan, w)

	if len(w.load) != count {
		t.Fatalf("pre-signed %d transactions, want %d", len(w.load), count)
	}
	hashes := make(map[string]bool, count)
	nonces := make(map[uint64]bool, count)
	for i, pbtx := range w.load {
		var txv tx.Txv1
		txv.UnmarshalBinary(pbtx)
		if err := txv.Verify(); err != nil {
			t.Fatalf("transaction %d does not verify: %s", i, err)
		}
		if hashes[txv.DefaultStringHash()] {
			t.Fatalf("transaction %d repeats the hash of an earlier one", i)
		}
		hashes[txv.DefaultStringHash()] = true
		if nonces[txv.Nonce] {
			t.Fatalf("transaction %d repeats nonce %d", i, txv.Nonce)
		}
		nonces[txv.Nonce] = true
	}
	if want := big.NewInt(10000 - count); sender.balance.Cmp(want) != 0 {
		t.Errorf("locally tracked balance = %s after %d payments of 1, want %s", sender.balance, count, want)
	}
}

func TestPreSigningStopsWhenTheFundedBalanceIsSpent(t *testing.T) {
	sender := &benchWallet{w: grape1crypto.NewWallet(), balance: big.NewInt(3)}
	plan := &benchPlan{
		opts:       &BenchOptions{Workers: 1, Amount: 1},
		amount:     big.NewInt(1),
		recipients: []string{grape1crypto.NewWallet().WalletAddress()},
		perWorker:  10,
	}
	w := &benchWorker{senders: []*benchWallet{sender}}
	presignWorkerLoad(plan, w)
	if len(w.load) != 3 {
		t.Errorf("pre-signed %d transactions from a balance of 3, want 3 - the generator must not offer what the sender cannot pay", len(w.load))
	}
}

func TestAWorkerHandsOutEachPreSignedTransactionExactlyOnce(t *testing.T) {
	w := stubWorkers(1, 5, &stubTrader{})[0]
	seen := 0
	for {
		txv := w.take()
		if txv == nil {
			break
		}
		seen++
	}
	if seen != 5 {
		t.Errorf("a worker handed out %d of its 5 pre-signed transactions", seen)
	}
	if w.take() != nil {
		t.Error("a spent pool should keep reporting nothing")
	}
}

func TestRunBenchOffersExactlyTheTransactionBudget(t *testing.T) {
	stub := &stubTrader{}
	workers := stubWorkers(4, 1000, stub)
	opts := &BenchOptions{
		Workers:  4,
		Max:      true,
		Duration: 30 * time.Second, // never reached: the budget ends the run
		Txmax:    500,
		Amount:   1,
		Timeout:  time.Second,
	}
	report := runBench(context.Background(), opts, workers)

	if report.Offered != 500 {
		t.Errorf("offered %d transactions, want the -bench_txmax of 500", report.Offered)
	}
	if report.Accepted != 500 {
		t.Errorf("accepted %d of 500, want all of them from a node that takes everything", report.Accepted)
	}
	if report.Failed != 0 {
		t.Errorf("failed %d, want 0", report.Failed)
	}
	if report.AcceptedRatio() != 1 {
		t.Errorf("accepted/offered = %.4f, want 1", report.AcceptedRatio())
	}
	if got := stub.published.Load(); got != 500 {
		t.Errorf("the node saw %d publishes, want 500", got)
	}
}

func TestRunBenchStopsWhenThePreSignedPoolIsSpent(t *testing.T) {
	stub := &stubTrader{}
	workers := stubWorkers(4, 25, stub)
	opts := &BenchOptions{
		Workers:  4,
		Max:      true,
		Duration: time.Minute, // the pool runs out long before this
		Amount:   1,
		Timeout:  time.Second,
	}
	started := time.Now()
	report := runBench(context.Background(), opts, workers)

	if report.Offered != 100 {
		t.Errorf("offered %d transactions, want the 100 that were pre-signed", report.Offered)
	}
	if elapsed := time.Since(started); elapsed > 30*time.Second {
		t.Errorf("the run took %s, so it waited out -bench_duration instead of stopping at the spent pool", elapsed)
	}
}

func TestRunBenchCountsFailuresByCause(t *testing.T) {
	stub := &stubTrader{failWith: status.Error(codes.ResourceExhausted, "publish queue full")}
	workers := stubWorkers(2, 100, stub)
	opts := &BenchOptions{Workers: 2, Max: true, Duration: time.Minute, Txmax: 60, Amount: 1, Timeout: time.Second}

	report := runBench(context.Background(), opts, workers)

	if report.Accepted != 0 {
		t.Errorf("accepted %d from a node that refuses everything, want 0", report.Accepted)
	}
	if report.Failed != 60 {
		t.Errorf("failed %d, want 60", report.Failed)
	}
	if got := report.Errors["grpc/resourceexhausted"]; got != 60 {
		t.Errorf("breakdown gives %d resource-exhausted failures, want 60 (%v)", got, report.Errors)
	}
	if report.AcceptedRatio() != 0 {
		t.Errorf("accepted/offered = %.4f, want 0", report.AcceptedRatio())
	}
}

func TestRunBenchIsPacedByTheOfferedRate(t *testing.T) {
	stub := &stubTrader{}
	workers := stubWorkers(4, 10000, stub)
	opts := &BenchOptions{
		Workers:  4,
		Rate:     500,
		Duration: 400 * time.Millisecond,
		Amount:   1,
		Timeout:  time.Second,
	}
	report := runBench(context.Background(), opts, workers)

	// 500/s for 0.4s is 200 transactions. The bucket starts empty and the window
	// is short, so allow generous slack below; what matters is that pacing holds
	// the offered rate near the target instead of running flat out, which against
	// this stub would be hundreds of thousands.
	if report.Offered == 0 || report.Offered > 260 {
		t.Errorf("offered %d transactions at 500/s over %s, want about 200", report.Offered, opts.Duration)
	}
}

func TestRunBenchStopsOnACancelledContext(t *testing.T) {
	stub := &stubTrader{}
	workers := stubWorkers(4, 1000000, stub)
	opts := &BenchOptions{Workers: 4, Max: true, Duration: time.Hour, Amount: 1, Timeout: time.Second}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	report := runBench(ctx, opts, workers)
	cancel()

	// The report still has to come out: a measurement that is thrown away when
	// interrupted is one nobody dares interrupt.
	if report.Offered == 0 {
		t.Error("an interrupted run reported nothing offered")
	}
	if report.Elapsed <= 0 {
		t.Error("an interrupted run reported no window")
	}
	// Publishes that were in flight when the window closed are our own shutdown,
	// not the node refusing work, and must be named as such.
	for kind := range report.Errors {
		if kind != benchShutdownFailure {
			t.Errorf("an interrupted run blamed the node with %q (%v)", kind, report.Errors)
		}
	}
}
