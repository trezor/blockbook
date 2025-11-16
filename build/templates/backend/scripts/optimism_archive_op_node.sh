#!/bin/sh

{{define "main" -}}

set -e

BIN={{.Env.BackendInstallPath}}/{{.Coin.Alias}}/op-node
PATH={{.Env.BackendDataPath}}/{{.Coin.Alias}}/backend

$BIN \
  --network op-mainnet \
  --l1 http://127.0.0.1:8116 \
  --l1.beacon http://127.0.0.1:7516 \
  --l1.trustrpc \
  --l1.rpckind=debug_geth \
  --l2 http://127.0.0.1:8402 \
  --rpc.addr 127.0.0.1 \
  --rpc.port {{.Ports.BackendRPC}} \
  --l2.jwt-secret {{.Env.BackendDataPath}}/optimism_archive/backend/jwtsecret \
  --p2p.priv.path $PATH/opnode_p2p_priv.txt \
  --p2p.peerstore.path $PATH/opnode_peerstore_db \
  --p2p.discovery.path $PATH/opnode_discovery_db

{{end}}
