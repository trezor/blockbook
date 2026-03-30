#!/usr/bin/env bash
set -euo pipefail

readonly LOG_PREFIX="CI/CD Pipeline:"
readonly SCRIPT_NAME="[deploy-local]"

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

command -v jq >/dev/null 2>&1 || die "jq is required"

package_name="$(jq -r '.blockbook.package_name // empty' "$config")"
if [[ -z "$package_name" ]]; then
  die "coin '$coin' does not define blockbook.package_name"
fi

log "coin=${coin}, package_name=${package_name}"
log "building package"
package_file="$(./contrib/scripts/build-blockbook-local.sh "$coin" | tail -n1)"
if [[ -z "$package_file" ]]; then
  die "build helper did not return a package path for '$coin'"
fi

package_path="$(readlink -f "$package_file")"
service_name="${package_name}.service"
log "resolved package path: ${package_path}"
log "target service: ${service_name}"

show_service_diagnostics() {
  sudo systemctl status --no-pager --full "$service_name" || true
  sudo journalctl -u "$service_name" -n 100 --no-pager || true
}

log "installing ${package_path}"
dpkg_install_cmd=(
  sudo DEBIAN_FRONTEND=noninteractive dpkg -i
)

if [[ "$force_confnew" -eq 1 ]]; then
  dpkg_install_cmd=(sudo DEBIAN_FRONTEND=noninteractive dpkg --force-confnew -i)
fi

dpkg_install_cmd+=("$package_path")
"${dpkg_install_cmd[@]}"

log "restarting ${service_name}"
if ! sudo systemctl restart "$service_name"; then
  show_service_diagnostics
  die "failed to restart ${service_name}"
fi

log "waiting for ${service_name} to become active"
for attempt in $(seq 1 30); do
  if sudo systemctl is-active --quiet "$service_name"; then
    log "service became active on attempt ${attempt}"
    log "deployed ${coin} via ${package_path}"
    exit 0
  fi
  log "service not active yet (attempt ${attempt}/30)"
  sleep 1
done

show_service_diagnostics
die "${service_name} did not become active within 30 seconds"
