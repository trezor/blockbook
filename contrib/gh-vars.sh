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

bb_export_gh_vars() {
    local repo="${BB_GH_REPO:-trezor/blockbook}"

    if ! command -v gh >/dev/null 2>&1; then
        echo "ERROR: gh CLI is required but not installed (see https://cli.github.com/)." >&2
        return 1
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
        export "$normalized=$value"
        count=$((count + 1))
    done <<< "$raw"

    if [[ $count -eq 0 ]]; then
        echo "ERROR: ${repo} returned no variables (check 'gh auth status' and Trezor-org membership)." >&2
        return 1
    fi

    echo "Exported $count BB_* variables from ${repo}." >&2
}
