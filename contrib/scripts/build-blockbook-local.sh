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

rm -f build/${package_name}_*.deb
make "deb-blockbook-${coin}"

package_file="$(ls -1t build/${package_name}_*.deb 2>/dev/null | head -n1 || true)"
if [[ -z "$package_file" ]]; then
  echo "error: built package for '$coin' was not found (pattern build/${package_name}_*.deb)" >&2
  exit 1
fi

echo "built ${coin} via ${package_file}" >&2
printf '%s\n' "$package_file"
