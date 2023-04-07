#!/bin/sh

{{define "main" -}}

set -e

INSTALL_DIR={{.Env.BackendInstallPath}}/{{.Coin.Alias}}
DATA_DIR={{.Env.BackendDataPath}}/{{.Coin.Alias}}/backend

GETH_BIN=$INSTALL_DIR/geth_linux
CHAINDATA_DIR=$DATA_DIR/geth/chaindata

if [ ! -d "$CHAINDATA_DIR" ]; then
  $GETH_BIN init --datadir $DATA_DIR $INSTALL_DIR/genesis.json
fi

$GETH_BIN \
  --config $INSTALL_DIR/config.toml \
  --datadir $DATA_DIR \
  --port {{.Ports.BackendP2P}} \
  --http \
  --http.addr 127.0.0.1 \
  --http.port {{.Ports.BackendHttp}} \
  --http.api eth,net,web3,debug,txpool \
  --http.vhosts '*' \
  --http.corsdomain '*' \
  --ws \
  --ws.addr 127.0.0.1 \
  --ws.port {{.Ports.BackendRPC}} \
  --ws.api eth,net,web3,debug,txpool \
  --ws.origins '*' \
  --syncmode full \
  --maxpeers 200 \
  --rpc.allow-unprotected-txs \
  --txlookuplimit 0 \
  --cache 8000 \
  --ipcdisable \
  --nat none

{{end}}