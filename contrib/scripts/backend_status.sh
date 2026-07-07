#!/usr/bin/env bash
# Query a coin's back-end RPC for sync status. Resolves the URL from
# BB_{DEV,PROD}_RPC_URL_HTTP_<coin> (controlled by BB_BUILD_ENV), fetched
# via gh-vars.sh. UTXO backends additionally need BB_RPC_USER and BB_RPC_PASS.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$script_dir/../gh-vars.sh"

die() { echo "error: $1" >&2; exit 1; }

[[ $# -ge 1 ]] || die "usage: backend_status.sh <coin>"
coin="$1"

command -v curl >/dev/null 2>&1 || die "curl is not installed"
command -v jq   >/dev/null 2>&1 || die "jq is not installed"

bb_export_gh_vars

build_env="${BB_BUILD_ENV:-dev}"
build_env="${build_env,,}"
case "$build_env" in
  dev)  prefix="BB_DEV_RPC_URL_HTTP_" ;;
  prod) prefix="BB_PROD_RPC_URL_HTTP_" ;;
  *)    die "invalid BB_BUILD_ENV value '$build_env', expected 'dev' or 'prod'" ;;
esac

# Mirror build/tools/templates.go:aliasCandidates — try <coin>, <coin>_archive,
# and the infix variant (e.g. `polygon_bor` → `polygon_archive_bor`).
candidates=("$coin" "${coin}_archive")
if [[ "$coin" == *_* && "$coin" != *_archive* ]]; then
  infix="${coin%%_*}_archive_${coin#*_}"
  [[ "$infix" != "${coin}_archive" ]] && candidates+=("$infix")
fi

# Mirror build/tools/templates.go:withEnvAliasVariants — for each alias
# candidate, also try the env-var-safe variant with '-' normalized to '_'.
env_candidates=()
for alias in "${candidates[@]}"; do
  env_candidates+=("$alias")
  normalized_alias="${alias//-/_}"
  [[ "$normalized_alias" != "$alias" ]] && env_candidates+=("$normalized_alias")
done

url=""
for alias in "${env_candidates[@]}"; do
  candidate="${prefix}${alias}"
  if [[ -n "${!candidate-}" ]]; then
    url="${!candidate}"
    break
  fi
done
[[ -n "$url" ]] || die "no backend RPC URL exported for '${coin}' (tried: ${env_candidates[*]/#/${prefix}})"

user="${BB_RPC_USER-}"
pass="${BB_RPC_PASS-}"
auth=()
if [[ -n "$user" || -n "$pass" ]]; then
  [[ -n "$user" && -n "$pass" ]] || die "set both BB_RPC_USER and BB_RPC_PASS"
  auth=(-u "${user}:${pass}")
fi

rpc() {
    local method="$1" params="${2:-[]}" out status body
    out=$(curl -skS "${auth[@]}" -H 'content-type: application/json' \
              --data "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"${method}\",\"params\":${params}}" \
              -w $'\n%{http_code}' "$url") || die "curl failed for ${url}"
    status="${out##*$'\n'}"
    body="${out%$'\n'*}"
    [[ "$status" == "200" ]] || die "${url} returned HTTP ${status} for ${method}: ${body:0:200}"
    printf '%s' "$body"
}

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

die "backend did not return a valid eth_syncing or getblockchaininfo response: ${resp:0:200}"
