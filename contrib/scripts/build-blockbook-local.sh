#!/usr/bin/env bash
set -euo pipefail

readonly LOG_PREFIX="CI/CD Pipeline:"
readonly SCRIPT_NAME="[build-local]"
readonly DEFAULT_PACKAGE_ROOT="/opt/blockbook-builds"

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

resolve_branch_or_tag() {
  if [[ -n "${BRANCH_OR_TAG:-}" ]]; then
    printf '%s\n' "$BRANCH_OR_TAG"
    return
  fi

  local current_branch
  current_branch="$(git branch --show-current 2>/dev/null || true)"
  if [[ -n "$current_branch" ]]; then
    printf '%s\n' "$current_branch"
    return
  fi

  local current_tag
  current_tag="$(git describe --tags --exact-match 2>/dev/null || true)"
  if [[ -n "$current_tag" ]]; then
    printf '%s\n' "$current_tag"
    return
  fi

  die "BRANCH_OR_TAG is not set and the current checkout is neither a branch nor an exact tag"
}

path_escape_ref() {
  printf '%s\n' "${1//\//-}"
}

branch_or_tag="$(resolve_branch_or_tag)"
branch_or_tag_path="$(path_escape_ref "$branch_or_tag")"
package_root="${BLOCKBOOK_PACKAGE_ROOT:-$DEFAULT_PACKAGE_ROOT}"

if [[ "${package_root:0:1}" != "/" ]]; then
  die "BLOCKBOOK_PACKAGE_ROOT must be an absolute path (got '${package_root}')"
fi

coins=("$@")
package_names=()
make_targets=()

log "requested coins: ${coins[*]}"
log "branch_or_tag=${branch_or_tag} -> path=${branch_or_tag_path}"
log "package_root=${package_root}"

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
  rm -f "${package_root}/${branch_or_tag_path}/${coin}/blockbook.deb"
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

  target_dir="${package_root}/${branch_or_tag_path}/${coin}"
  target_file="${target_dir}/blockbook.deb"
  mkdir -p "$target_dir"
  mv -f "$package_file" "$target_file"

  log "built ${coin} via ${target_file}"
  printf '%s\n' "$target_file"
done
