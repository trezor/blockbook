#!/bin/sh

{{define "main" -}}

set -e

INSTALL_DIR={{.Env.BackendInstallPath}}/{{.Coin.Alias}}
DATA_DIR={{.Env.BackendDataPath}}/{{.Coin.Alias}}/backend

HEIMDALL_BIN=$INSTALL_DIR/heimdalld
HOME_DIR=$DATA_DIR/heimdalld
CONFIG_DIR=$HOME_DIR/config

if [ ! -d "$CONFIG_DIR" ]; then
  # init chain
  $HEIMDALL_BIN init --home $HOME_DIR

  # overwrite genesis file
  cp $INSTALL_DIR/genesis.json $CONFIG_DIR/genesis.json
fi

# --bor_rpc_url: backend-polygon-bor ports.backend_http
# --eth_rpc_url: backend-ethereum ports.backend_http
$HEIMDALL_BIN start \
  --home $HOME_DIR \
  --rpc.laddr tcp://127.0.0.1:{{.Ports.BackendRPC}} \
  --p2p.laddr tcp://0.0.0.0:{{.Ports.BackendP2P}} \
  --laddr tcp://127.0.0.1:{{.Ports.BackendHttp}} \
  --p2p.seeds "2a53a15ffc70ad41b6876ecbe05c50a66af01e20@3.211.248.31:26656,6f829065789e5b156cbbf076f9d133b4d7725847@3.212.183.151:26656,7285a532bad665f051c0aadc31054e2e61ca2b3d@3.93.224.197:26656,0b431127d21c8970f1c353ab212be4f1ba86c3bf@184.73.124.158:26656,f4f605d60b8ffaaf15240564e58a81103510631c@159.203.9.164:26656,31b79cf4a628a4619e8e9ae95b72e4354c5a5d90@44.232.55.71:26656,a385dd467d11c4cdb0be8b51d7bfb0990f49abc3@35.199.4.13:26656,daad548c0a163faae1d8d58425f97207acf923fd@35.230.116.151:26656,81c76e82fcc3dc9a0a1554a3edaa09a632795ea8@35.221.13.28:26656" \
  --node tcp://127.0.0.1:{{.Ports.BackendRPC}} \
  --bor_rpc_url http://127.0.0.1:8170 \
  --eth_rpc_url http://127.0.0.1:8136 \
  --rest-server 

{{end}}