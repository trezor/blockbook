#!/bin/sh

{{define "main" -}}

set -e

export USING_OVM=true
export ETH1_SYNC_SERVICE_ENABLE=false

GETH_BIN={{.Env.BackendInstallPath}}/{{.Coin.Alias}}/geth
DATA_DIR={{.Env.BackendDataPath}}/{{.Coin.Alias}}/backend

CHAINDATA_DIR=$DATA_DIR/geth/chaindata
SNAPSHOT=https://datadirs.optimism.io/mainnet-legacy-archival.tar.zst

if [ ! -d "$CHAINDATA_DIR" ]; then
  wget -c $SNAPSHOT -O - | zstd -cd | tar xf - -C $DATA_DIR
fi

# Bind RPC endpoints based on BB_RPC_BIND_HOST_* so defaults remain local unless explicitly overridden.
$GETH_BIN \
  --networkid 10 \
  --datadir $DATA_DIR \
  --port {{.Ports.BackendP2P}} \
  --rpc \
  --rpcport {{.Ports.BackendHttp}} \
  --rpcaddr {{.Env.RPCBindHost}} \
  --rpcapi eth,rollup,net,web3,debug \
  --rpcvhosts "*" \
  --rpccorsdomain "*" \
  --ws \
  --wsport {{.Ports.BackendRPC}} \
  --wsaddr {{.Env.RPCBindHost}} \
  --wsapi eth,rollup,net,web3,debug \
  --wsorigins "*" \
  --nousb \
  --ipcdisable \
  --nat=none \
  --nodiscover

{{end}}
