#!/usr/bin/env sh

set -e

echo "Running bitcoind..."
bitcoind

# with no config file in ~/.bitcoin:
# bitcoind -regtest -txindex -server -fallbackfee=0.00001 -rpcuser=rpc -rpcpassword=rpc -rpcport=18021 -zmqpubhashtx=tcp://127.0.0.1:48321 -zmqpubhashblock=tcp://127.0.0.1:48321
