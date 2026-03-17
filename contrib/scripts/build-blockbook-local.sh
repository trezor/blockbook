#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "Usage: $(basename "$0") <coin-alias> [<coin-alias> ...]" >&2
  exit 1
fi

command -v jq >/dev/null 2>&1 || { echo "error: jq is required" >&2; exit 1; }

coins=("$@")
package_names=()
make_targets=()

for coin in "${coins[@]}"; do
  config="configs/coins/${coin}.json"
  if [[ ! -f "$config" ]]; then
    echo "error: missing coin config $config" >&2
    exit 1
  fi

  package_name="$(jq -r '.blockbook.package_name // empty' "$config")"
  if [[ -z "$package_name" ]]; then
    echo "error: coin '$coin' does not define blockbook.package_name" >&2
    exit 1
  fi

  package_names+=("$package_name")
  make_targets+=("deb-blockbook-${coin}")
  rm -f "build/${package_name}"_*.deb
done

make "${make_targets[@]}" 1>&2

for i in "${!coins[@]}"; do
  coin="${coins[$i]}"
  package_name="${package_names[$i]}"
  package_file="$(ls -1t build/${package_name}_*.deb 2>/dev/null | head -n1 || true)"
  if [[ -z "$package_file" ]]; then
    echo "error: built package for '$coin' was not found (pattern build/${package_name}_*.deb)" >&2
    exit 1
  fi

  echo "built ${coin} via ${package_file}" >&2
  printf '%s\n' "$package_file"
done
