#!/bin/sh

{{define "main" -}}

set -e

INSTALL_DIR={{.Env.BackendInstallPath}}/{{.Coin.Alias}}
DATA_DIR={{.Env.BackendDataPath}}/{{.Coin.Alias}}/backend

BOR_BIN=$INSTALL_DIR/bor

# --bor.heimdall = backend-polygon-heimdall-archive ports.backend_http
$BOR_BIN server \
  --chain $INSTALL_DIR/genesis.json \
  --syncmode full \
  --datadir $DATA_DIR \
  --bor.heimdall http://127.0.0.1:8173 \
  --bootnodes enode://76316d1cb93c8ed407d3332d595233401250d48f8fbb1d9c65bd18c0495eca1b43ec38ee0ea1c257c0abb7d1f25d649d359cdfe5a805842159cfe36c5f66b7e8@52.78.36.216:30303,enode://b8f1cc9c5d4403703fbf377116469667d2b1823c0daf16b7250aa576bacf399e42c3930ccfcb02c5df6879565a2b8931335565f0e8d3f8e72385ecf4a4bf160a@3.36.224.80:30303,enode://8729e0c825f3d9cad382555f3e46dcff21af323e89025a0e6312df541f4a9e73abfa562d64906f5e59c51fe6f0501b3e61b07979606c56329c020ed739910759@54.194.245.5:30303,enode://681ebac58d8dd2d8a6eef15329dfbad0ab960561524cf2dfde40ad646736fe5c244020f20b87e7c1520820bc625cfb487dd71d63a3a3bf0baea2dbb8ec7c79f1@34.240.245.39:30303 \
  --port {{.Ports.BackendP2P}} \
  --http \
  --http.addr 0.0.0.0 \
  --http.port {{.Ports.BackendHttp}} \
  --http.api eth,net,web3,debug,txpool,bor \
  --http.vhosts '*' \
  --http.corsdomain '*' \
  --ws \
  --ws.addr 0.0.0.0 \
  --ws.port {{.Ports.BackendRPC}} \
  --ws.api eth,net,web3,debug,txpool,bor \
  --ws.origins '*' \
  --gcmode archive \
  --txlookuplimit 0 \
  --cache 4096 \
s  --nat none 

{{end}}