#!/usr/bin/env sh

set -e

bitcoin-cli createwallet "testwallet" || true
bitcoin-cli loadwallet testwallet || true
bitcoin-cli -rpcwallet=testwallet settxfee 0.00001
bitcoin-cli -rpcwallet=testwallet -generate 101
bitcoin-cli -rpcwallet=testwallet getnewaddress
bitcoin-cli generatetoaddress 101 bcrt1qqpq2efk9vpwt02sgffr3de4aszxmjsacj6er8j
bitcoin-cli -rpcwallet=testwallet sendtoaddress bcrt1qqpq2efk9vpwt02sgffr3de4aszxmjsacj6er8j 10
bitcoin-cli -rpcwallet=testwallet -generate 1
bitcoin-cli getblockchaininfo

# Without the specific bitcoin.conf, need to specify these CLI options:
# bitcoin-cli -regtest -rpcuser=rpc -rpcpassword=rpc -rpcport=18021 createwallet "testwallet"
