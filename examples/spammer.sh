#!/bin/sh
set -e
if [ -z "${WAITING_TIMEOUT}" ]; then
  echo "Set default timeout=30s"
  WAITING_TIMEOUT=30
fi

echo "Waiting timeout ${WAITING_TIMEOUT}s ..."
sleep "${WAITING_TIMEOUT}"

node_ip=$1
[ ! -z "${node_ip}" ] && shift
tx_rate=$1
[ ! -z "${tx_rate}" ] && shift
tx_max=$1
[ ! -z "${tx_max}" ] && shift

#set defaults
[ -z "${node_ip}" ] && node_ip="localhost"
[ -z "${tx_rate}" ] && tx_rate=5
[ -z "${tx_max}" ] && tx_max=50000
echo $node_ip $tx_rate $tx_max $@

cfg="/home/luna/.grap3/txgenerator.yml"

sed -ri "s/nodeip: \"(.+)\".*/nodeip: \"${node_ip}\"/g;s/txrate:(.+)/txrate: ${tx_rate}/g;s/txmax:(.+)/txmax: ${tx_max}/g" "$cfg"

cat "$cfg"

echo "start txgen"
txgen $@
