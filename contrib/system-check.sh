#!/usr/bin/env bash
# Preflight checks for local Blockbook test wrappers.
set -euo pipefail

usage() {
    cat >&2 <<EOF
Usage: $0

Checks local Go/cgo/RocksDB setup before running tests.
Set BB_SKIP_SYSTEM_CHECK=1 to bypass this preflight.
EOF
}

fail() {
    echo "ERROR: $*" >&2
    status=1
}

require_env() {
    local name="$1"
    local example="$2"

    if [[ -z "${!name:-}" ]]; then
        fail "${name} is not exported. Example: export ${name}=\"${example}\""
    fi
}

include_dirs_from_cflags() {
    local previous=""
    local flag

    for flag in "${cflags[@]}"; do
        if [[ "$previous" == "-I" ]]; then
            echo "$flag"
            previous=""
            continue
        fi
        if [[ "$flag" == "-I" ]]; then
            previous="-I"
            continue
        fi
        if [[ "$flag" == -I* ]]; then
            echo "${flag#-I}"
        fi
    done
}

library_dirs_from_ldflags() {
    local previous=""
    local flag

    for flag in "${ldflags[@]}"; do
        if [[ "$previous" == "-L" ]]; then
            echo "$flag"
            previous=""
            continue
        fi
        if [[ "$flag" == "-L" ]]; then
            previous="-L"
            continue
        fi
        if [[ "$flag" == -L* ]]; then
            echo "${flag#-L}"
        fi
    done
}

check_rocksdb_header() {
    local dir

    while IFS= read -r dir; do
        [[ -n "$dir" ]] || continue
        if [[ -r "$dir/rocksdb/c.h" ]]; then
            return 0
        fi
    done < <(include_dirs_from_cflags)

    fail "rocksdb/c.h was not found in any CGO_CFLAGS -I path."
}

check_rocksdb_library() {
    local dir found_lrocksdb=0

    for flag in "${ldflags[@]}"; do
        [[ "$flag" == "-lrocksdb" ]] && found_lrocksdb=1
    done
    if [[ "$found_lrocksdb" -ne 1 ]]; then
        fail "CGO_LDFLAGS must include -lrocksdb."
        return
    fi

    while IFS= read -r dir; do
        [[ -n "$dir" ]] || continue
        if compgen -G "$dir/librocksdb.so*" >/dev/null || [[ -r "$dir/librocksdb.a" ]]; then
            return 0
        fi
    done < <(library_dirs_from_ldflags)

    fail "librocksdb was not found in any CGO_LDFLAGS -L path."
}

status=0

if [[ "${BB_SKIP_SYSTEM_CHECK:-0}" == "1" ]]; then
    echo "WARNING: skipping system checks because BB_SKIP_SYSTEM_CHECK=1." >&2
    exit 0
fi

if [[ $# -ne 0 ]]; then
    usage
    exit 2
fi

if ! command -v go >/dev/null 2>&1; then
    fail "go is required but was not found in PATH."
else
    cgo_enabled="$(go env CGO_ENABLED 2>/dev/null || true)"
    [[ "$cgo_enabled" == "1" ]] || fail "CGO is disabled (go env CGO_ENABLED=${cgo_enabled:-<unset>})."
fi

require_env CGO_CFLAGS "-I/path/to/rocksdb/include"
require_env CGO_LDFLAGS "-L/path/to/rocksdb -lrocksdb -lstdc++ -lm -lz -ldl -lbz2 -lsnappy -llz4 -lzstd"

[[ "$status" -eq 0 ]] || exit "$status"

read -r -a cflags <<< "${CGO_CFLAGS:-}"
read -r -a ldflags <<< "${CGO_LDFLAGS:-}"

check_rocksdb_header
check_rocksdb_library

if [[ "$status" -eq 0 ]]; then
    echo "OK: Go/cgo/RocksDB dependencies are configured." >&2
fi
exit "$status"
