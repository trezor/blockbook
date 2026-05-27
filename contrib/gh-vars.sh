#!/usr/bin/env bash
# Requires bash 4+ (uses `mapfile` and `${var,,}`).
#
# Sourced helper. Fetches Blockbook BB_* repository variables from GitHub via
# the gh CLI and exports them into the current shell, applying the same
# prefix-suffix normalisation used by .github/actions/export-env-vars so that
# locally running tests see the exact same environment as CI.
#
# Trezor-internal: this requires read access to the private trezor/blockbook
# repository's Actions variables. Authenticate with `gh auth login` using a
# token that has `repo` (or `actions:read`) scope, and make sure your GitHub
# account is a member of the Trezor organisation with access to the repo.
# Override the source repo with BB_GH_REPO if you fork under a different org.
#
# Cache: exports are persisted to ${XDG_CACHE_HOME:-$HOME/.cache}/blockbook/
# with a 60-minute TTL (chmod 600 — values include QuickNode endpoint paths).
# Force a fresh fetch with BB_GH_REFRESH=1. Override the TTL with
# BB_GH_CACHE_TTL=<seconds>.

# Keep in sync with .github/actions/export-env-vars/action.yml.
_bb_gh_prefixes=(
    BB_DEV_RPC_URL_HTTP_
    BB_DEV_RPC_URL_WS_
    BB_DEV_MQ_URL_
    BB_PROD_RPC_URL_HTTP_
    BB_PROD_RPC_URL_WS_
    BB_PROD_MQ_URL_
    BB_RPC_BIND_HOST_
    BB_RPC_ALLOW_IP_
    BB_DEV_API_URL_HTTP_
    BB_DEV_API_URL_WS_
)

# Bump if the cache file format changes; older caches are then ignored.
_BB_GH_CACHE_VERSION=1

_bb_mtime() { stat -c %Y "$1" 2>/dev/null || stat -f %m "$1" 2>/dev/null; }

bb_export_gh_vars() {
    local repo="${BB_GH_REPO:-trezor/blockbook}"
    local ttl="${BB_GH_CACHE_TTL:-3600}"
    local refresh="${BB_GH_REFRESH:-0}"
    local cache_dir="${XDG_CACHE_HOME:-$HOME/.cache}/blockbook"
    local cache_file="${cache_dir}/gh-vars-${repo//\//-}.env"
    local schema_header="# bb-gh-vars schema ${_BB_GH_CACHE_VERSION}"

    if ! command -v gh >/dev/null 2>&1; then
        echo "ERROR: gh CLI is required but not installed (see https://cli.github.com/)." >&2
        return 1
    fi

    if [[ "$refresh" != "1" && -r "$cache_file" ]]; then
        local mtime now age header
        mtime=$(_bb_mtime "$cache_file") || mtime=0
        now=$(date +%s)
        age=$((now - mtime))
        if (( age < ttl )); then
            IFS= read -r header < "$cache_file" || header=""
            if [[ "$header" == "$schema_header" ]]; then
                # shellcheck disable=SC1090
                source "$cache_file"
                echo "Loaded BB_* variables from cache (${age}s old, ${cache_file}). Refresh with BB_GH_REFRESH=1." >&2
                return 0
            fi
        fi
    fi

    if ! gh auth status >/dev/null 2>&1; then
        cat >&2 <<EOF
ERROR: gh CLI is not authenticated.

These tests pull GitHub Actions repository variables from ${repo}, which is
Trezor-internal. Authenticate first:

    gh auth login

Use a token with 'repo' or 'actions:read' scope, and make sure your GitHub
account has read access to ${repo}.
EOF
        return 1
    fi

    # Assumes variable values contain no literal tab/newline — @tsv would escape
    # them and the bash `read -r name value` below would split incorrectly.
    local raw
    if ! raw=$(gh api "/repos/${repo}/actions/variables" --paginate \
                   --jq '.variables[] | [.name, .value] | @tsv' 2>&1); then
        cat >&2 <<EOF
ERROR: failed to fetch repository variables from ${repo}.

gh said:
${raw}

Likely causes:
  * Your GitHub account is not a member of the Trezor organisation, or it
    lacks read access to ${repo} (ask in #blockbook to be added).
  * Your gh token is missing required scopes — refresh with:
        gh auth refresh -s repo,read:org
EOF
        return 1
    fi

    mkdir -p "$cache_dir"
    local tmp="${cache_file}.tmp.$$"
    (umask 077; : > "$tmp") || { echo "ERROR: cannot create cache temp file ${tmp}" >&2; return 1; }
    printf '%s\n' "$schema_header" >> "$tmp"

    local count=0 name value prefix suffix normalized
    while IFS=$'\t' read -r name value; do
        [[ -z "$name" ]] && continue
        normalized="$name"
        for prefix in "${_bb_gh_prefixes[@]}"; do
            if [[ "$name" == "$prefix"* ]]; then
                suffix="${name#"$prefix"}"
                suffix="${suffix//-/_}"
                normalized="${prefix}${suffix,,}"
                break
            fi
        done
        printf 'export %s=%q\n' "$normalized" "$value" >> "$tmp"
        count=$((count + 1))
    done <<< "$raw"

    if [[ $count -eq 0 ]]; then
        rm -f "$tmp"
        echo "ERROR: ${repo} returned no variables (check 'gh auth status' and Trezor-org membership)." >&2
        return 1
    fi

    mv "$tmp" "$cache_file"
    # shellcheck disable=SC1090
    source "$cache_file"
    echo "Fetched $count BB_* variables from ${repo}, cached at ${cache_file}." >&2
}
