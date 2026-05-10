#!/usr/bin/env bash
# Run unit + integration suites sequentially. CI/CD use only — too slow for agent loops.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

"$script_dir/run-unit-tests.sh" "$@"
"$script_dir/run-integration-tests.sh" "$@"
