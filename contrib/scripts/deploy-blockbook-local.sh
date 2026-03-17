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

package_file="$(./contrib/scripts/build-blockbook-local.sh "$coin")"
package_path="$(readlink -f "$package_file")"

sudo DEBIAN_FRONTEND=noninteractive apt install -y --reinstall "$package_path"
sudo systemctl restart "${package_name}.service"
sudo systemctl is-active --quiet "${package_name}.service"

echo "deployed ${coin} via ${package_path}"
