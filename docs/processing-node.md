# Running a machine as a processing node

A wallet user should be able to turn the machine the wallet is installed on into
a network processing node, and turn it back off again, without learning anything
about the node's configuration. This describes what that takes: the provisioning
a wallet application drives, the endpoints it then talks to, and what remains to
be wired up.

## What "stop processing" means

The model is NXT forging. Processing is the encapsulation of transactions into
sites and the collection of the fees on them. Starting and stopping processing
starts and stops *that*, and nothing else.

A node with processing stopped:

- keeps syncing, and its pin height keeps rising;
- keeps validating what arrives and keeps applying commit transactions;
- keeps gossiping, so it is still a useful member of the network;
- does not encapsulate transactions into sites, and so earns nothing.

This is why the switch is one `atomic.Bool` read in the publish loop rather than
anything that stops a service. There is no configuration that turns processing
off: a node is provisioned as a processing node, and whether it is processing at
any given moment is a runtime decision that survives nothing but the process.

## Provisioning: what a wallet application does

```
grapepeer join [--wallet-file PATH] [--network N] [--bootstrap-nodes LIST]
               [--data-dir PATH] [--api-port N] [--force]
```

| flag | default | meaning |
| --- | --- | --- |
| `--wallet-file` | `<data-dir>/wallet.json` | the account this node runs and earns as. Generated and written here when the file does not exist. |
| `--network` | `2` | `0` MAINNET, `1` PUBLIC_TESTNET, `2` PRIVATE_TESTNET. |
| `--bootstrap-nodes` | none | comma separated multiaddrs. Each is parsed before anything is written. |
| `--data-dir` | `$HOME/.grap3` | where the configuration, bootstrap list, credentials and ledger go. |
| `--api-port` | `33330` | port for the REST API, the node endpoints and the bundled page. |
| `--force` | off | replace an existing configuration. Never replaces the wallet file or the API credentials. |

`join` writes, in the data directory:

- `wallet.json` — the account's keys. **The only place a private key belongs.**
  An existing file is loaded and never rewritten, `--force` included: the key in
  it is the only copy of an account.
- `grapepeer.yml` — the node's configuration, rendered from a template embedded
  in the binary. `peer.nodetype: 0`, the wallet's keys under `dag:`, and
  `dag.coinbaseaccount` set to the same account as `dag.wallet`, which is what
  makes the earnings endpoint mean anything. The name is not negotiable:
  `config.LoadGrapepeer` looks for `grapepeer.yml` and nothing else.
- `bootstrap.json` — written only when `--bootstrap-nodes` is given, in the shape
  `config.LoadBootstrap` reads (an object whose values are multiaddrs).
- `api-credentials.env` — a generated `GRAPE_REST_API_USERNAME` and a 32-byte
  random `GRAPE_REST_API_PASSWORD`, the variables `config/param.go` reads. The
  node's own controls are behind these, so `peer.apiauthdisabled` is written
  `false` and existing credentials are kept even under `--force` — rotating them
  would lock out whatever wallet application is already connected.

Everything is mode `0600` in a `0700` directory. Nothing secret is printed: the
summary names the wallet address and the paths, never the private key and never
the password.

`join` does not start the node. It prints the exact command that does, including
the `-f` when the configuration is not in the single place the loader looks by
itself, and the peer id — derived from the wallet address, so that a node comes
back as the same peer after a restart.

```
set -a; . /home/u/.grap3/api-credentials.env; set +a
grapepeer -id node-1a6c7792
```

Provisioning and running are separate commands on purpose. A wallet application
wants to write the files, show the operator what was written, and then run the
node as a supervised child process it can restart; one command that did both
would give it no point at which to do either.

`grapepeer status` asks a running node what it is doing, reading the credentials
from the environment and falling back to the file `join` wrote.

## The endpoints

Plain `net/http` handlers in `services/node`, deliberately not on the generated
OpenAPI server: they are the node's own control surface rather than part of the
ledger API, and they should be mountable without regenerating anything.

### `GET /node/status`

```json
{
  "state": "processing",
  "wallet": "0x1a6c77929698e36981b9b0e0486a253ae33185e6",
  "pinHeight": 4231,
  "peers": 7,
  "tpsContribution": 12.40
}
```

`state` is one of four:

| state | meaning |
| --- | --- |
| `stopped` | the gate is off. Still syncing, still validating, not earning. |
| `syncing` | the gate is on but the node has not caught up. It will begin on its own. |
| `ready` | the gate is on and the node has caught up, but it has no peers, so a site built here would reach nobody. |
| `processing` | the gate is on, the node has caught up, and it has peers. |

The gate is read first, and the ordering is the point: when somebody stops
processing, the answer to "what is my node doing" has to be `stopped`, not a
report on a part of the node they did not ask about. That a stopped node is still
syncing is shown by `pinHeight` in the same response, which keeps rising.

`tpsContribution` is what *this* node encapsulated, averaged over a trailing ten
seconds — not the network's throughput.

### `GET /node/earnings`

```json
{
  "wallet": "0x1a6c...",
  "lifetime": "1180591620717411303424",
  "pending": "42",
  "recent": [
    {"pin": 991, "site": "3f0c...", "amount": "1000", "at": "2026-08-28T10:30:00Z"}
  ]
}
```

Every amount is a decimal string. A JSON number is a `float64` in every browser,
so a fee total large enough to matter would arrive rounded, and a wallet must not
display a rounded balance. `recent` is always an array, never `null`. `lifetime`
is what a commit transaction has settled; `pending` is earned but not yet
settled.

### `POST /node/processing`

```json
{"enabled": false}
```

Answers with the whole status object, so a UI can move its toggle from the same
round trip that flipped it. A body with no `enabled` field is a `400`: the field
is a pointer in the request struct precisely so that `{}` cannot be read as
"stop".

### `GET /node/`

The bundled reference page, `web/runnode/index.html`, embedded in the binary. One
file, no build step, no framework, and no external reference of any kind — served
with `Content-Security-Policy: default-src 'none'; connect-src 'self'`, so the
browser enforces that the page talks to the node that served it and to nothing
else. It is a reference implementation for driving the endpoints, not a product
skin; a wallet application is expected to build its own.

Behind the REST API's basic auth like every other route, so a browser will ask
for the credentials in `api-credentials.env` the first time.

## What is still to be wired

`services/node` compiles and is tested against `StubLedger`, which reports a node
that knows nothing and says `Syncing() == true` rather than optimistically
claiming to have caught up. Four changes turn it into a live surface.

### 1. The processing gate, in the publish loop

One line, as the first statement of the `for` body in `publish`,
`diffusion/publish.go` (insert after line 29, `for {`):

```go
if !node.ProcessingEnabled() { time.Sleep(100 * time.Millisecond); continue }
```

plus `"github.com/Grape-Chain/Grape-Dag/services/node"` in that file's imports.
`time` is already imported.

Two things about the placement. It goes **before** `txqueue.GetPublishQueue().Dequeue()`
so that transactions submitted to this node wait in the queue rather than being
dequeued and dropped. And it sleeps rather than calling `runtime.Gosched()` as
the empty-queue path below it does, because a stopped node would otherwise spin
a core for as long as it is stopped.

The gate starts enabled (`services/node/processing.go`, `enabledGate`), so adding
that line changes nothing about how any existing node behaves until somebody
stops it.

Optionally, one more line to make `tpsContribution` report anything, in the
success branch after `stats.Enqueue(...)` (`diffusion/publish.go` line 114):

```go
node.RecordProcessed()
```

### 2. A real `Ledger`

```go
type Ledger interface {
	PinHeight() int64
	PeerCount() int
	Syncing() bool
	WalletAddress() string
	EarningsFor(wallet string) (lifetime, pending *big.Int, recent []Credit, err error)
}
```

It cannot live in `services/node`, which is kept free of the ledger on purpose.
It belongs in package `services` — a new `services/nodeledger.go` — which already
imports `app`, `dag` and `config`, and which `dag` does not import back.

| method | implementation |
| --- | --- |
| `PinHeight()` | `int64(dag.GetPin().CurrentHeight())` — `dag.GetPin` at `dag/dag.go:586`, `(*NodeTxPin).CurrentHeight` at `dag/pin.go:695`. |
| `PeerCount()` | `len(grapepeer.GetHost().Network().Peers())` — `peer.GetHost` at `peer/peer.go:95`. Guard the nil host: the endpoint can be hit before `grapepeer.NewHost` has run. |
| `Syncing()` | true when `app.GetApp().App_dagsyncmgr` is nil, or when either of its `HaveJoined` and `SitesProcessed` flags is still false. Both are `atomic.Bool` fields on `DagSyncMngr`, `dag/syncmngr.go:28-29`, set at `dag/syncmngr.go:465` and `dag/syncmngr.go:380` respectively. |
| `WalletAddress()` | `config.GetConfig().Dag.Wallet`, which `grapepeer join` writes and keeps equal to `dag.coinbaseaccount`. `dag.GetDag().Wallet().WalletAddress()` (`dag/dag.go:190`) is the same account but generates a throwaway wallet when none is configured, so prefer the config. |
| `EarningsFor()` | Nothing to read yet — see below. |

### 3. Fees, which do not exist yet

Nothing in the tree credits a fee to anybody. `dag/confirmation.go:61`,
`dag/confirmtracker.go:91` and `dag/recovery.go:144` all say "once fees land",
and the coinbase account is currently only passed to the VM and then ignored on
the way back.

The place a credit becomes recordable is `(*NodeTxPin).executeSMCTx`,
`dag/pin.go:902`: it already builds the `pb.PinTxHeader` with
`CoinbaseAccountAddress` from `config.GetConfig().Dag.Coinbaseaccount`
(`dag/pin.go:910-915`) and already gets `GasUsed` back from the VM. A fee credit
is that gas at the transaction's fuel price, attributed to the coinbase account,
against the pin number `executeSMCTx` was given.

Its callers are where the credits accumulate:
`(*NodeTxPin).runSmartContractStage` (`dag/pin.go:564`), reached from
`(*NodeTxPin).unsafe_buildPin` (`dag/pin.go:469`). Credits recorded there are
`pending` until the commit transaction they belong to is applied, and `lifetime`
after — which is the distinction the earnings endpoint reports and the reason it
has two totals rather than one.

Until then, `EarningsFor` should return zeros and an empty slice rather than an
invented figure. `services/node` exports `ErrNotWired` for an implementation that
would rather say so.

### 4. Mounting the routes

In `(*RestAPIConfig).routes`, `services/rest/init.go`, beside the existing
`mux.HandleFunc("/faucet", ...)` lines (around line 140) so that the endpoints
inherit the basic-auth and recovery middleware already on that mux:

```go
nodeSvc := node.NewService(services.NewNodeLedger())
mux.Handle("/node", http.RedirectHandler("/node/", http.StatusMovedPermanently))
mux.Handle("/node/*", nodeSvc.Routes())
```

`Service.Routes` registers absolute `/node/...` patterns and chi passes the
request path through unchanged, so no `StripPrefix` is wanted; the paths the
handler serves are the paths the bundled page fetches.

Two smaller things worth deciding at the same time:

- **The initial gate position.** It is enabled at start-up. If a node should come
  back stopped after a restart, that has to be persisted somewhere and
  `node.SetProcessing` called from the start-up path; nothing does that now, and
  nothing should until somebody decides where the state lives.
- **`peer.walletdir`.** The bundled page is embedded, unlike the web wallet, which
  is served from disk because `wallet.wasm` is ~25 MB. Nothing about the node page
  needs a directory, and it has no configuration knob for that reason.

## Testing

```
go test -race ./services/node/ ./cmd/grapepeer/
```

`services/node` is tested against a fake `Ledger`, so none of it needs a running
node: the state machine over all four states, the gate under concurrent flips and
reads, the earnings JSON shape, and the bundled page's lack of any external
reference. `cmd/grapepeer` provisions into a temporary directory and reads the
result back with viper — the same library the node's own loader uses, so a
generated file that viper cannot parse fails there rather than at start-up.
