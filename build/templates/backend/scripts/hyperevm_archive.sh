#!/bin/sh

{{define "main" -}}

set -e

RETH_BIN={{.Env.BackendInstallPath}}/{{.Coin.Alias}}/reth-hl
DATA_DIR={{.Env.BackendDataPath}}/{{.Coin.Alias}}/backend

# HyperEVM (Hyperliquid) archive node served by nanoreth (reth-hl). nanoreth does
# not re-execute blocks; it ingests pre-executed blocks + receipts from Hyperliquid's
# S3 stream (s3://hl-mainnet-evm-blocks/). AWS credentials with read access must be
# provisioned for the backend system user (~/.aws/credentials) out of band; they are
# secrets and are not shipped in this package. Both HTTP and WS RPC are exposed
# because Blockbook's Ethereum driver dials a WebSocket endpoint at startup.
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
