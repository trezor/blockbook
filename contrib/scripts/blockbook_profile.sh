#!/usr/bin/env bash
# Profile a dev Blockbook instance using Go pprof.
#
# Resolves the dev Blockbook API endpoint from BB_DEV_API_URL_HTTP_* variables
# fetched via contrib/gh-vars.sh, then derives the pprof port used by generated
# dev services: ports.blockbook_internal + 20000.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
source "$script_dir/../gh-vars.sh"

die() {
  echo "error: $1" >&2
  exit 1
}

usage() {
  cat >&2 <<'EOF'
usage: blockbook_profile.sh <coin-config-name> [options]

Options:
  --profile <name>   pprof profile to collect: cpu, heap, goroutine, allocs,
                     threadcreate (default: cpu)
  --seconds <n>      CPU profile duration in seconds (default: 30)
  --nodecount <n>    number of nodes in go tool pprof -top output (default: 20)
  --out-dir <dir>    directory for downloaded profiles (default: /tmp/blockbook-pprof)

Examples:
  contrib/scripts/blockbook_profile.sh polygon_archive --seconds 30
  contrib/scripts/blockbook_profile.sh polygon_archive --profile goroutine
  contrib/scripts/blockbook_profile.sh polygon_archive --profile heap --nodecount 30

Profiling is enabled only on dev Blockbooks. The script expects the dev pprof
port to be reachable from the machine where it runs.
EOF
  exit 2
}

[[ $# -ge 1 ]] || usage
coin="$1"
shift

profile="cpu"
seconds=30
nodecount=20
out_dir="${TMPDIR:-/tmp}/blockbook-pprof"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --profile)
      [[ $# -ge 2 ]] || die "--profile requires a value"
      profile="$2"
      shift 2
      ;;
    --seconds)
      [[ $# -ge 2 ]] || die "--seconds requires a value"
      seconds="$2"
      shift 2
      ;;
    --nodecount)
      [[ $# -ge 2 ]] || die "--nodecount requires a value"
      nodecount="$2"
      shift 2
      ;;
    --out-dir)
      [[ $# -ge 2 ]] || die "--out-dir requires a value"
      out_dir="$2"
      shift 2
      ;;
    -h|--help)
      usage
      ;;
    *)
      die "unknown argument: $1"
      ;;
  esac
done

case "$profile" in
  cpu|heap|goroutine|allocs|threadcreate) ;;
  *) die "unsupported profile '$profile'" ;;
esac

[[ "$seconds" =~ ^[0-9]+$ && "$seconds" -ge 1 ]] || die "--seconds must be a positive integer"
[[ "$nodecount" =~ ^[0-9]+$ && "$nodecount" -ge 1 ]] || die "--nodecount must be a positive integer"

command -v curl >/dev/null 2>&1 || die "curl is not installed"
command -v jq >/dev/null 2>&1 || die "jq is not installed"
command -v python3 >/dev/null 2>&1 || die "python3 is not installed"
command -v go >/dev/null 2>&1 || die "go is not installed; go tool pprof is required"

bb_export_gh_vars
export BB_BUILD_ENV="${BB_BUILD_ENV:-dev}"

config_path="$repo_root/configs/coins/${coin}.json"
[[ -f "$config_path" ]] || die "missing coin config: $config_path"

resolver_output="$(
  python3 - "$repo_root" "$coin" <<'PY'
import json
import os
import shlex
import sys
from pathlib import Path
from urllib.parse import urlparse, urlunparse

repo_root = Path(sys.argv[1])
coin = sys.argv[2]
cfg = json.loads((repo_root / "configs" / "coins" / f"{coin}.json").read_text())

test_identity = (cfg.get("coin", {}).get("test_name") or coin).strip()
alias = (cfg.get("coin", {}).get("alias") or coin).strip()
ports = cfg.get("ports", {})
public_port = int(ports.get("blockbook_public") or 0)
internal_port = int(ports.get("blockbook_internal") or 0)
if internal_port <= 0:
    raise SystemExit("missing ports.blockbook_internal")

# Mirror tests/endpoints/endpoints.go:resolveAPIEndpoints — resolve the dev API
# base from BB_DEV_API_URL_HTTP_<test_name>, then the '-'->'_' env variant, so
# this targets the same Blockbook instance the integration tests reach.
candidates = []
for candidate in (test_identity, test_identity.replace("-", "_")):
    if candidate and candidate not in candidates:
        candidates.append(candidate)

base_url = ""
source_var = ""
for candidate in candidates:
    key = f"BB_DEV_API_URL_HTTP_{candidate}"
    value = os.environ.get(key, "").strip()
    if value:
        base_url = value
        source_var = key
        break

if not base_url:
    if public_port <= 0:
        raise SystemExit("no BB_DEV_API_URL_HTTP_* value and missing ports.blockbook_public")
    base_url = f"http://127.0.0.1:{public_port}"
    source_var = "configs/coins fallback"

parsed = urlparse(base_url)
if parsed.scheme not in ("http", "https") or not parsed.hostname:
    raise SystemExit(f"invalid Blockbook HTTP URL from {source_var}: {base_url!r}")

host = parsed.hostname
if ":" in host and not host.startswith("["):
    host_for_url = f"[{host}]"
else:
    host_for_url = host

pprof_port = internal_port + 20000
pprof_base = f"http://{host_for_url}:{pprof_port}/debug/pprof"
status_url = urlunparse((parsed.scheme, parsed.netloc, parsed.path.rstrip("/") + "/api/status", "", "", ""))
metrics_https = f"https://{host_for_url}:{internal_port}/metrics"
metrics_http = f"http://{host_for_url}:{internal_port}/metrics"

values = {
    "BBP_COIN": coin,
    "BBP_TEST_IDENTITY": test_identity,
    "BBP_ALIAS": alias,
    "BBP_SOURCE_VAR": source_var,
    "BBP_BASE_URL": base_url,
    "BBP_STATUS_URL": status_url,
    "BBP_METRICS_HTTPS": metrics_https,
    "BBP_METRICS_HTTP": metrics_http,
    "BBP_PPROF_BASE": pprof_base,
    "BBP_PPROF_PORT": str(pprof_port),
}
for key, value in values.items():
    print(f"{key}={shlex.quote(value)}")
PY
)" || die "failed to resolve dev Blockbook/pprof endpoint for ${coin}"
eval "$resolver_output"

fetch_with_status() {
  local url="$1"
  local out
  out="$(curl -sk --max-time 10 -w $'\n%{http_code}' "$url")" || return 1
  FETCH_STATUS="${out##*$'\n'}"
  FETCH_BODY="${out%$'\n'*}"
}

echo "coin: ${BBP_COIN} (test=${BBP_TEST_IDENTITY}, alias=${BBP_ALIAS})"
echo "dev endpoint: ${BBP_BASE_URL} (${BBP_SOURCE_VAR})"
echo "pprof endpoint: ${BBP_PPROF_BASE}/"
echo

FETCH_STATUS=""
FETCH_BODY=""
if fetch_with_status "$BBP_STATUS_URL"; then
  if [[ "$FETCH_STATUS" == "400" && "${FETCH_BODY,,}" == *"http request to an https server"* ]]; then
    BBP_STATUS_URL="${BBP_STATUS_URL/#http:/https:}"
    fetch_with_status "$BBP_STATUS_URL" || true
  fi
fi

if [[ "$FETCH_STATUS" == "200" ]]; then
  echo "status snapshot:"
  printf '%s' "$FETCH_BODY" | jq -r '
    . as $status
    | $status.blockbook as $bb
    | $status.backend as $be
    | "  sync: inSync=\($bb.inSync) initialSync=\($bb.initialSync) syncMode=\($bb.syncMode) blockbookHeight=\($bb.bestHeight) backendHeight=\($be.blocks // $be.bestHeight // "n/a")"
    , "  mempool: inSync=\($bb.inSyncMempool) size=\($bb.mempoolSize) lastMempoolTime=\($bb.lastMempoolTime)"
    , "  last block: \($bb.lastBlockTime)"' || echo "  (status snapshot body was not valid JSON)" >&2
else
  echo "status snapshot: unavailable from ${BBP_STATUS_URL}${FETCH_STATUS:+ (HTTP ${FETCH_STATUS})}" >&2
fi

metrics_body=""
for metrics_url in "$BBP_METRICS_HTTPS" "$BBP_METRICS_HTTP"; do
  if fetch_with_status "$metrics_url" && [[ "$FETCH_STATUS" == "200" ]]; then
    metrics_body="$FETCH_BODY"
    break
  fi
done

if [[ -n "$metrics_body" ]]; then
  echo "metrics snapshot:"
  awk '
    /^blockbook_average_block_time_seconds\{/ { avg_target=$NF }
    /^blockbook_avg_block_period\{/ { avg_actual=$NF }
    /^blockbook_backend_best_height\{/ { backend_height=$NF }
    /^blockbook_best_height\{/ { blockbook_height=$NF }
    /^blockbook_backend_subscription_age_seconds\{/ { sub_age=$NF }
    /^blockbook_mempool_size\{/ { mempool_size=$NF }
    /^blockbook_mempool_resync_duration_sum\{/ { mempool_resync_sum=$NF }
    /^blockbook_mempool_resync_duration_count\{/ { mempool_resync_count=$NF }
    END {
      if (blockbook_height != "" || backend_height != "") {
        lag = (backend_height == "" || blockbook_height == "") ? "n/a" : sprintf("%.0f", backend_height - blockbook_height)
        printf "  heights: blockbook=%.0f backend=%.0f lag=%s\n", blockbook_height, backend_height, lag
      }
      if (avg_target != "" || avg_actual != "" || sub_age != "") {
        printf "  chain feed: configured_block_time=%ss observed_100_block_period=%ss subscription_age=%ss\n", avg_target, avg_actual, sub_age
      }
      if (mempool_size != "") {
        avg = (mempool_resync_count > 0) ? sprintf("%.1fms", mempool_resync_sum / mempool_resync_count) : "n/a"
        printf "  mempool: size=%.0f resync_count=%.0f avg_resync_duration=%s\n", mempool_size, mempool_resync_count, avg
      }
    }' <<< "$metrics_body"
else
  echo "metrics snapshot: unavailable from ${BBP_METRICS_HTTPS} or ${BBP_METRICS_HTTP}" >&2
fi
echo

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
safe_coin="${coin//[^A-Za-z0-9_.-]/_}"
mkdir -p "$out_dir"
profile_file="${out_dir}/${safe_coin}_${profile}_${timestamp}.pb.gz"

case "$profile" in
  cpu)
    profile_url="${BBP_PPROF_BASE}/profile?seconds=${seconds}"
    max_time=$((seconds + 30))
    ;;
  *)
    profile_url="${BBP_PPROF_BASE}/${profile}"
    max_time=30
    ;;
esac

echo "downloading ${profile} profile: ${profile_url}"
if ! curl -fsS --max-time "$max_time" "$profile_url" -o "$profile_file"; then
  die "failed to download profile from ${profile_url}; profiling is enabled only on dev Blockbooks, so check that the service has -prof and port ${BBP_PPROF_PORT} is reachable"
fi
echo "saved profile: ${profile_file}"
echo
echo "go tool pprof -top:"
go tool pprof -top "-nodecount=${nodecount}" "$profile_file"
