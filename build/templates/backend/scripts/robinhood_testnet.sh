#!/bin/sh

{{define "main" -}}

set -e

INSTALL_DIR={{.Env.BackendInstallPath}}/{{.Coin.Alias}}
DATA_DIR={{.Env.BackendDataPath}}/{{.Coin.Alias}}/backend

NITRO_BIN=$INSTALL_DIR/nitro

# Chain info taken verbatim from the official node config published at
# https://cdn.robinhood.com/assets/generated_assets/hoodchain_docsite/chain-node-configs/robinhood-chain-testnet-config.json
# (parent chain is Ethereum Sepolia).
CHAIN_INFO='[{"chain-id":46630,"parent-chain-id":11155111,"parent-chain-is-arbitrum":false,"chain-name":"Robinhood Chain Sepolia","chain-config":{"chainId":46630,"homesteadBlock":0,"daoForkBlock":null,"daoForkSupport":true,"eip150Block":0,"eip150Hash":"0x0000000000000000000000000000000000000000000000000000000000000000","eip155Block":0,"eip158Block":0,"byzantiumBlock":0,"constantinopleBlock":0,"petersburgBlock":0,"istanbulBlock":0,"muirGlacierBlock":0,"berlinBlock":0,"londonBlock":0,"clique":{"period":0,"epoch":0},"arbitrum":{"EnableArbOS":true,"AllowDebugPrecompiles":false,"DataAvailabilityCommittee":false,"InitialArbOSVersion":51,"InitialChainOwner":"0x7131b1c28Df46d845Ee044f2f79AC19Ba14526a8","GenesisBlockNum":0,"MaxCodeSize":98304,"MaxInitCodeSize":196608}},"rollup":{"bridge":"0x96295BDad104eaD97cC08797b3dC68efF59CcF30","inbox":"0xF2939afA86F6f933A3CE17fCAB007907B6b0B7a4","sequencer-inbox":"0xA0D9dB3DC9791D54b5183C1C1866eFe1eCA7D414","rollup":"0xdc5F8E399DBd8a9F5F87AeC4C23Beb12431b386D","validator-utils":"0x0000000000000000000000000000000000000000","validator-wallet-creator":"0xB7FE37712e46F28C8f22Ec4bAA33A09fb8B52BD0","stake-token":"0x7b79995e5f793A07Bc00c21412e50Ecae098E7f9","deployed-at":10204516}}]'

# Robinhood does not publish a testnet genesis file; the testnet genesis state is
# empty (verified: mainnet-preallocated contracts have no code at testnet block 0),
# so a fresh database is initialized with --init.empty.
# Bind RPC endpoints based on BB_RPC_BIND_HOST_* so defaults remain local unless explicitly overridden.
$NITRO_BIN \
  --chain.id 46630 \
  --chain.info-json "$CHAIN_INFO" \
  --init.empty \
  --init.download-path $DATA_DIR/tmp \
  --auth.jwtsecret $DATA_DIR/jwtsecret \
  --persistent.chain $DATA_DIR \
  --parent-chain.connection.url http://127.0.0.1:18176 \
  --parent-chain.blob-client.beacon-url http://127.0.0.1:17576 \
  --node.feed.input.url wss://feed.testnet.chain.robinhood.com \
  --execution.forwarding-target https://sequencer.testnet.chain.robinhood.com \
  --http.addr {{.Env.RPCBindHost}} \
  --http.port {{.Ports.BackendHttp}} \
  --http.api eth,net,web3,debug,txpool,arb \
  --http.vhosts '*' \
  --http.corsdomain '*' \
  --ws.addr {{.Env.RPCBindHost}} \
  --ws.api eth,net,web3,debug,txpool,arb \
  --ws.port {{.Ports.BackendRPC}} \
  --ws.origins '*' \
  --file-logging.enable='false' \
  --node.staker.enable='false' \
  --execution.caching.archive \
  --execution.tx-indexer.tx-lookup-limit 0 \
  --validation.wasm.allowed-wasm-module-roots "$INSTALL_DIR/target/machines"

{{end}}
