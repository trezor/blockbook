#!/bin/sh

{{define "main" -}}

set -e

INSTALL_DIR={{.Env.BackendInstallPath}}/{{.Coin.Alias}}
DATA_DIR={{.Env.BackendDataPath}}/{{.Coin.Alias}}/backend

HEIMDALL_BIN=$INSTALL_DIR/heimdalld
HOME_DIR=$DATA_DIR/heimdalld
CONFIG_DIR=$HOME_DIR/config

if [[ ! -d "$CONFIG_DIR" ]]; then
  # init chain
  $HEIMDALL_BIN init --home $HOME_DIR

  # overwrite genesis file
  cp $INSTALL_DIR/genesis.json $CONFIG_DIR/genesis.json
fi

# bor_rpc_url: backend-polygon-bor service
# eth_rpc_url: backend-ethereum service
$HEIMDALL_BIN start \
  --home $HOME_DIR \
  --rpc.laddr tcp://127.0.0.1:{{.Ports.BackendRPC}} \
  --p2p.laddr tcp://0.0.0.0:{{.Ports.BackendP2P}} \
  --laddr tcp://127.0.0.1:{{.Ports.BackendHttp}} \
  --bor_rpc_url http://127.0.0.1:8303 \
  --eth_rpc_url http://127.0.0.1:8136 \
  --rest-server 

{{end}}