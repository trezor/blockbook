#!/bin/sh

{{define "main" -}}

set -e

GETH_BIN={{.Env.BackendInstallPath}}/{{.Coin.Alias}}/geth
DATA_DIR={{.Env.BackendDataPath}}/{{.Coin.Alias}}/backend

CHAINDATA_DIR=$DATA_DIR/geth/chaindata
SNAPSHOT=https://mainnet-full-snapshots.base.org/$(curl https://mainnet-full-snapshots.base.org/latest)

if [ ! -d "$CHAINDATA_DIR" ]; then
  wget -c $SNAPSHOT -O - | zstd -cd | tar xf - --strip-components=1 -C $DATA_DIR
fi

$GETH_BIN \
  --op-network base-mainnet \
  --datadir $DATA_DIR \
  --authrpc.jwtsecret $DATA_DIR/jwtsecret \
  --authrpc.addr 127.0.0.1 \
  --authrpc.port {{.Ports.BackendAuthRpc}} \
  --authrpc.vhosts "*" \
  --port {{.Ports.BackendP2P}} \
  --http \
  --http.port {{.Ports.BackendHttp}} \
  --http.addr 127.0.0.1 \
  --http.api eth,net,web3,debug,txpool,engine \
  --http.vhosts "*" \
  --http.corsdomain "*" \
  --ws \
  --ws.port {{.Ports.BackendRPC}} \
  --ws.addr 127.0.0.1 \
  --ws.api eth,net,web3,debug,txpool,engine \
  --ws.origins "*" \
  --rollup.disabletxpoolgossip=true \
  --rollup.sequencerhttp https://mainnet-sequencer.base.io \
  --state.scheme hash \
  --history.transactions 0 \
  --cache 4096 \
  --syncmode full \
  --maxpeers 0 \
  --nodiscover

{{end}}
