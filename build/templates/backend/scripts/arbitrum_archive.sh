#!/bin/sh

{{define "main" -}}

set -e

INSTALL_DIR={{.Env.BackendInstallPath}}/{{.Coin.Alias}}
DATA_DIR={{.Env.BackendDataPath}}/{{.Coin.Alias}}/backend

NITRO_BIN=$INSTALL_DIR/nitro

$NITRO_BIN \
  --chain.name arb1 \
  --init.latest archive \
  --init.download-path $DATA_DIR/tmp \
  --auth.jwtsecret $DATA_DIR/jwtsecret \
  --persistent.chain $DATA_DIR \
  --parent-chain.connection.url http://127.0.0.1:8116 \
  --parent-chain.blob-client.beacon-url http://127.0.0.1:7516 \
  --http.addr 127.0.0.1 \
  --http.port {{.Ports.BackendHttp}} \
  --http.api eth,net,web3,debug,txpool,arb \
  --http.vhosts '*' \
  --http.corsdomain '*' \
  --ws.addr 127.0.0.1 \
  --ws.api eth,net,web3,debug,txpool,arb \
  --ws.port {{.Ports.BackendRPC}} \
  --ws.origins '*' \
  --file-logging.enable='false' \
  --node.staker.enable='false' \
  --execution.caching.archive \
  --execution.tx-lookup-limit 0 \
  --validation.wasm.allowed-wasm-module-roots "$INSTALL_DIR/nitro-legacy/machines,$INSTALL_DIR/target/machines"

{{end}}