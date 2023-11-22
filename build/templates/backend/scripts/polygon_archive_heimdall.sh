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

# --bor_rpc_url: backend-polygon-bor-archive ports.backend_http
# --eth_rpc_url: backend-ethereum-archive ports.backend_http
$HEIMDALL_BIN start \
  --home $HOME_DIR \
  --chain=mainnet \
  --rpc.laddr tcp://127.0.0.1:{{.Ports.BackendRPC}} \
  --p2p.laddr tcp://0.0.0.0:{{.Ports.BackendP2P}} \
  --laddr tcp://127.0.0.1:{{.Ports.BackendHttp}} \
  --p2p.seeds "f4f605d60b8ffaaf15240564e58a81103510631c@159.203.9.164:26656,4fb1bc820088764a564d4f66bba1963d47d82329@44.232.55.71:26656,2eadba4be3ce47ac8db0a3538cb923b57b41c927@35.199.4.13:26656,3b23b20017a6f348d329c102ddc0088f0a10a444@35.221.13.28:26656,25f5f65a09c56e9f1d2d90618aa70cd358aa68da@35.230.116.151:26656,4cd60c1d76e44b05f7dfd8bab3f447b119e87042@54.147.31.250:26656,b18bbe1f3d8576f4b73d9b18976e71c65e839149@34.226.134.117:26656,1500161dd491b67fb1ac81868952be49e2509c9f@52.78.36.216:26656,dd4a3f1750af5765266231b9d8ac764599921736@3.36.224.80:26656,8ea4f592ad6cc38d7532aff418d1fb97052463af@34.240.245.39:26656,e772e1fb8c3492a9570a377a5eafdb1dc53cd778@54.194.245.5:26656,6726b826df45ac8e9afb4bdb2469c7771bd797f1@52.209.21.164:26656" \
  --node tcp://127.0.0.1:{{.Ports.BackendRPC}} \
  --bor_rpc_url http://127.0.0.1:8172 \
  --eth_rpc_url http://127.0.0.1:8116 \
  --rest-server 

{{end}}