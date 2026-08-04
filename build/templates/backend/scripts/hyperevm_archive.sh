#!/bin/sh

{{define "main" -}}

set -e

RETH_BIN={{.Env.BackendInstallPath}}/{{.Coin.Alias}}/reth-hl
DATA_DIR={{.Env.BackendDataPath}}/{{.Coin.Alias}}/backend

# nanoreth (reth-hl) does not re-execute blocks, it ingests pre-executed blocks and
# receipts from s3://hl-mainnet-evm-blocks/, so the backend user needs AWS read
# credentials in ~/.aws/credentials, provisioned out of band as a secret.
# Bind RPC endpoints based on BB_RPC_BIND_HOST_* so defaults remain local unless explicitly overridden.
$RETH_BIN node \
  --datadir $DATA_DIR \
  --http \
  --http.addr {{.Env.RPCBindHost}} \
  --http.port {{.Ports.BackendHttp}} \
  --http.api eth,net,web3,txpool,debug,trace \
  --http.vhosts '*' \
  --http.corsdomain '*' \
  --ws \
  --ws.addr {{.Env.RPCBindHost}} \
  --ws.port {{.Ports.BackendRPC}} \
  --ws.api eth,net,web3,txpool,debug,trace \
  --ws.origins '*' \
  --port {{.Ports.BackendP2P}} \
  --disable-discovery \
  --s3

{{end}}
