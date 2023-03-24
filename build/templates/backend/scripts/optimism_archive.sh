#!/bin/sh

{{define "main" -}}

set -e

GETH_BIN={{.Env.BackendInstallPath}}/{{.Coin.Alias}}/geth
DATA_DIR={{.Env.BackendDataPath}}/{{.Coin.Alias}}/backend

CHAINDATA_DIR=$DATA_DIR/geth/chaindata
SNAPSHOT=

if [[ -n "$SNAPSHOT" && ! -d "$CHAINDATA_DIR" ]]; then
  wget -c $SNAPSHOT -O - | tar -xvf - -C $DATA_DIR
fi

$GETH_BIN \
  --datadir=$DATA_DIR \
  --networkid=10 \
  --authrpc.addr=127.0.0.1 \
  --authrpc.port={{.Ports.BackendAuthRpc}} \
  --authrpc.vhosts="*" \
  --port={{.Ports.BackendP2P}} \
  --http \
  --http.port={{.Ports.BackendHttp}} \
  --http.addr=127.0.0.1 \
  --http.api=eth,net,web3,debug,txpool,engine \
  --http.vhosts="*" \
  --http.corsdomain="*" \
  --ws \
  --ws.port={{.Ports.BackendRPC}} \
  --ws.addr=127.0.0.1 \
  --ws.api=eth,net,web3,debug,txpool,engine \
  --ws.origins="*" \
  --rollup.disabletxpoolgossip=true \
  --rollup.historicalrpc=http://127.0.0.1:8303 \
  --rollup.sequencerhttp=https://sequencer.optimism.io \
  --cache=4096 \
  --cache.gc=0 \
  --cache.trie=30 \
  --cache.snapshot=20 \
  --syncmode=full \
  --gcmode=archive \
  --maxpeers=0 \
  --nodiscover

{{end}}
