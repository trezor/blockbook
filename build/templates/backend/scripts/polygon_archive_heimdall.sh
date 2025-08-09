#!/bin/sh

{{define "main" -}}

set -e

INSTALL_DIR={{.Env.BackendInstallPath}}/{{.Coin.Alias}}
DATA_DIR={{.Env.BackendDataPath}}/{{.Coin.Alias}}/backend

HEIMDALL_BIN=$INSTALL_DIR/heimdalld
HOME_DIR=$DATA_DIR
CONFIG_DIR=$HOME_DIR/config

if [ ! -d "$CONFIG_DIR" ]; then
  # init chain
  $HEIMDALL_BIN init $(hostname -s) --home $HOME_DIR --chain-id heimdallv2-137
fi

# --bor_rpc_url: backend-polygon-bor-archive ports.backend_http
# --eth_rpc_url: backend-ethereum-archive ports.backend_http
$HEIMDALL_BIN start \
  --home $HOME_DIR \
  --rpc.laddr tcp://127.0.0.1:{{.Ports.BackendRPC}} \
  --p2p.laddr tcp://0.0.0.0:{{.Ports.BackendP2P}} \
  --grpc_server tcp://127.0.0.1:{{.Ports.BackendHttp}} \
  --p2p.seeds "e019e16d4e376723f3adc58eb1761809fea9bee0@35.234.150.253:26656,7f3049e88ac7f820fd86d9120506aaec0dc54b27@34.89.75.187:26656,1f5aff3b4f3193404423c3dd1797ce60cd9fea43@34.142.43.240:26656,2d5484feef4257e56ece025633a6ea132d8cadca@35.246.99.203:26656,17e9efcbd173e81a31579310c502e8cdd8b8ff2e@35.197.233.249:26656,72a83490309f9f63fdca3a0bef16c290e5cbb09c@35.246.95.65:26656,00677b1b2c6282fb060b7bb6e9cc7d2d05cdd599@34.105.180.11:26656,721dd4cebfc4b78760c7ee5d7b1b44d29a0aa854@34.147.169.102:26656,4760b3fc04648522a0bcb2d96a10aadee141ee89@34.89.55.74:26656" \
  --bor_rpc_url http://127.0.0.1:8172 \
  --eth_rpc_url http://127.0.0.1:8116 
{{end}}