#!/usr/bin/env bash
set -euo pipefail

readonly LOG_PREFIX="CI/CD Pipeline:"
readonly SCRIPT_NAME="[deploy-bb-backend]"

log() {
  printf '%s %s %s\n' "$LOG_PREFIX" "$SCRIPT_NAME" "$*" >&2
}

die() {
  printf '%s error: %s\n' "$LOG_PREFIX" "$*" >&2
  exit 1
}

if [[ $# -lt 1 ]]; then
  die "usage: $(basename "$0") <coin-alias> [--force-confnew]"
fi

coin=""
force_confnew=0

for arg in "$@"; do
  case "$arg" in
    --force-confnew)
      force_confnew=1
      ;;
    -*)
      die "unknown option: $arg"
      ;;
    *)
      if [[ -n "$coin" ]]; then
        die "usage: $(basename "$0") <coin-alias> [--force-confnew]"
      fi
      coin="$arg"
      ;;
  esac
done

if [[ -z "$coin" ]]; then
  die "usage: $(basename "$0") <coin-alias> [--force-confnew]"
fi

config="configs/coins/${coin}.json"
if [[ ! -f "$config" ]]; then
  die "missing coin config $config"
fi

policy_output="$(
  python3 ./.github/scripts/backend_decision.py "$coin"
)"
eval "$policy_output"

deploy_backend="$BACKEND_SHOULD_BUILD"
backend_reason="$BACKEND_REASON"
rpc_env="$BACKEND_RPC_ENV"
rpc_host="$BACKEND_RPC_HOST"
build_env="$BACKEND_BUILD_ENV"

log "coin=${coin}, alias=${BACKEND_COIN_ALIAS}"
log "backend deploy rule: deploy unless the selected BB_{DEV|PROD}_RPC_URL_HTTP_<alias> is non-empty and non-local"
log "backend decision: deploy_backend=${deploy_backend}, reason=${backend_reason}, rpc_env=${rpc_env}, rpc_host=${rpc_host:-<unset>}"

if [[ "$deploy_backend" -eq 1 ]]; then
  log "deploying backend first"
  ./contrib/scripts/backend-deploy-and-test.sh "$coin"
else
  log "backend deploy skipped: ${backend_reason}"
fi

if [[ "$force_confnew" -eq 1 ]]; then
  ./contrib/scripts/deploy-blockbook-local.sh "$coin" --force-confnew
else
  ./contrib/scripts/deploy-blockbook-local.sh "$coin"
fi
