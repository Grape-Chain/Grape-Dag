# Grape testnet web wallet

A small, self-contained wallet served by the peer itself at `/wallet/`. It
creates and imports accounts, shows the balance and transaction history, sends
payments, requests testnet funds from the faucet, and reports what the node it
is talking to is doing.

## Why the signer is WebAssembly

A Grape payment on the wire is a protobuf `Txv1` whose signature covers the
SHA-256 of the marshalled message with the signature field blanked. Any second
implementation of that — in JavaScript, say — has to stay byte-identical to the
Go one forever, and the failure mode when it drifts is silent rejection (or
worse, an accepted transaction that means something slightly different).

So the wallet does not reimplement it. `cmd/walletwasm` compiles the node's own
`crypto`, `tx` and `wallet` packages to WebAssembly, and the browser calls into
that. The transaction bytes are produced by the same code that validates them,
and the private key never leaves the page.

`tx/test/walletwasm_test.go` locks this down: it holds a payment produced by
this wallet and asserts the node parses and verifies it.

## Build

```sh
make wallet
```

That writes `wallet.wasm` (~25 MB, gzipped to ~5 MB alongside it) and copies
`wasm_exec.js` out of the Go distribution. Both are gitignored — they are build
outputs. The peer serves the `.gz` to any client that accepts gzip.

The route is registered only when `index.html` is present, so a node without
built assets simply does not expose `/wallet/`; it logs a line saying so. The
directory is configurable with `peer.walletdir`.

## Run it

```sh
make wallet
make compose-up          # or run a peer directly
```

then open `https://localhost:8010/wallet/`.

The REST API is behind HTTP basic auth. Because the wallet is served from the
node's own origin, authenticating once to load the page is enough — the browser
sends the same credentials on the wallet's API calls. For a local development
node you can instead set `peer.apiauthdisabled: true`, which turns the auth off
entirely and logs a warning.

## Files

| File | What it is |
|---|---|
| `index.html` | the whole UI: markup and styles, no build step, light and dark |
| `app.js` | flow, API calls, encrypted key storage in `localStorage` |
| `units.js` | value parsing and formatting, isolated so it can be unit-tested |
| `units.test.mjs` | `node web/wallet/units.test.mjs` |
| `wasm_exec.js` | Go's WebAssembly loader (copied by `make wallet`) |
| `wallet.wasm` | the signer (built by `make wallet`) |

`scripts/wallet_e2e.mjs` drives the real `wallet.wasm` and a stub node through
the whole flow — create, faucet, balance, sign, submit, history — and is the
fastest way to check the wallet after a change:

```sh
node scripts/wallet_e2e.mjs
```

## Keys, and what this wallet is not

The private key is stored in `localStorage`, encrypted with AES-GCM under a
PBKDF2-SHA-256 key (250 000 iterations) derived from a passphrase. It is
decrypted into memory on unlock and dropped on lock.

That is appropriate for a testnet and no more. Browser storage is reachable by
anything that can run script on the origin, there is no seed phrase or recovery
path, and clearing site data destroys the key. The UI says as much. Do not put
value in it.

## Amounts

Balances and amounts are integers in the smallest unit (10^-18 GRAPE) and are
handled as `BigInt` end to end — no floats anywhere in the value path.

One wrinkle worth knowing: the REST API is not uniform about radix. Account
balances arrive as decimal strings while transaction amounts arrive `0x`-hex
encoded, so `units.js` parses both. That is covered by the unit tests.
