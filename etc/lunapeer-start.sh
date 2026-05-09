#!/bin/bash
# Sample wrapper script the systemd unit invokes. Adjust to match your
# deployment (binary path, peer id, bootstrap flags, environment file).
set -euo pipefail

# Optional environment overrides:
#   LUNAPEER_BIN     path to the lunapeer binary
#   LUNAPEER_ID      peer identity name (e.g. peer1, leader1)
#   LUNAPEER_FLAGS   additional flags (e.g. "-bootstrap" or
#                    "-bootstrap_nodes /ip4/.../tcp/.../p2p/...")
LUNAPEER_BIN="${LUNAPEER_BIN:-/usr/local/bin/lunapeer}"
LUNAPEER_ID="${LUNAPEER_ID:-peer1}"
LUNAPEER_FLAGS="${LUNAPEER_FLAGS:-}"

# Increase the file-descriptor limit for libp2p / TCP connections.
ulimit -n 65536

exec "$LUNAPEER_BIN" -id "$LUNAPEER_ID" $LUNAPEER_FLAGS
