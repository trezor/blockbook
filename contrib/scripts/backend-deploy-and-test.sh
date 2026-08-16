#!/usr/bin/env bash
set -euo pipefail

readonly LOG_PREFIX="CI/CD Pipeline:"
readonly SCRIPT_NAME="[backend-deploy]"

log() {
  printf '%s %s %s\n' "$LOG_PREFIX" "$SCRIPT_NAME" "$*" >&2
}

die() {
  printf '%s error: %s\n' "$LOG_PREFIX" "$*" >&2
  exit 1
}

if [ $# -ne 1 ] && [ $# -ne 4 ]
then
    die "usage: $(basename $(readlink -f "$0")) coin service_name coin_test backend_log_file OR $(basename $(readlink -f "$0")) coin"
fi

command -v jq >/dev/null 2>&1 || die "jq is required"

COIN=$1
SERVICE=${2:-}
COIN_TEST=${3:-}
LOGFILE=${4:-}
CONFIG="configs/coins/${COIN}.json"

BACKEND_TIMEOUT="${BACKEND_TIMEOUT:-}"
[ -z "${BACKEND_TIMEOUT}" ] && BACKEND_TIMEOUT=15s
[ -z "${SERVICE}" ] && SERVICE="${COIN}"
[ -z "${COIN_TEST}" ] && COIN_TEST="${COIN}=main"
if [[ -z "${LOGFILE}" ]]; then
  if [[ -f "${CONFIG}" ]]; then
    alias="$(jq -r '.coin.alias // empty' "${CONFIG}")"
    if [[ -n "${alias}" ]]; then
      LOGFILE="${alias}.log"
    else
      LOGFILE=debug.log
    fi
  else
    LOGFILE=debug.log
  fi
fi

log "running: $(basename $(readlink -f "$0")) ${COIN} ${SERVICE} ${COIN_TEST} ${LOGFILE}"

rm -f build/*.deb
log "building backend package for ${COIN}"
make PORTABLE=1 "deb-backend-${COIN}"

shopt -s nullglob
packages=(./build/backend-"${SERVICE}"*.deb)
shopt -u nullglob
if [[ "${#packages[@]}" -eq 0 ]]; then
  die "package not found for backend-${SERVICE}"
fi
PACKAGE="${packages[0]}"

log "installing ${PACKAGE}"
sudo /usr/bin/dpkg -i "${PACKAGE}"
log "restarting backend-${SERVICE}"
sudo /bin/systemctl restart "backend-${SERVICE}"

log "waiting for backend startup for ${BACKEND_TIMEOUT}"
set +e
sudo -u bitcoin /usr/bin/timeout "${BACKEND_TIMEOUT}" /usr/bin/tail -f "/opt/coins/data/${COIN}/backend/${LOGFILE}"
status=$?
set -e
if [[ "$status" -ne 0 && "$status" -ne 124 ]]; then
  if [[ "$status" -eq 1 ]]; then
    log "backend log ${LOGFILE} is not available yet, continuing to integration tests"
  else
    die "backend startup log wait failed with exit code ${status}"
  fi
fi

log "running integration tests: TestIntegration/${COIN_TEST}"
make PORTABLE=1 test-integration ARGS="-v -run=TestIntegration/${COIN_TEST}"
