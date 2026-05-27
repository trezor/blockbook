#!/usr/bin/env bash
# Hit the /api/status endpoint of a Blockbook instance for a given coin.
# Resolves the URL from BB_DEV_API_URL_HTTP_<coin>, fetched via gh-vars.sh.
# Retries with https:// if the http:// endpoint responds with the nginx
# "HTTP request to HTTPS server" 400 — matches what the connectivity
# integration test does (tests/connectivity/blockbook_connectivity.go).
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$script_dir/../gh-vars.sh"

die() { echo "error: $1" >&2; exit 1; }
[[ $# -eq 1 ]] || die "usage: blockbook_status.sh <coin>"
coin="$1"

command -v curl >/dev/null 2>&1 || die "curl is not installed"
command -v jq   >/dev/null 2>&1 || die "jq is not installed"

bb_export_gh_vars

# Mirror build/tools/templates.go:withEnvAliasVariants — also try the env-var-safe
# variant with '-' normalized to '_' (gh-vars.sh exports BB_* names with this form).
var="BB_DEV_API_URL_HTTP_${coin}"
base_url="${!var-}"
if [[ -z "$base_url" ]]; then
    normalized_coin="${coin//-/_}"
    if [[ "$normalized_coin" != "$coin" ]]; then
        var="BB_DEV_API_URL_HTTP_${normalized_coin}"
        base_url="${!var-}"
    fi
fi
[[ -n "$base_url" ]] || die "no Blockbook URL exported for '${coin}' (BB_DEV_API_URL_HTTP_${coin} not set)"

# Curl with response body and status; pure-bash split on the trailing newline.
fetch() {
    local out
    out=$(curl -sk --max-time 10 -w $'\n%{http_code}' "$1") || die "curl failed for $1"
    status="${out##*$'\n'}"
    body="${out%$'\n'*}"
}

status=""; body=""
status_url="${base_url%/}/api/status"
fetch "$status_url"

if [[ "$status" == "400" && "${body,,}" == *'http request to an https server'* ]]; then
    status_url="${status_url/#http:/https:}"
    fetch "$status_url"
fi

[[ "$status" == "200" ]] || die "GET $status_url returned HTTP $status: ${body:0:200}"
printf '%s' "$body" | jq
