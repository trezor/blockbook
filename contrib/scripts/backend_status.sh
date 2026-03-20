#!/usr/bin/env bash
set -euo pipefail

die() { echo "error: $1" >&2; exit 1; }

[[ $# -ge 1 ]] || die "missing coin argument. usage: blockbook_backend_status.sh <coin>"
coin="$1"
var="BB_RPC_URL_HTTP_${coin}"
url="${!var-}"
[[ -n "$url" ]] || die "environment variable ${var} is not set"
user_var="BB_RPC_USER"
pass_var="BB_RPC_PASS"
user="${!user_var-}"
pass="${!pass_var-}"
auth=()
if [[ -n "$user" || -n "$pass" ]]; then
  [[ -n "$user" && -n "$pass" ]] || die "set both ${user_var} and ${pass_var}"
  auth=(-u "${user}:${pass}")
fi
command -v curl >/dev/null 2>&1 || die "curl is not installed"
command -v jq >/dev/null 2>&1 || die "jq is not installed"

rpc() { curl -skS "${auth[@]}" -H 'content-type: application/json' --data "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"$1\",\"params\":${2:-[]}}" "$url"; }

resp="$(rpc eth_syncing)"
if echo "$resp" | jq -e '.error|not' >/dev/null 2>&1; then
  if echo "$resp" | jq -e '.result == false' >/dev/null 2>&1; then
    bn="$(rpc eth_blockNumber)"
    echo "$bn" | jq -e '.error|not' >/dev/null 2>&1 || die "eth_blockNumber failed"
    hex="$(echo "$bn" | jq -r '.result')"
    [[ -n "$hex" && "$hex" != "null" ]] || die "eth_blockNumber returned empty result"
    height=$((16#${hex#0x}))
    jq -n --argjson height "$height" '{backend:"evm", is_synced:true, height:$height}'
  else
    cur_hex="$(echo "$resp" | jq -r '.result.currentBlock')"
    high_hex="$(echo "$resp" | jq -r '.result.highestBlock')"
    [[ -n "$cur_hex" && "$cur_hex" != "null" ]] || die "eth_syncing returned empty currentBlock"
    [[ -n "$high_hex" && "$high_hex" != "null" ]] || die "eth_syncing returned empty highestBlock"
    cur=$((16#${cur_hex#0x}))
    high=$((16#${high_hex#0x}))
    jq -n --argjson height "$cur" --argjson highest "$high" \
      '{backend:"evm", is_synced:false, height:$height, highest:$highest}'
  fi
  exit 0
fi

resp="$(rpc getblockchaininfo)"
if echo "$resp" | jq -e '.result and (.error|not)' >/dev/null 2>&1; then
  echo "$resp" | jq '{backend:"utxo", is_synced:(.result.initialblockdownload|not), height:.result.blocks, getblockchaininfo:.}'
  exit 0
fi

die "backend did not return a valid eth_syncing or getblockchaininfo response"
