#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "Usage: $(basename "$0") <coin-alias>" >&2
  exit 1
fi

coin="$1"
config="configs/coins/${coin}.json"

if [[ ! -f "$config" ]]; then
  echo "error: missing coin config $config" >&2
  exit 1
fi

command -v jq >/dev/null 2>&1 || { echo "error: jq is required" >&2; exit 1; }

package_name="$(jq -r '.blockbook.package_name // empty' "$config")"
if [[ -z "$package_name" ]]; then
  echo "error: coin '$coin' does not define blockbook.package_name" >&2
  exit 1
fi

package_file="$(./contrib/scripts/build-blockbook-local.sh "$coin" | tail -n1)"
if [[ -z "$package_file" ]]; then
  echo "error: build helper did not return a package path for '$coin'" >&2
  exit 1
fi

package_path="$(readlink -f "$package_file")"
service_name="${package_name}.service"

show_service_diagnostics() {
  sudo systemctl status --no-pager --full "$service_name" || true
  sudo journalctl -u "$service_name" -n 100 --no-pager || true
}

echo "installing ${package_path}"
sudo DEBIAN_FRONTEND=noninteractive apt install -y --reinstall "$package_path"

echo "restarting ${service_name}"
if ! sudo systemctl restart "$service_name"; then
  echo "error: failed to restart ${service_name}" >&2
  show_service_diagnostics
  exit 1
fi

echo "waiting for ${service_name} to become active"
for _ in $(seq 1 30); do
  if sudo systemctl is-active --quiet "$service_name"; then
    echo "deployed ${coin} via ${package_path}"
    exit 0
  fi
  sleep 1
done

echo "error: ${service_name} did not become active within 30 seconds" >&2
show_service_diagnostics
exit 1
