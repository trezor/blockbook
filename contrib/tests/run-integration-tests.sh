#!/usr/bin/env bash
# requires `gh auth login` and read access to trezor/blockbook
#
# Pulls BB_* repository variables from trezor/blockbook via gh once, then runs
# the integration suite. Args are forwarded to `go test`; narrow with -run.
#
# By default the connectivity tests check only Blockbook reachability; the raw
# backend/node RPC checks need direct node access (only routable from CI) and
# are skipped locally. Pass --backend-connectivity (or export
# BB_TEST_BACKEND_CONNECTIVITY=1) to run them too.
#
# Examples:
#   contrib/tests/run-integration-tests.sh
#   contrib/tests/run-integration-tests.sh -run 'TestIntegration/ethereum=main/rpc'
#   contrib/tests/run-integration-tests.sh -run 'TestIntegration/ethereum=main/rpc/GetBlock' -count=1
#   contrib/tests/run-integration-tests.sh --backend-connectivity -run 'TestIntegration/.*/connectivity'
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"

# Strip the local-only --backend-connectivity flag before forwarding to `go test`.
go_args=()
for arg in "$@"; do
    case "$arg" in
        --backend-connectivity)
            export BB_TEST_BACKEND_CONNECTIVITY=1
            ;;
        *)
            go_args+=("$arg")
            ;;
    esac
done

"$repo_root/contrib/system-check.sh"

source "$script_dir/../gh-vars.sh"
bb_export_gh_vars

export BB_BUILD_ENV="${BB_BUILD_ENV:-dev}"

cd "$repo_root"
mapfile -t pkgs < <(go list github.com/trezor/blockbook/tests/...)
[[ ${#pkgs[@]} -gt 0 ]] || { echo "ERROR: 'go list' produced no packages." >&2; exit 1; }
exec go test -v -tags 'integration' "${pkgs[@]}" -run 'TestIntegration' -timeout 30m ${go_args[@]+"${go_args[@]}"}
