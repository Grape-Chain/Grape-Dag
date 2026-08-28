#!/usr/bin/env bash
#
# setup-dev.sh - check a development machine can build and run a Grape node,
# and say precisely what is missing when it cannot.
#
# Works on macOS (Apple Silicon or Intel) and Linux. Written for bash 3.2,
# because that is what macOS ships - no associative arrays, no ${var,,}.
#
# Checks rather than installs, except with --install, because a script that
# silently installs toolchains on someone's workstation is a script nobody
# should run twice. Every failure prints the command that fixes it.
#
#   ./scripts/setup-dev.sh              # report what is missing
#   ./scripts/setup-dev.sh --install    # install the missing pieces (brew/apt)
#   ./scripts/setup-dev.sh --verify     # also build Go and Java, and run tests

set -u

GO_MIN="1.24.6"
JDK_WANT="17"

install=0
verify=0
for arg in "$@"; do
  case "$arg" in
    --install) install=1 ;;
    --verify)  verify=1 ;;
    -h|--help) sed -n '3,20p' "$0"; exit 0 ;;
    *) echo "unknown option: $arg" >&2; exit 2 ;;
  esac
done

# ---------------------------------------------------------------- platform

os="$(uname -s)"
arch="$(uname -m)"
case "$os" in
  Darwin) cores=$(sysctl -n hw.ncpu); mem_gb=$(( $(sysctl -n hw.memsize) / 1073741824 )) ;;
  Linux)  cores=$(nproc); mem_gb=$(( $(awk '/MemTotal/{print $2}' /proc/meminfo) / 1048576 )) ;;
  *)      echo "unsupported platform: $os" >&2; exit 2 ;;
esac

ok=0
fail=0
warn=0

say()  { printf '  %s\n' "$*"; }
good() { printf '  \033[32mok\033[0m    %s\n' "$*"; ok=$((ok+1)); }
bad()  { printf '  \033[31mmiss\033[0m  %s\n' "$*"; fail=$((fail+1)); }
soft() { printf '  \033[33mwarn\033[0m  %s\n' "$*"; warn=$((warn+1)); }
fix()  { printf '        fix: %s\n' "$*"; }

echo
echo "Grape-Dag development machine check"
echo "==================================="
say "$os $arch, $cores cores, ${mem_gb} GiB memory"

# How many agents a workflow will run at once. This is the number that decides
# whether a large fan-out is worth starting: the cap is min(16, cores - 2), so a
# four-core machine runs two agents however many you ask for.
agents=$(( cores - 2 ))
[ "$agents" -gt 16 ] && agents=16
[ "$agents" -lt 1 ] && agents=1
say "concurrent agents this machine supports: $agents  (cap is min(16, cores - 2))"
if [ "$agents" -lt 8 ]; then
  soft "fewer than 8 concurrent agents - large fan-outs will queue and take much longer"
fi
echo

# ---------------------------------------------------------------- helpers

have() { command -v "$1" >/dev/null 2>&1; }

# version_ge A B - true when version A is at least version B.
version_ge() {
  [ "$1" = "$2" ] && return 0
  local lowest
  lowest=$(printf '%s\n%s\n' "$1" "$2" | sort -t. -k1,1n -k2,2n -k3,3n | head -1)
  [ "$lowest" = "$2" ]
}

pkg_install() {
  if [ "$install" -eq 0 ]; then
    return 1
  fi
  case "$os" in
    Darwin)
      have brew || { echo "Homebrew is needed for --install: https://brew.sh"; return 1; }
      # shellcheck disable=SC2086
      brew install $1 ;;
    Linux)
      have apt-get && sudo apt-get install -y $2 ;;
  esac
}

# ---------------------------------------------------------------- go

echo "Go toolchain"
if have go; then
  gov=$(go version | awk '{print $3}' | sed 's/^go//')
  if version_ge "$gov" "$GO_MIN"; then
    good "go $gov (needs >= $GO_MIN)"
  else
    bad "go $gov is older than the required $GO_MIN"
    fix "brew upgrade go   # or download from https://go.dev/dl/"
  fi
  # cgo is not used anywhere in this tree, which is what makes the node build
  # unchanged on Apple Silicon. Worth stating, because a future cgo dependency
  # would quietly reintroduce a toolchain requirement.
  if [ -z "$(grep -rl 'import "C"' --include='*.go' . 2>/dev/null | head -1)" ]; then
    good "no cgo in the tree - builds natively on $arch"
  else
    soft "cgo has appeared in the tree; a C toolchain is now required"
  fi
else
  bad "go is not installed"
  fix "brew install go"
  pkg_install go golang-go
fi
echo

# ---------------------------------------------------------------- java

echo "JVM toolchain (for the smart-contract VM under smc/)"
if have java; then
  # Grep for the line that actually carries the version rather than taking the
  # first one. JAVA_TOOL_OPTIONS makes the JVM print a "Picked up ..." banner
  # first, which is where a head -1 lands.
  jv=$(java -version 2>&1 | grep -oE 'version "[0-9]+' | head -1 | grep -oE '[0-9]+')
  if [ -z "$jv" ]; then
    soft "java is installed but its version could not be read"
    jv="unknown"
  fi
  if [ "$jv" = "$JDK_WANT" ]; then
    good "java $jv"
  else
    soft "java $jv - smc/ pins source/target $JDK_WANT, and Lombok 1.18.26 does not run on 21+"
    fix "brew install --cask temurin@$JDK_WANT   then: export JAVA_HOME=\$(/usr/libexec/java_home -v $JDK_WANT)"
  fi
else
  bad "java is not installed"
  fix "brew install --cask temurin@$JDK_WANT"
fi

if have mvn; then
  good "maven $(mvn -v 2>/dev/null | head -1 | awk '{print $3}')"
else
  bad "maven is not installed"
  fix "brew install maven"
  pkg_install maven maven
fi
echo

# ---------------------------------------------------------------- the arch trap

echo "Native library architecture (the one real Apple Silicon risk)"
# smc/grap3-ether depends on Besu's bls12-381 through JNA. That is a wrapper
# around a compiled native library, and native code is per-architecture. If the
# jar carries no darwin-aarch64 build, the JVM cannot load it on Apple Silicon.
#
# It matters less than it sounds: BLS12-381 precompiles are not on Ethereum
# mainnet, so a node that never executes one is unaffected. But it fails at class
# load rather than at first use, so it has to be known about up front.
jar=$(find ~/.m2/repository -name 'bls12-381-*.jar' 2>/dev/null | head -1)
if [ -z "$jar" ]; then
  soft "bls12-381 jar not in the local Maven cache yet - cannot check until smc/ is built once"
  fix "cd smc && mvn -q -DskipTests package   then re-run this script"
elif have unzip; then
  plats=$(unzip -l "$jar" 2>/dev/null | grep -ioE 'darwin[/_-]?(aarch64|arm64)|linux[/_-]?x86[-_]64|darwin[/_-]?x86[-_]64' | sort -u | tr '\n' ' ')
  if [ -z "$plats" ]; then
    soft "could not identify bundled platforms in $(basename "$jar")"
  elif [ "$os" = "Darwin" ] && [ "$arch" = "arm64" ]; then
    case "$plats" in
      *darwin*aarch64*|*darwin*arm64*) good "bls12-381 bundles an arm64 macOS build: $plats" ;;
      *) soft "bls12-381 bundles [$plats] - no arm64 macOS build"
         fix "run the JVM under Docker (linux/amd64), or stub the BLS precompiles for local work" ;;
    esac
  else
    good "bls12-381 bundles: $plats"
  fi
else
  soft "unzip not available; skipping the native-library check"
fi
echo

# ---------------------------------------------------------------- limits

echo "Process limits"
# A node opens a libp2p connection per peer plus a gRPC connection per benchmark
# worker - the saturation run alone uses 48. macOS ships a low soft limit, and
# the failure looks like random dial errors rather than like a limit.
nofile=$(ulimit -n)
if [ "$nofile" = "unlimited" ] || [ "$nofile" -ge 8192 ]; then
  good "open files: $nofile"
else
  soft "open files: $nofile - low for a p2p node plus 48 benchmark connections"
  fix "ulimit -n 65536   (add it to ~/.zshrc to make it stick)"
fi
echo

# ---------------------------------------------------------------- verify

if [ "$verify" -eq 1 ]; then
  echo "Building"
  if go build ./... 2>&1 | head -20; then
    good "go build ./... "
  else
    bad "go build ./... failed"
  fi
  if go vet ./... >/dev/null 2>&1; then
    good "go vet ./..."
  else
    soft "go vet reported findings"
  fi
  echo
  echo "Testing (race detector on; the dag package is the slow one)"
  if go test -race -count=1 ./... 2>&1 | tail -30; then
    good "go test -race ./..."
  else
    bad "tests failed - do not build on top of this until it is green"
  fi
  echo
  if have mvn && [ -d smc ]; then
    echo "Building the contract VM"
    if (cd smc && mvn -q -DskipTests package 2>&1 | tail -20); then
      good "smc/ packaged"
    else
      bad "smc/ build failed - see the output above"
    fi
  fi
  echo
fi

# ---------------------------------------------------------------- summary

echo "-------------------------------------------------------------"
printf '  %d ok, %d missing, %d warnings\n' "$ok" "$fail" "$warn"
if [ "$fail" -gt 0 ]; then
  echo
  echo "  Install what is missing, then re-run with --verify."
  exit 1
fi
echo
echo "  Ready. Next:"
echo "    ./scripts/setup-dev.sh --verify     # build and test everything"
echo "    ./scripts/localnet.sh up            # start a two-node local network"
echo "    ./scripts/localnet.sh measure       # 5-minute saturation measurement"
exit 0
