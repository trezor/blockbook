#!/bin/sh

{{define "main" -}}

set -e

INSTALL_DIR={{.Env.BackendInstallPath}}/{{.Coin.Alias}}
DATA_DIR={{.Env.BackendDataPath}}/{{.Coin.Alias}}/backend

NITRO_BIN=$INSTALL_DIR/nitro

# Robinhood Chain is an Arbitrum Orbit chain whose definition is not embedded in the
# nitro binary. Keep this chain info aligned with Robinhood's published mainnet file;
# the genesis state is supplied separately via the packaged {{.Coin.Alias}}.conf.
CHAIN_INFO='[{"chain-id":4663,"parent-chain-id":1,"parent-chain-is-arbitrum":false,"chain-name":"Robinhood Chain","chain-config":{"chainId":4663,"homesteadBlock":0,"daoForkBlock":null,"daoForkSupport":true,"eip150Block":0,"eip150Hash":"0x0000000000000000000000000000000000000000000000000000000000000000","eip155Block":0,"eip158Block":0,"byzantiumBlock":0,"constantinopleBlock":0,"petersburgBlock":0,"istanbulBlock":0,"muirGlacierBlock":0,"berlinBlock":0,"londonBlock":0,"clique":{"period":0,"epoch":0},"arbitrum":{"EnableArbOS":true,"AllowDebugPrecompiles":false,"DataAvailabilityCommittee":false,"InitialArbOSVersion":51,"InitialChainOwner":"0xc8451c5DE260E7ea1b879e7994967077e71230Ca","GenesisBlockNum":0,"MaxCodeSize":98304,"MaxInitCodeSize":196608}},"rollup":{"bridge":"0xDf8755334ce7A73cCF6b581C02eA649AE3E864b3","inbox":"0x1A07cc4BD17E0118BdB54D70990D2158AbAD7a2D","sequencer-inbox":"0xBd0D173EEb87D57A09521c24388a12789F33ba96","rollup":"0x23A19d23e89166adedbDcB432518AB01e4272D94","validator-utils":"0x0000000000000000000000000000000000000000","validator-wallet-creator":"0xB7FE37712e46F28C8f22Ec4bAA33A09fb8B52BD0","stake-token":"0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2","deployed-at":24994238}}]'

# Bind RPC endpoints based on BB_RPC_BIND_HOST_* so defaults remain local unless explicitly overridden.
$NITRO_BIN \
  --chain.id 4663 \
  --chain.info-json "$CHAIN_INFO" \
  --init.genesis-json-file $INSTALL_DIR/{{.Coin.Alias}}.conf \
  --init.download-path $DATA_DIR/tmp \
  --auth.jwtsecret $DATA_DIR/jwtsecret \
  --persistent.chain $DATA_DIR \
  --parent-chain.connection.url http://127.0.0.1:8116 \
  --parent-chain.blob-client.beacon-url http://127.0.0.1:7516 \
  --node.feed.input.url wss://feed.mainnet.chain.robinhood.com \
  --execution.forwarding-target https://sequencer.mainnet.chain.robinhood.com \
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
