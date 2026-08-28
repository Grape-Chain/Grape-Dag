# Benchmarking a node: what `-mode bench` measures, and why the old generator could not

Trader mode topped out at roughly 580 transactions per second while the node
under test sat at 42% CPU. Nothing in that number described the node. Every one
of the four limits below belonged to the generator, and each had to be removed
before the node's own ceiling became visible.

`-mode bench` is a separate mode for that job. The existing modes — `trader`,
`genesis`, `local`, `balance`, `payment`, `wallet`, `count`, `check`, `watchdog` —
are unchanged apart from the pacing arithmetic described at the end.

## What was limiting the generator

**One connection.** gRPC multiplexes every call made on a `ClientConn` onto a
single HTTP/2 connection. Adding senders behind one client adds streams, not
throughput: they queue behind that connection's stream limit and flow control
window. Bench mode dials one connection per worker.

**Two round trips per transaction.** `Trade1` called `GetBalances` and then
`PublishTx`, in series, for every transaction. Two serial round trips is a hard
ceiling of a few hundred per second no matter how many senders there are, and the
first of the two asks the node for a number the generator could keep itself.
Bench mode tracks balances locally and makes exactly one round trip per
transaction.

**A sleep every thousand transactions.** The main loop slept a full second on
every thousandth transaction. That alone caps the average rate, and it also
distorted the figure the run printed, because the loop subtracted those seconds
from the elapsed time it divided by.

**Pacing arithmetic that could not express the configured rate.** `Txrate` is
documented as transactions per second, but all three pacing functions in
`tools/txgen/misc.go` computed 60000/rate — every configured rate came out 60
times slower than asked for. `TxuniformTime` was worse than slow: it derived an
integer bound for `rand.Int31n` from the interval, and that bound is 0 for any
rate above 120, where `rand.Int31n` panics. So every rate a load test would
actually use crashed the generator on its first tick. Those functions are fixed
(see the last section); bench mode does not use them at all, because a sleep
between transactions cannot pace above about a thousand per second whatever the
arithmetic says.

## How bench mode works

### Parallel senders, one connection each

`-bench_workers` goroutines, default 32, each holding its own `grpc.ClientConn`
to the node and its own slice of pre-signed transactions. Nothing is shared
between workers except the token bucket and the counters, so the send path has no
lock contention worth measuring. Connections are dialled with the `passthrough`
resolver and each is waited to `Ready` before the window opens, so a bad address
fails as a setup error rather than as a hundred thousand failed publishes.

### Pacing: a shared token bucket, drawn in batches

Above about a thousand per second the gap between transactions falls to a
millisecond or less, which is at or below the resolution of a Go timer. A loop
that sleeps per transaction therefore settles at whatever the runtime delivers,
not at the configured rate — the reason a rate of 2000 could never have worked
even with the arithmetic corrected.

The bucket in `tools/txgen/bench_pacing.go` avoids timers on the hot path:

- It holds permits and refills continuously from the wall clock at the configured
  rate. There is no ticker.
- A worker draws a **batch** of permits and sends that many transactions back to
  back with **no sleep at all**.
- A worker sleeps only when the bucket is empty, and then for exactly the time it
  takes to earn one permit. The number of sleeps per second is bounded by the
  number of workers, not by the rate.
- Capacity is a tenth of a second's worth of permits. That is enough that a
  worker blocked on one slow round trip does not throw away the permits it earned
  while waiting, and small enough that the catch-up afterwards is not a spike the
  node sees as a different workload. Permits earned past capacity are dropped, so
  an idle generator cannot open with a stored-up burst.
- The bucket starts **empty**. A full one would let the first instant of the run
  offer a burst on top of the configured rate, and a short run reports that
  overshoot as its offered rate.
- **A rate of zero means unpaced.** `take` returns immediately without touching
  the clock or the lock. This is what `-bench_max` sets, and it is the setting
  that finds the node's ceiling.

The batch is capacity divided by the worker count, so no single worker can drain
the bucket and starve the others. At 10000/s with 32 workers that is a bucket of
1000 permits and 31 per draw.

### Nothing expensive inside the timed window

Before the clock starts:

1. A pool of sender wallets is created — `-bench_wallets`, or one per worker,
   whichever is larger. Every worker owns a disjoint set, dealt out in strides,
   because two workers spending one wallet would race on its locally tracked
   balance.
2. The faucet (the genesis wallet from `generator.publickey` and
   `generator.privatekey`) funds them **one payment at a time**. Sequential on
   purpose: a faucet publishing concurrently with itself hands the node several
   payments drawn from one balance at once.
3. The run waits until the node reports that funding, polling `GetBalances`, and
   seeds each local balance from what the node actually says. Without this the
   window would open while the faucet payments were still queued and every sender
   would be spending coin the node does not think it has. `-bench_settle 0` skips
   the wait and the check.
4. Every transaction is generated and signed, in parallel across workers. ed25519
   over a SHA-256 of the marshalled transaction is tens of microseconds; leaving
   it in the send loop charges the node's score for the generator's own CPU.
   Each transaction gets a nonce of its own rather than the random one
   `GeneratePayment` picks, so no two can hash alike — duplicates would be counted
   as offered here and then deduplicated by the node, which reads as throughput
   the node never had.

Inside the window a worker does one thing: take a permit, take a pre-signed
transaction, `PublishTx`, record the latency. No signing, no balance query, no
sleep.

The cost of that is memory: roughly 0.6 KB per pre-signed transaction, so the
200000 of an unpaced run is a few hundred megabytes held by the generator. Raise
`-bench_prepare` for a longer saturation run and watch the generator's own
resident size when you do. When a worker's pool runs out it stops, and the report
shows the shorter window rather than starting to sign inside the measurement.

### What the report says

A progress line every `-bench_report` (default 5s), rated over the interval since
the previous line rather than since the start, so a change in the node's
behaviour appears in the line where it happened:

```
[bench] t=  5.0s offered    24310 (   4862.0/s) accepted    24310 (   4862.0/s) failed      0 ratio 1.000 p50    412µs p95   1.1ms p99   3.4ms
```

and a summary at the end:

```
--- bench report ------------------------------------------------
  window            : 30.00 sec
  offered           : 145830 tx (4861.0 tx/s)
  accepted          : 145702 tx (4856.7 tx/s)
  failed            : 128 tx
  accepted/offered  : 0.9991
  generator stalls  : 0 (0.00% of offered)
  publish latency   : p50 412µs  p95 1.1ms  p99 3.4ms  max 91.2ms  mean 615µs
  errors            :
      grpc/resourceexhausted       128
-----------------------------------------------------------------
```

- **offered** — transactions handed to the node, whether or not it took them.
- **accepted** — publishes the node acknowledged. Read this as the throughput.
- **accepted/offered** — the share the node took. A ratio that falls away from 1
  while the offered rate keeps climbing is the node refusing load, which is the
  signal that the ceiling has been reached.
- **latency percentiles** — recorded in exponentially spaced buckets, so a
  reported percentile is the upper bound of the bucket the true value falls in:
  it overstates by at most 15% and never understates. Accepted publishes only;
  failures appear in the error breakdown instead.
- **generator stalls** — times a sender found its buffer empty and had to wait
  for its own signer. Read this line before any of the others. While a sender is
  waiting, the rate being measured is the generator's signing rate and not the
  node's capacity, so past 1% of offered the report says so outright and the
  throughput figure is not the node's ceiling. Zero is the expected reading.
- **errors** — grouped by cause, `grpc/<code>` for a status error, ordered with
  the dominant failure first. Expect one `bench/shutdown` per worker at the end
  of every run: that is the generator cancelling its own in-flight calls, not the
  node refusing work.

**Check `window` against what you asked for.** If the window is shorter than
`-bench_duration`, the run ended early and every rate in the report is an
average over a window that includes whatever the node was doing after the load
stopped. See below.

### The run that ended early, and how to tell

Worth knowing about because it produced a number that looked completely
plausible and was wrong.

Bench mode used to sign its entire offered load before the window opened, which
meant the size of that pool decided how long a run could last. A `-bench_max`
run asking for five minutes spent its 200,000 transactions in 65 seconds and
then went quiet:

```
[bench] offering as fast as the node accepts, for 5m0s
  window            : 64.79 sec
  offered           : 200016 tx (3087.3 tx/s)
  accepted/offered  : 1.0000
```

Nothing in the report was untrue — it stated the real 64.79-second window. But
65 seconds is not long enough for the node's queues to reach a steady state, so
3,087/s was a transient, not a sustained rate; and anything reading `/metrics`
afterwards was sampling an idle node and averaging the silence in.

Each worker now keeps a bounded reserve of signed transactions with a signer
goroutine behind it, so run length no longer depends on how much the generator
can hold: the reserve is 2,000 per worker whatever the window, about 60 MB
across a 48-worker run. The same request now yields the window it asked for:

```
  window            : 300.03 sec
  offered           : 739785 tx (2465.7 tx/s)
  accepted/offered  : 0.9999
  generator stalls  : 0 (0.00% of offered)
```

Signing inside the window costs the generator CPU the node would otherwise have
had. That cost is measured rather than assumed away, which is what the stall
line is for.

SIGINT cancels the window and still prints the report. A measurement that throws
its numbers away when interrupted is one nobody dares interrupt, so bad runs get
left to finish instead.

## What one node can actually do, and what stops it

Measured on the reference harness below, on a **4 vCPU Intel Xeon @ 2.80GHz,
16 GiB** box — *not* the 8-vCPU class the throughput target is written against,
so read these as a floor rather than as the target being met or missed.

Two nodes, leader plus one peer, both on the same four cores; 48 generator
workers offering as fast as the node accepts, for five minutes.

```
t= 55s  ingress=3415/s  inserted=2853/s  confirmed=2782/s  live=596
t=110s  ingress=2715/s  inserted=2715/s  confirmed=2717/s  live=507
t=165s  ingress=2728/s  inserted=2728/s  confirmed=2723/s  live=700
t=220s  ingress=2645/s  inserted=2645/s  confirmed=2664/s  live=362
t=275s  ingress=2631/s  inserted=2631/s  confirmed=2615/s  live=322
```

**About 2,700 transactions a second, sustained.** The three counters converge
after the first window, which is the reading that matters: ingress is not
running ahead of the graph, so this is throughput and not buffering. The first
window is higher because the publish queue is still filling. `live` stays in the
hundreds throughout, so slicing and confirmation hold their working set. The
generator reported **0.00% stalls**, so the number is the node's and not the
generator's.

### What is not the limit

- **Not CPU.** A 45-second profile under load collected 56.76s of samples, which
  is **1.26 cores out of four**. The node cannot use the other 2.7.
- **Not lock contention.** `AddTxDag` was 63.54% of 9.12s of mutex delay before
  this work; it is now 1.97% of 24.5s. `GetConfirmedSites` is 4.35%. The largest
  entry left is `runtime._LostContendedRuntimeLock` at 72.84%, which is the Go
  runtime's own locks, not the ledger's.
- **Not the generator.** Zero stalls, and `accepted/offered` at 0.9994.

### What is the limit

`grape_queue_depth{queue="publish"}` sits at **32,810** — its 32,768 ceiling —
for the whole run, every run. The publish queue is full, `Enqueue` blocks rather
than drops, and that backpressure is what makes ingress equal insertion.

Behind that queue is **one goroutine**. `publish()` dequeues a transaction,
builds a site, signs it, marshals it and hands it to gossipsub, one at a time.
At 2,700 a second that is about 370 microseconds per transaction on a single
goroutine, against an insert that A/B benchmarking puts at 65. The rest is
signing and serialisation, and about 30% of all CPU the node uses is ed25519
field arithmetic (`feMul` 12.90%, `feSquare` 9.90%, plus carries and selects).

So the ceiling is a serialisation ceiling, not a contention or capacity one, and
the next multiple comes from fanning that goroutine out the way the subscriber's
verification was fanned out — which needs `AddTxDag` to be safe for concurrent
callers first.

### Component measurements behind it

Each measured A/B on the same box, same substrate both sides:

| | before | after | |
|---|---|---|---|
| insert, 4 goroutines | 96.8 µs/op | 65.5 µs/op | 1.5× |
| insert lock contention delay | 0.253 ms | 0.084 ms | 3× |
| received-insert exclusive section | 75,718 ns | 4,022 ns | **19×** |
| subscriber verification | 12,657 msg/s | 23,148 msg/s | 1.83× |
| commit build, 5000 sites | 45.6 ms | 22.7 ms | 2× |
| tip selection, 5000 sites/fanout 64 | 229 µs | 19.6 µs | **11.7×** |
| allocations per insert round, 5000/64 | 118,794 | 2,018 | **59×** |
| chain readers completing per commit | 274–298 | 1364–1575 | 5× |
| resident memory over a 5-minute run | 5.2 GB | 3.6 GB | |
| `go test -race ./dag/` | 275.7 s | 16.9 s | |

### Reproducing it

The harness deliberately records the box's load average beside every sample.
A rate taken while the machine is three times oversubscribed is a rate for a
machine that is three times oversubscribed, and without that number written
down beside it there is no telling that reading from a real regression later.
Every figure above was taken with nothing else running; several earlier ones
were not, and had to be thrown away.

## Running it

The node's gRPC port is the only thing that has to be right. To find the maximum
sustained throughput of a node listening on 50333:

```
go build -o bin/ ./cmd/txgen
./bin/txgen -mode bench -grpc_port 50333 -bench_max -bench_workers 32 -bench_duration 60s -bench_report 5s
```

or through the Makefile, which builds first:

```
make bench-node-max TXGEN_PORT=50333 TXGEN_ARGS="-bench_workers 32 -bench_duration 60s"
```

A fixed offered rate, to check the node holds a target without queueing:

```
./bin/txgen -mode bench -grpc_port 50333 -bench_rate 5000 -bench_duration 60s
# or: make bench-node-rate RATE=5000 TXGEN_PORT=50333
```

`generator.nodeip`, the faucet keys and the network id come from
`~/.grap3/txgenerator.yml` as they do for the other modes. `-grpc_port`
overrides `generator.nodeport`. **Bench mode ignores `generator.txrate`** — a
saturation run must not silently inherit a paced config, so the rate comes from
`-bench_rate` or `-bench_max` and nowhere else.

### Flags

| flag | default | meaning |
| --- | --- | --- |
| `-bench_workers` | 32 | parallel senders, each with its own gRPC connection |
| `-bench_rate` | 0 | offered transactions per second across all workers; 0 is unpaced |
| `-bench_max` | false | unpaced, ignoring `-bench_rate` |
| `-bench_duration` | 30s | length of the timed window |
| `-bench_txmax` | 0 | stop after this many transactions; 0 runs for the duration |
| `-bench_wallets` | 0 | faucet-funded sender wallets; 0 means one per worker |
| `-bench_prepare` | 0 | transactions to pre-sign; 0 derives it from the rate and duration, or 200000 when unpaced |
| `-bench_amount` | 1 | amount each pre-signed payment moves |
| `-bench_fund` | 0 | faucet amount per sender wallet; 0 derives it from the pool size and the amount |
| `-bench_timeout` | 5s | deadline for one `PublishTx` call |
| `-bench_report` | 5s | progress interval; 0 reports only at the end |
| `-bench_settle` | 30s | how long to wait for faucet funding to appear in balances; 0 does not wait or check |

### Finding the ceiling

Unpaced with the default 32 workers, then vary one thing at a time:

1. **`-bench_max` first.** If the accepted rate is high and the ratio stays at
   1.000, the node took everything offered and the ceiling is above what the
   generator produced — add workers.
2. **Add workers while the accepted rate rises.** More workers means more
   in-flight round trips. When the accepted rate stops rising and only the
   latency percentiles grow, the workers are queueing inside the node and that
   accepted rate is the ceiling.
3. **Confirm with a paced run.** Set `-bench_rate` to just under the ceiling for
   a few minutes. A sustained run should hold ratio 1.000 with a stable p99. A
   p99 that climbs through the run is a backlog growing, not a rate the node can
   sustain.
4. **Watch the generator too.** If the generator's own CPU is saturated the
   number is about the generator again. Pre-signing takes it out of the window,
   but marshalling and the gRPC stack do not.

## What "accepted" does and does not mean

`RoboTraderServer.PublishTx` checks that the request carries a transaction with
a twenty-byte sender, then puts it on the publish queue and returns `Status: 0`.
The signature is checked by the diffusion subscriber and the balance by the DAG,
both after that return. So **accepted means the node took the transaction in,
not that it reached the DAG.** Three consequences worth keeping in mind:

- The failures that do appear are transport-level: the publish queue's notify
  channel holds 32768 entries, and once the node's consumer falls that far behind,
  `Enqueue` blocks. `PublishTx` then stops returning until the queue drains, which
  shows up here first as a rising p99 and then as `-bench_timeout` expiring. That
  is genuine backpressure and it is the ceiling this tool is looking for.
- Ingest is not commitment. After a run, compare the DAG against what was
  accepted:

  ```
  ./bin/txgen -mode count -grpc_port 50333
  ```

  A DAG that grew by far less than the accepted count means the node took work it
  could not process, and the sustainable rate is the smaller of the two numbers.
- The node's own view is on `/metrics`, and it is the one to trust for a
  sustained figure: `grape_site_insert_seconds_count` and
  `grape_sites_confirmed_total` are sites that actually entered the graph and
  actually confirmed. At steady state all four numbers converge, because the
  publish queue blocks rather than drops — a 300-second run measured 2,465 tx/s
  offered, 2,465 accepted, and inserted and confirmed both within 1% of that,
  with the live graph holding steady under 300 sites.

  Those node-side counters were not always trustworthy either.
  `grape_tx_accepted_total` read **zero** through a run that offered 739,785
  transactions, because `TxIngress`, `TxAccepted` and `TxRejected` were wired
  only into the REST entry point and never into the gRPC handler that every
  benchmark uses. They are wired now. A metric that stays at zero under load is
  worse than a missing one, because it reads as an answer.

## The pacing fix in `misc.go`

`Txrate` now means transactions per second in all three pacing functions, as the
field was always documented:

- `TxdefaultTime(rate)` returns `1000/rate` ms, floored at 1 ms.
- `TxuniformTime(rate)` jitters uniformly ±25% around that mean, using
  `rand.Float64` — the integer bound that panicked above 120/s is gone.
- `TxnormalTime(rate)` draws from a normal distribution around the same mean,
  σ = 12.5% of it, truncated at ±3σ so one sample from the tail cannot stall the
  generator for a multiple of the interval.

Every interval is clamped to [1ms, 60000ms], so no rate value — including 0 and
`MaxUint64` — can return a non-positive period to `time.NewTicker` or overflow
the conversion. A rate of 0 has no interval to compute and gets the 1 ms floor,
since the timer-driven modes have no way to express "unpaced"; bench mode has an
explicit unpaced path for that intent.

This changes the effective rate of the existing timer-driven modes: they now run
at the rate their config asks for, which is 60 times faster than before. A config
carried over from the old behaviour — `txrate: 60` meaning one per second — now
means sixty per second. Divide an old `txrate` by 60 to keep the old pace.

## Tests

`go test ./tools/txgen/` covers the pacing and the metrics without a node, and
the send path against a stub client:

- the rate arithmetic across 0, 1, 2, 60, 100, 120, 121, 1000, 10000 and 100000
- that no pacing function panics or returns a non-positive interval at any rate,
  including the range above 120/s that used to crash
- that the token bucket offers the configured rate over a second, never exceeds
  its capacity in a burst, and grants batches rather than one permit at a time
- that the transaction budget hands out exactly `-bench_txmax` across concurrent
  workers
- that latency percentiles bracket the samples they came from
- that pre-signed transactions verify, are all distinct, and stop when the funded
  balance is spent
- that a run stops on its budget, on a spent pool and on a cancelled context, and
  reports either way
