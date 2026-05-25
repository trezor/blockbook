#!/usr/bin/env bash
# Validate openapi.yaml, generate a small typed TypeScript client, and smoke it
# against selected deployed Blockbook instances.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
openapi_dir="$repo_root/tests/openapi"

if [[ ! -x "$openapi_dir/node_modules/.bin/redocly" ]]; then
    echo "ERROR: OpenAPI test dependencies are not installed. Run: npm ci --prefix tests/openapi" >&2
    exit 1
fi

mkdir -p "$openapi_dir/.generated"
export NO_UPDATE_NOTIFIER="${NO_UPDATE_NOTIFIER:-1}"
export REDOCLY_TELEMETRY="${REDOCLY_TELEMETRY:-off}"
export REDOCLY_SUPPRESS_UPDATE_NOTICE="${REDOCLY_SUPPRESS_UPDATE_NOTICE:-true}"

npm --prefix "$openapi_dir" run lint:spec
npm --prefix "$openapi_dir" run generate
npm --prefix "$openapi_dir" run typecheck

export REPO_ROOT="$repo_root"
npm --prefix "$openapi_dir" run smoke
