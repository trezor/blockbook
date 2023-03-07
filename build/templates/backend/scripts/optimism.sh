#!/bin/sh

{{define "main" -}}

set -e

export CHAIN_ID=10
export USING_OVM=true
export ROLLUP_DISABLE_TRANSFERS=false
export ROLLUP_ENABLE_L2_GAS_POLLING=false
export ROLLUP_ADDRESS_MANAGER_OWNER_ADDRESS=0x9BA6e03D8B90dE867373Db8cF1A58d2F7F006b3A

GETH_BIN={{.Env.BackendInstallPath}}/{{.Coin.Alias}}/geth
DATA_DIR={{.Env.BackendDataPath}}/{{.Coin.Alias}}/backend

CHAINDATA_DIR=$DATA_DIR/geth/chaindata
KEYSTORE_DIR=$DATA_DIR/keystore
BLOCK_SIGNER_ADDRESS=0x00000398232E2064F896018496b4b44b3D62751F
BLOCK_SIGNER_PRIVATE_KEY=6587ae678cf4fc9a33000cdbf9f35226b71dcc6a4684a31203241f9bcfd55d27
BLOCK_SIGNER_PRIVATE_KEY_PASSWORD=pwd

L2GETH_GENESIS_URL=https://storage.googleapis.com/optimism/mainnet/genesis-berlin.json
L2GETH_GENESIS_HASH=0x106b0a3247ca54714381b1109e82cc6b7e32fd79ae56fbcc2e7b1541122f84ea
L2GETH_BERLIN_ACTIVATION_HEIGHT=3950000

if [ ! -d "$KEYSTORE_DIR" ]; then
    echo -n $BLOCK_SIGNER_PRIVATE_KEY_PASSWORD > $DATA_DIR/password
    echo -n $BLOCK_SIGNER_PRIVATE_KEY > $DATA_DIR/block-signer-key
    $GETH_BIN account import --datadir=$DATA_DIR --password=$DATA_DIR/password $DATA_DIR/block-signer-key
fi

if [ ! -d "$CHAINDATA_DIR" ]; then
    $GETH_BIN init --datadir=$DATA_DIR $L2GETH_GENESIS_URL $L2GETH_GENESIS_HASH
else
    if !($GETH_BIN dump-chain-cfg --datadir=$DATA_DIR | grep -q "\"berlinBlock\": $L2GETH_BERLIN_ACTIVATION_HEIGHT"); then
        $GETH_BIN init --datadir=$DATA_DIR $L2GETH_GENESIS_URL $L2GETH_GENESIS_HASH
    fi
fi

$GETH_BIN \
  --datadir=$DATA_DIR \
  --password=$DATA_DIR/password \
  --networkid=10 \
  --allow-insecure-unlock \
  --unlock=$BLOCK_SIGNER_ADDRESS \
  --mine \
  --miner.etherbase=$BLOCK_SIGNER_ADDRESS \
  --miner.gastarget=15000000 \
  --miner.gaslimit=15000000 \
  --port={{.Ports.BackendP2P}} \
  --rpc \
  --rpcaddr=127.0.0.1 \
  --rpcport={{.Ports.BackendHttp}} \
  --rpcapi=eth,rollup,net,web3,debug \
  --rpcvhosts="*" \
  --rpccorsdomain="*" \
  --ws \
  --wsaddr=127.0.0.1 \
  --wsport={{.Ports.BackendRPC}} \
  --wsapi=eth,rollup,net,web3,debug \
  --wsorigins="*" \
  --eth1.syncservice \
  --eth1.ctcdeploymentheight=13596466 \
  --rollup.backend=l2 \
  --rollup.clienthttp=http://127.0.0.1:8302 \
  --rollup.maxcalldatasize=40000 \
  --rollup.pollinterval=1s \
  --rollup.verifier \
  --sequencer.clienthttp=https://mainnet.optimism.io \
  --cache=4096 \
  --ipcdisable \
  --nat=none \
  --nousb \
  --nodiscover \
  --syncmode=full \
  --gcmode=full

{{end}}