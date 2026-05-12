#!/usr/bin/env bash
# requires `gh auth login` and read access to trezor/blockbook
#
# Pulls BB_* repository variables from trezor/blockbook via gh once, then runs
# the integration suite. Args are forwarded to `go test`; narrow with -run.
#
# Examples:
#   contrib/tests/run-integration-tests.sh
#   contrib/tests/run-integration-tests.sh -run 'TestIntegration/ethereum=main/rpc'
#   contrib/tests/run-integration-tests.sh -run 'TestIntegration/ethereum=main/rpc/GetBlock' -count=1
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"

source "$script_dir/../gh-vars.sh"
bb_export_gh_vars

export BB_BUILD_ENV="${BB_BUILD_ENV:-dev}"

cd "$repo_root"
mapfile -t pkgs < <(go list github.com/trezor/blockbook/tests/...)
[[ ${#pkgs[@]} -gt 0 ]] || { echo "ERROR: 'go list' produced no packages." >&2; exit 1; }
exec go test -v -tags 'integration' "${pkgs[@]}" -run 'TestIntegration' -timeout 30m "$@"
