#!/bin/sh

{{define "main" -}}

set -e

export USING_OVM=true
export ETH1_SYNC_SERVICE_ENABLE=false

GETH_BIN={{.Env.BackendInstallPath}}/{{.Coin.Alias}}/geth
DATA_DIR={{.Env.BackendDataPath}}/{{.Coin.Alias}}/backend

CHAINDATA_DIR=$DATA_DIR/geth/chaindata
SNAPSHOT=https://storage.googleapis.com/oplabs-goerli-data/goerli-legacy-archival.tar

if [[ ! -d "$CHAINDATA_DIR" ]]; then
  wget -c $SNAPSHOT -O - | tar -xvf - -C $DATA_DIR
fi

$GETH_BIN \
  --datadir=$DATA_DIR \
  --networkid=420 \
  --port={{.Ports.BackendP2p}} \
  --rpc \
  --rpcport={{.Ports.BackendHttp}} \
  --rpcaddr=127.0.0.1 \
  --rpcapi=eth,rollup,net,web3,debug \
  --rpcvhosts="*" \
  --rpccorsdomain="*" \
  --ws \
  --wsport={{.Ports.BackendRPC}} \
  --wsaddr=0.0.0.0 \
  --wsapi=eth,rollup,net,web3,debug \
  --wsorigins="*" \
  --nousb \
  --ipcdisable \
  --nat=none \
  --nodiscover

{{end}}