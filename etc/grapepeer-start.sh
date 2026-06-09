#!/bin/bash
# Sample wrapper script the systemd unit invokes. Adjust to match your
# deployment (binary path, peer id, bootstrap flags, environment file).
set -euo pipefail

# Optional environment overrides:
#   GRAPEPEER_BIN     path to the grapepeer binary
#   GRAPEPEER_ID      peer identity name (e.g. peer1, leader1)
#   GRAPEPEER_FLAGS   additional flags (e.g. "-bootstrap" or
#                    "-bootstrap_nodes /ip4/.../tcp/.../p2p/...")
GRAPEPEER_BIN="${GRAPEPEER_BIN:-/usr/local/bin/grapepeer}"
GRAPEPEER_ID="${GRAPEPEER_ID:-peer1}"
GRAPEPEER_FLAGS="${GRAPEPEER_FLAGS:-}"

# Increase the file-descriptor limit for libp2p / TCP connections.
ulimit -n 65536

exec "$GRAPEPEER_BIN" -id "$GRAPEPEER_ID" $GRAPEPEER_FLAGS
