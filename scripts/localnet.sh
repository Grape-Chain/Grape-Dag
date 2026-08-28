#!/usr/bin/env bash
#
# localnet.sh - run a small local network and measure what it can do.
#
# Portable between macOS and Linux, and written for bash 3.2 because that is what
# macOS ships. This replaces a Linux-only scratch harness that used setsid,
# /proc/loadavg and free(1), none of which exist on macOS.
#
#   ./scripts/localnet.sh up                 # leader + one peer, fresh data
#   ./scripts/localnet.sh load 300s 48       # offer load as fast as it is taken
#   ./scripts/localnet.sh rate 55 5          # sample five 55-second windows
#   ./scripts/localnet.sh measure            # up, load and rate in one go
#   ./scripts/localnet.sh prof 45            # profiles, taken UNDER load
#   ./scripts/localnet.sh down
#
# Deliberately no `set -e`. Half of what this does is ask whether a process is
# running, and the tools for that exit non-zero when the answer is no, which is
# not an error - it is the answer.

set -u

R="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUN="$R/.localnet"
LOGS="$RUN/logs"
METRICS="http://127.0.0.1:6060/metrics"
PPROF="http://127.0.0.1:6060/debug/pprof"
GRPC_PORT=50333
# A fixed libp2p port for the leader, so the peer's bootstrap address is
# predictable. join writes port: 0, which lets the OS choose - right for a real
# node, useless for a script that has to hand the address to a second one.
LEADER_P2P_PORT=43431

mkdir -p "$LOGS" "$RUN/node0" "$RUN/node1"

# ---------------------------------------------------------------- portability

os="$(uname -s)"

# loadavg - the one-minute figure, on either platform.
#
# Recorded beside every throughput sample on purpose. A rate measured while the
# machine is three times oversubscribed is a rate for a machine that is three
# times oversubscribed, and without the number written down next to it there is
# no way to tell that reading from a real regression a week later. Two of this
# project's measurement runs had to be thrown away for exactly that reason.
loadavg() {
  case "$os" in
    Darwin) sysctl -n vm.loadavg | awk '{print $2}' ;;
    *)      awk '{print $1}' /proc/loadavg ;;
  esac
}

cores() {
  case "$os" in
    Darwin) sysctl -n hw.ncpu ;;
    *)      nproc ;;
  esac
}

# start_bg <logfile> <pidfile> <command...>
#
# A pidfile rather than pgrep. On both platforms `pgrep -f bin/grapepeer` also
# matches the shell that ran it, which is a false positive that cost real
# debugging time - it reported a node as running after it had exited.
start_bg() {
  local log="$1" pidf="$2"; shift 2
  nohup "$@" > "$log" 2>&1 < /dev/null &
  echo $! > "$pidf"
  disown 2>/dev/null || true
}

# start_node <n> <command...> - start a node with its own HOME and its own API
# credentials loaded.
#
# In a subshell, because each node has its own credentials file and they must not
# leak into the other node's environment or into this script's. `join` writes the
# file and documents this exact incantation inside it.
#
# Loading it is not optional: the REST API refuses to start when no credentials
# are configured - deliberately, since those endpoints can start and stop the
# node's processing - and that refusal is fatal, so a node started without them
# opens its transaction port and then exits.
start_node() {
  local n="$1"; shift
  local dir="$RUN/node$n/.grap3"
  (
    export HOME="$RUN/node$n"
    if [ -f "$dir/api-credentials.env" ]; then
      set -a
      # shellcheck disable=SC1090
      . "$dir/api-credentials.env"
      set +a
    fi
    nohup "$@" > "$LOGS/node$n.log" 2>&1 < /dev/null &
    echo $! > "$RUN/node$n.pid"
  )
}

alive() {
  local pidf="$1"
  [ -f "$pidf" ] || return 1
  kill -0 "$(cat "$pidf")" 2>/dev/null
}

rss_mb() {
  local pidf="$1"
  alive "$pidf" || { echo 0; return; }
  # ps reports kilobytes on both platforms.
  echo $(( $(ps -o rss= -p "$(cat "$pidf")" 2>/dev/null || echo 0) / 1024 ))
}

need_bins() {
  if [ ! -x "$R/bin/grapepeer" ] || [ ! -x "$R/bin/txgen" ]; then
    echo "building binaries first..."
    (cd "$R" && go build -o bin/grapepeer ./cmd/grapepeer && go build -o bin/txgen ./cmd/txgen) || exit 1
  fi
}

# ---------------------------------------------------------------- lifecycle

down() {
  for n in 0 1; do
    if alive "$RUN/node$n.pid"; then
      kill "$(cat "$RUN/node$n.pid")" 2>/dev/null
    fi
    rm -f "$RUN/node$n.pid"
  done
  if alive "$RUN/txgen.pid"; then kill "$(cat "$RUN/txgen.pid")" 2>/dev/null; fi
  rm -f "$RUN/txgen.pid"
  sleep 2
  # A sweep by binary path as well as by pidfile.
  #
  # Pidfiles alone leak: an `up` interrupted part-way overwrites the pidfile of a
  # node that is still running, and the orphan then holds the metrics port so the
  # next node's diagnostics server fails to bind - which shows up as a scrape
  # returning nothing rather than as anything named. Matching the absolute path is
  # safe: this script's own argv does not contain it.
  pkill -9 -f "$R/bin/grapepeer" >/dev/null 2>&1
  pkill -9 -f "$R/bin/txgen" >/dev/null 2>&1
  sleep 1
  echo "stopped"
}

# up [solo]
#
# A peer by default, and not only for realism. A leader started with no peer does
# not currently finish RunSynchronization, so RunRoboTraderService is never
# reached and the node serves no API at all - it looks alive and answers nothing.
# `up solo` is kept for reproducing that, not for measuring.
up() {
  local mode="${1:-pair}"
  need_bins
  down >/dev/null
  rm -rf "$RUN/node0/.grap3/data" "$RUN/node1/.grap3/data"
  rm -f "$LOGS"/*.log
  mkdir -p "$RUN/node0/.grap3" "$RUN/node1/.grap3"

  echo "cores=$(cores) loadavg=$(loadavg) mode=$mode"

  # Provision each node the way an operator would, rather than hand-writing yaml
  # here. `join` generates a wallet if there is none, writes the configuration
  # and the bootstrap list, and starts nothing - so this exercises the real
  # onboarding path on every run instead of letting it rot untested.
  #
  # Idempotent: the wallet is never overwritten, so restarting a local network
  # keeps the same identities and the same balances.
  local n
  for n in 0 1; do
    [ "$mode" = "solo" ] && [ "$n" = "1" ] && continue
    if [ ! -f "$RUN/node$n/.grap3/grapepeer.yml" ]; then
      "$R/bin/grapepeer" join \
        -data-dir "$RUN/node$n/.grap3" \
        -network 2 \
        -api-port $((9080 + n)) > "$LOGS/join$n.log" 2>&1
      if [ $? -ne 0 ]; then
        echo "provisioning node$n failed:" >&2
        tail -10 "$LOGS/join$n.log" >&2
        return 1
      fi
      echo "provisioned node$n (api port $((9080 + n)))"
    fi
  done

  # Node 0 has to run AS the genesis wallet, not as a wallet of its own.
  #
  # Genesis funding flows from the chain-creating node's own dag wallet out to the
  # exodus wallets, so a node started with -genesis while holding a freshly
  # generated identity funds nobody: every exodus payment is drawn on an empty
  # account, the faucet ends up with no balance, and the benchmark fails at setup
  # with "Balance for wallet ... not found". That is correct behaviour from
  # `join`, which provisions a node to JOIN an existing chain; creating one is a
  # different job.
  #
  # The keys are lifted from the generator profile rather than written here, so
  # the leader and the generator cannot disagree about which account is the
  # faucet, and so this script contains no key material of its own.
  if [ "$mode" != "solo" ] || [ -f "$RUN/node0/.grap3/grapepeer.yml" ]; then
    local gsrc="$R/config/txgenerator-t2.yml"
    local gpub gpriv gwallet cfg tmp
    gpub=$(awk '/^  publickey:/{print $2; exit}' "$gsrc")
    gpriv=$(awk '/^  privatekey:/{print $2; exit}' "$gsrc")
    gwallet=$(awk '/^  wallet:/{print $2; exit}' "$gsrc")
    cfg="$RUN/node0/.grap3/grapepeer.yml"
    if [ -n "$gpub" ] && [ -n "$gpriv" ] && [ -n "$gwallet" ] && \
       ! grep -q "wallet: $gwallet" "$cfg"; then
      tmp="$cfg.tmp"
      # No sed -i: GNU wants no argument and BSD/macOS wants an empty one, so a
      # temp file is the only spelling that works on both.
      sed -e "s|^  publickey: .*|  publickey: $gpub|" \
          -e "s|^  privatekey: .*|  privatekey: $gpriv|" \
          -e "s|^  wallet: .*|  wallet: $gwallet|" \
          -e "s|^  coinbaseaccount: .*|  coinbaseaccount: $gwallet|" \
          "$cfg" > "$tmp" && mv "$tmp" "$cfg"
      echo "node0 runs as the genesis wallet $gwallet"
    fi
  fi

  # The generator reads its own configuration from the node's data directory.
  # config/txgenerator-t2.yml is the network-2 profile and names the genesis
  # faucet the benchmark funds its sender wallets from.
  #
  # That file carries a private key, committed to this repository. It is a
  # throwaway local-testnet key and nothing else may ever use it - the faucet it
  # unlocks holds the whole opening supply of whatever chain it is pointed at.
  if [ ! -f "$RUN/node0/.grap3/txgenerator.yml" ]; then
    cp "$R/config/txgenerator-t2.yml" "$RUN/node0/.grap3/txgenerator.yml" || return 1
    echo "provisioned the generator (network 2, local faucet)"
  fi

  # -grpc on purpose: join writes grpc:false, which is the right default for a
  # real processing node - that port publishes transactions with nothing in front
  # of it. A benchmark needs it, so it is turned on here by flag rather than by
  # weakening the template every node gets provisioned from.
  start_node 0 \
    "$R/bin/grapepeer" -id pnode0 -bootstrap -leader -genesis \
    -node_type 0 -purge -profile -metrics_addr 127.0.0.1:6060 \
    -grpc -grpc_port "$GRPC_PORT" -port "$LEADER_P2P_PORT"

  local code=""
  local i=0
  while [ "$i" -lt 40 ]; do
    sleep 2
    code=$(curl -s --max-time 3 -o /dev/null -w "%{http_code}" "$METRICS" 2>/dev/null)
    [ "$code" = "200" ] && break
    i=$((i+1))
  done
  echo "leader diagnostics endpoint: ${code:-no response}"

  local pid
  pid=$(grep -oE "12D3Koo[A-Za-z0-9]+" "$LOGS/node0.log" 2>/dev/null | head -1)
  if [ -z "$pid" ]; then
    echo "could not read the leader's peer id from $LOGS/node0.log" >&2
    return 1
  fi
  echo "leader peer id: $pid"

  if [ "$mode" != "solo" ]; then
    printf '{ "pnode0": "/ip4/127.0.0.1/tcp/%s/p2p/%s" }\n' "$LEADER_P2P_PORT" "$pid" \
      > "$RUN/node1/.grap3/bootstrap.json"
    start_node 1 "$R/bin/grapepeer" -id pnode1 -node_type 0 -purge
  fi

  # The gRPC service starts only after synchronisation completes, so wait for the
  # port rather than for a fixed sleep.
  i=0
  while [ "$i" -lt 40 ]; do
    sleep 2
    if (exec 3<>/dev/tcp/127.0.0.1/$GRPC_PORT) 2>/dev/null; then
      exec 3>&- 2>/dev/null
      echo "transaction service listening on $GRPC_PORT"
      return 0
    fi
    i=$((i+1))
  done
  echo "transaction service never opened on $GRPC_PORT - see $LOGS/node0.log" >&2
  return 1
}

# ---------------------------------------------------------------- load

load() {
  local dur="${1:-300s}" workers="${2:-48}"
  need_bins
  HOME="$RUN/node0" start_bg "$LOGS/bench.log" "$RUN/txgen.pid" \
    "$R/bin/txgen" -mode bench -grpc_port $GRPC_PORT -bench_max \
    -bench_workers "$workers" -bench_duration "$dur" -bench_report 60s
  local i=0
  while [ "$i" -lt 60 ]; do
    sleep 3
    if grep -q "offering as fast as" "$LOGS/bench.log" 2>/dev/null; then
      echo "offering load: $workers workers for $dur"
      return 0
    fi
    if ! alive "$RUN/txgen.pid"; then
      echo "the generator exited during setup:" >&2
      tail -5 "$LOGS/bench.log" >&2
      return 1
    fi
    i=$((i+1))
  done
  echo "the generator never reached the timed window - see $LOGS/bench.log" >&2
  return 1
}

# ---------------------------------------------------------------- measurement

snap() {
  local m
  m=$(curl -s --max-time 5 "$METRICS")
  # Nothing rather than five empty fields: a row of blanks would parse as zeros
  # and be reported as a node doing no work, which is a different claim from a
  # node that could not be reached.
  [ -z "$m" ] && return 0
  printf '%s %s %s %s %s' \
    "$(echo "$m" | awk '/^grape_tx_accepted_total /{print int($2)}')" \
    "$(echo "$m" | awk '/^grape_site_insert_seconds_count /{print int($2)}')" \
    "$(echo "$m" | awk '/^grape_sites_confirmed_total /{print int($2)}')" \
    "$(echo "$m" | awk '/^grape_live_sites /{print int($2)}')" \
    "$(echo "$m" | awk '/^grape_queue_depth\{queue="publish"\} /{print int($2)}')"
}

# rate [windowSeconds] [windows]
#
# Ingress, insertion and confirmation together, because the three agreeing is
# what distinguishes throughput from buffering. Ingress alone can run ahead while
# the publish queue fills; once that queue is at its ceiling the enqueue blocks
# and the three converge. That convergence is the reading to trust.
rate() {
  local secs="${1:-55}" n="${2:-5}"
  local prev cur t=0
  prev=$(snap)
  local i=1
  while [ "$i" -le "$n" ]; do
    sleep "$secs"
    cur=$(snap); t=$((t+secs))
    # A failed scrape yields an empty line, and reading $1 from it under `set -u`
    # aborts the run with "unbound variable" - losing the whole measurement
    # because one HTTP request did not answer. Say what happened and carry on;
    # the next window may well succeed.
    if [ -z "$prev" ] || [ -z "$cur" ]; then
      printf 't=%4ds  no reading - %s did not answer\n' "$t" "$METRICS"
      prev=$cur
      i=$((i+1))
      continue
    fi
    set -- $prev; local pa=$1 pi=$2 pc=$3
    set -- $cur;  local ca=$1 ci=$2 cc=$3 cl=$4 cq=$5
    printf 't=%4ds ingress=%5d/s inserted=%5d/s confirmed=%5d/s live=%6d pubq=%6d rss=%5dMB load=%s\n' \
      "$t" $(( (ca-pa)/secs )) $(( (ci-pi)/secs )) $(( (cc-pc)/secs )) \
      "$cl" "$cq" "$(rss_mb "$RUN/node0.pid")" "$(loadavg)"
    prev=$cur
    i=$((i+1))
  done
}

# prof [seconds] - profiles taken WHILE load is running.
#
# Taken afterwards they describe an idle node and report the hot path as cheap,
# which is a mistake this project made once already.
prof() {
  local secs="${1:-45}"
  local out="$RUN/prof-$(date +%H%M%S)"
  mkdir -p "$out"
  echo "collecting for ${secs}s into $out"
  curl -s --max-time $((secs+30)) "$PPROF/profile?seconds=$secs" -o "$out/cpu.pb.gz" &
  local cpu=$!
  curl -s --max-time 30 "$PPROF/mutex"            -o "$out/mutex.pb.gz"
  curl -s --max-time 30 "$PPROF/block"            -o "$out/block.pb.gz"
  curl -s --max-time 30 "$PPROF/heap"             -o "$out/heap.pb.gz"
  curl -s --max-time 30 "$PPROF/goroutine?debug=1" -o "$out/goroutine.txt"
  wait $cpu
  echo "read them with:"
  echo "  go tool pprof -top -nodecount=15 bin/grapepeer $out/cpu.pb.gz"
  echo "  go tool pprof -top -nodecount=10 -sample_index=inuse_space bin/grapepeer $out/heap.pb.gz"
}

status() {
  for n in 0 1; do
    if alive "$RUN/node$n.pid"; then
      printf 'node%d: running pid %s, %s MB\n' "$n" "$(cat "$RUN/node$n.pid")" "$(rss_mb "$RUN/node$n.pid")"
    else
      printf 'node%d: not running\n' "$n"
    fi
  done
  alive "$RUN/txgen.pid" && echo "generator: running" || echo "generator: not running"
  echo "cores=$(cores) loadavg=$(loadavg)"
}

# measure - the whole protocol, in the right order.
measure() {
  local dur="${1:-300s}" workers="${2:-48}"
  up pair || return 1
  echo
  load "$dur" "$workers" || return 1
  echo
  rate 55 5
  echo
  echo "generator's own report:"
  sed -n '/--- bench report/,/^---------/p' "$LOGS/bench.log"
  echo
  echo "Read the 'generator stalls' line first: above 1% of offered, the number"
  echo "is the generator's ceiling and not the node's."
}

case "${1:-}" in
  up)      shift; up "${1:-pair}" ;;
  down)    down ;;
  load)    shift; load "${1:-300s}" "${2:-48}" ;;
  rate)    shift; rate "${1:-55}" "${2:-5}" ;;
  prof)    shift; prof "${1:-45}" ;;
  status)  status ;;
  measure) shift; measure "${1:-300s}" "${2:-48}" ;;
  *) sed -n '3,20p' "$0"; exit 2 ;;
esac
