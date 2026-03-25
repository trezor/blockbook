#!/usr/bin/env bash
set -euo pipefail

die() { echo "error: $1" >&2; exit 1; }
[[ $# -ge 1 ]] || die "missing coin argument. usage: blockbook_status.sh <coin> [hostname]"
coin="$1"
if [[ -n "${2-}" ]]; then
  host="$2"
else
  host="localhost"
fi

var="BB_DEV_API_URL_HTTP_${coin}"
base_url="${!var-}"
[[ -n "$base_url" ]] || die "environment variable ${var} is not set"
command -v curl >/dev/null 2>&1 || die "curl is not installed"
command -v jq >/dev/null 2>&1 || die "jq is not installed"

# Preserve legacy host override argument by replacing host in the configured base URL.
if [[ -n "${2-}" ]]; then
  if [[ "$base_url" =~ ^(https?://)([^/@]+@)?([^/:]+)(:[0-9]+)?(.*)$ ]]; then
    base_url="${BASH_REMATCH[1]}${BASH_REMATCH[2]}${host}${BASH_REMATCH[4]}${BASH_REMATCH[5]}"
  else
    die "invalid URL in ${var}: ${base_url}"
  fi
fi

status_url="${base_url%/}/api/status"
curl -skv "$status_url" | jq
