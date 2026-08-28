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
- **errors** — grouped by cause, `grpc/<code>` for a status error, ordered with
  the dominant failure first.

SIGINT cancels the window and still prints the report. A measurement that throws
its numbers away when interrupted is one nobody dares interrupt, so bad runs get
left to finish instead.

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

`RoboTraderServer.PublishTx` puts the transaction on the publish queue and
returns `Status: 0` without validating it. So **accepted means the node took the
transaction in, not that it reached the DAG.** Two consequences worth keeping in
mind:

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
