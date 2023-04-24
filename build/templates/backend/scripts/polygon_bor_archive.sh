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
  --bootnodes enode://0cb82b395094ee4a2915e9714894627de9ed8498fb881cec6db7c65e8b9a5bd7f2f25cc84e71e89d0947e51c76e85d0847de848c7782b13c0255247a6758178c@44.232.55.71:30303,enode://88116f4295f5a31538ae409e4d44ad40d22e44ee9342869e7d68bdec55b0f83c1530355ce8b41fbec0928a7d75a5745d528450d30aec92066ab6ba1ee351d710@159.203.9.164:30303 \
  --port {{.Ports.BackendP2P}} \
  --http \
  --http.addr 127.0.0.1 \
  --http.port {{.Ports.BackendHttp}} \
  --http.api eth,net,web3,debug,txpool,bor \
  --http.vhosts '*' \
  --http.corsdomain '*' \
  --ws \
  --ws.addr 127.0.0.1 \
  --ws.port {{.Ports.BackendRPC}} \
  --ws.api eth,net,web3,debug,txpool,bor \
  --ws.origins '*' \
  --gcmode archive \
  --txlookuplimit 0 \
  --cache 4096 \
  --ipcdisable \
  --nat none 

{{end}}