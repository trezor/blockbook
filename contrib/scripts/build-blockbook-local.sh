#!/usr/bin/env bash
set -euo pipefail

readonly LOG_PREFIX="CI/CD Pipeline:"
readonly SCRIPT_NAME="[build-local]"

log() {
  printf '%s %s %s\n' "$LOG_PREFIX" "$SCRIPT_NAME" "$*" >&2
}

die() {
  printf '%s error: %s\n' "$LOG_PREFIX" "$*" >&2
  exit 1
}

if [[ $# -lt 1 ]]; then
  die "usage: $(basename "$0") <coin-alias> [<coin-alias> ...]"
fi

command -v jq >/dev/null 2>&1 || die "jq is required"

coins=("$@")
package_names=()
make_targets=()

log "requested coins: ${coins[*]}"

for coin in "${coins[@]}"; do
  config="configs/coins/${coin}.json"
  if [[ ! -f "$config" ]]; then
    die "missing coin config $config"
  fi

  package_name="$(jq -r '.blockbook.package_name // empty' "$config")"
  if [[ -z "$package_name" ]]; then
    die "coin '$coin' does not define blockbook.package_name"
  fi

  package_names+=("$package_name")
  make_targets+=("deb-blockbook-${coin}")
  log "validated ${coin}: package_name=${package_name}, target=deb-blockbook-${coin}"
  log "removing previous packages matching build/${package_name}_*.deb"
  rm -f "build/${package_name}"_*.deb
done

log "starting build: make ${make_targets[*]}"
make "${make_targets[@]}" 1>&2
log "build finished"

for i in "${!coins[@]}"; do
  coin="${coins[$i]}"
  package_name="${package_names[$i]}"
  package_file="$(ls -1t build/${package_name}_*.deb 2>/dev/null | head -n1 || true)"
  if [[ -z "$package_file" ]]; then
    die "built package for '$coin' was not found (pattern build/${package_name}_*.deb)"
  fi

  log "built ${coin} via ${package_file}"
  printf '%s\n' "$package_file"
done
