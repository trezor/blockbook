#!/usr/bin/env bash
# Run Blockbook unit tests directly (no Docker, no gh fetch). Args forwarded to `go test`.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"

mapfile -t pkgs < <(go list ./... | grep -vE '^github\.com/trezor/blockbook/(contrib|tests)')
[[ ${#pkgs[@]} -gt 0 ]] || { echo "ERROR: 'go list' produced no packages." >&2; exit 1; }
exec go test -tags 'unittest' "${pkgs[@]}" "$@"
