# Examples

Ready-to-run scripts that exercise a Grape peer's REST API. They are
intentionally short so they double as documentation: every one is a
single `curl` (or a small loop of them) you can paste into a shell
and tweak.

## Prerequisites

A reachable Grape peer with the REST API enabled. The simplest way is
the local stack:

```sh
cp deploy/.env.example deploy/.env   # set GRAPE_REST_API_PASSWORD
make compose-up
```

That starts a peer on `https://localhost:8010` along with the
smart-contract VM and MongoDB.

## Configuration

All scripts read two environment variables:

| Variable        | Required | Default                          | Notes                                                  |
| --------------- | -------- | -------------------------------- | ------------------------------------------------------ |
| `GRAPE_API_URL`  | no       | `https://localhost:8010` (or `http://...` for `publish_nft.sh`) | Base URL of the peer's REST API. |
| `GRAPE_API_AUTH` | yes      | none                             | Base64 of `user:password` for HTTP Basic auth.         |

Build `GRAPE_API_AUTH` once:

```sh
export GRAPE_API_URL=https://localhost:8010
export GRAPE_API_AUTH=$(printf 'grape:changeme' | base64)
```

If `GRAPE_API_AUTH` is unset the scripts fail fast with a clear
message rather than emitting an unsigned header.

## Scripts

| Script                | What it does                                                                        |
| --------------------- | ----------------------------------------------------------------------------------- |
| `payment.sh`          | Submit a single signed payment transaction.                                         |
| `zero-payment.sh`     | Submit five zero-fee payments in a loop. Useful as a sanity check after `compose-up`. |
| `publish_nft.sh`      | Deploy an ERC721-style NFT contract and call a few of its methods.                  |
| `publish_factory.sh`  | Deploy a factory contract and exercise the deployed children.                       |
| `spammer.sh`          | Configure and launch the `txgen` load generator. Reads `WAITING_TIMEOUT`, `NODE_IP`, `TX_RATE`, `TX_MAX`. |
| `start-peer.sh`       | Reference orchestration script the systemd unit / Docker entry point can wrap. Substantial; treat as a template. |

## Running

```sh
# basic
./examples/payment.sh

# point at a remote testnet
GRAPE_API_URL=https://testnet1.example.com ./examples/zero-payment.sh

# load test against a local peer
WAITING_TIMEOUT=10 ./examples/spammer.sh
```

## Adding new examples

Keep new scripts:

- short — under ~100 lines
- self-contained — one feature per file
- env-driven — never hard-code endpoints, credentials, or addresses

If a script needs more than a few `curl` calls, consider promoting it
to a Go test under the relevant package instead.
