#!/usr/bin/env bash
#
# reset_eth_token_rates.sh
#
# Clears all historical fiat-rate data from a running Ethereum Archive Blockbook instance
# so that Blockbook re-fetches it from the configured provider after restart.
#
# What gets deleted:
#   - Special tickers in the `default` column family:
#       CurrentTickers, HourlyTickers, FiveMinutesTickers
#   - Historical bootstrap markers in the `default` column family:
#       HistoricalFiatBootstrapComplete, HistoricalFiatBootstrapAttempts
#   - All entries in the `fiatRates` column family (14-byte ASCII timestamps
#     keyed as YYYYMMDDhhmmss).
#
# A full rsync backup of the DB directory is taken before any delete so the
# operation is reversible by restoring the backup.
#
# After Blockbook restarts, historical bootstrap runs on the very next
# downloader cycle as long as UTC hour is > 0 (fiat/fiat_rates.go:479,540 —
# lastHistoricalTickers starts at Go's zero time, so the day-difference check
# is trivially true on first run). If you restart at, say, 00:30 UTC, bootstrap
# waits until 01:00 UTC. Current / hourly / 5-minute tickers refresh on the
# normal schedule regardless.
#
# Usage:
#   sudo ./reset_eth_token_rates.sh [--dry-run] [--yes] [--skip-backup]
#
# Env overrides (defaults in parentheses):
#   DB       (/opt/coins/data/ethereum_archive/blockbook/db)
#   LDB      (/opt/coins/blockbook/ethereum_archive/bin/ldb)
#   CFG      (/opt/coins/blockbook/ethereum_archive/config/blockchaincfg.json)
#   SERVICE  (blockbook-ethereum-archive)
#   BB_USER  (blockbook-ethereum) — user that owns the DB / runs the service
#   SKIP_CFG_CHECK (0)            — set to 1 to bypass the platformVsCurrency guard

set -euo pipefail

DB="${DB:-/opt/coins/data/ethereum_archive/blockbook/db}"
LDB="${LDB:-/opt/coins/blockbook/ethereum_archive/bin/ldb}"
CFG="${CFG:-/opt/coins/blockbook/ethereum_archive/config/blockchaincfg.json}"
SERVICE="${SERVICE:-blockbook-ethereum-archive}"
BB_USER="${BB_USER:-blockbook-ethereum}"
SKIP_CFG_CHECK="${SKIP_CFG_CHECK:-0}"

DRY_RUN=0
ASSUME_YES=0
SKIP_BACKUP=0
for arg in "$@"; do
    case "$arg" in
        --dry-run)     DRY_RUN=1 ;;
        --yes|-y)      ASSUME_YES=1 ;;
        --skip-backup) SKIP_BACKUP=1 ;;
        -h|--help)
            sed -n '2,30p' "$0"
            exit 0
            ;;
        *) echo "unknown arg: $arg" >&2; exit 2 ;;
    esac
done

log() { printf '[%s] %s\n' "$(date +%H:%M:%S)" "$*"; }

run() {
    if [ "$DRY_RUN" -eq 1 ]; then
        printf '  DRY-RUN: %s\n' "$*"
    else
        eval "$@"
    fi
}

# ldb must run as the blockbook user so RocksDB-owned files (LOG, LOCK,
# *.sst, *.log) stay owned by that user; otherwise blockbook cannot reopen
# the DB after restart.
ldb_as_bb() {
    if [ "$DRY_RUN" -eq 1 ]; then
        printf '  DRY-RUN: sudo -u %s %s --db=%s %s\n' "$BB_USER" "$LDB" "$DB" "$*"
    else
        sudo -u "$BB_USER" "$LDB" --db="$DB" "$@"
    fi
}

# --- sanity checks ------------------------------------------------------------

[ "$(id -u)" -eq 0 ] || { echo "must run as root (uses systemctl + sudo -u)"; exit 1; }
[ -d "$DB" ]  || { echo "DB dir not found: $DB"; exit 1; }
[ -x "$LDB" ] || { echo "ldb not found / not executable: $LDB"; exit 1; }
id -u "$BB_USER" >/dev/null 2>&1 || { echo "user not found: $BB_USER"; exit 1; }

log "DB          = $DB"
log "LDB         = $LDB"
log "CFG         = $CFG"
log "SERVICE     = $SERVICE"
log "BB_USER     = $BB_USER"
log "DRY_RUN     = $DRY_RUN"
log "SKIP_BACKUP = $SKIP_BACKUP"

# --- config guard -------------------------------------------------------------
#
# The deployed blockchaincfg.json is what the running blockbook actually uses.
# If platformVsCurrency is still the old (broken) value, wiping fiat history
# and restarting will just rebuild the same broken state. Fail fast here so
# the operator fixes the config first.
#
# fiat_rates_params is serialized as an ESCAPED JSON STRING inside the outer
# JSON (common/config.go:18 — FiatRatesParams is `string`, not nested object).
# The deployed file literally contains text like:
#   "fiat_rates_params":"{\"platformVsCurrency\": \"usd\", ...}"
# Prefer jq to parse the inner JSON; fall back to a regex on the escaped form.
if [ "$SKIP_CFG_CHECK" -ne 1 ]; then
    [ -r "$CFG" ] || { echo "config not readable: $CFG (set SKIP_CFG_CHECK=1 to bypass)"; exit 1; }
    cfg_ok=0
    if command -v jq >/dev/null 2>&1; then
        if jq -er '.fiat_rates_params | fromjson | .platformVsCurrency == "usd"' "$CFG" >/dev/null 2>&1; then
            cfg_ok=1
        fi
    else
        # escaped-JSON-in-JSON: \"platformVsCurrency\": \"usd\"
        if grep -Eq '\\"platformVsCurrency\\"[[:space:]]*:[[:space:]]*\\"usd\\"' "$CFG"; then
            cfg_ok=1
        fi
    fi
    if [ "$cfg_ok" -ne 1 ]; then
        echo "ABORT: $CFG does not have platformVsCurrency=\"usd\" in fiat_rates_params." >&2
        echo "Wiping fiat history before changing the deployed config will just" >&2
        echo "rebuild rates with the old platformVsCurrency value." >&2
        echo "Fix the config (and redeploy if needed), then re-run. Bypass with SKIP_CFG_CHECK=1." >&2
        exit 1
    fi
    log "config guard OK (platformVsCurrency=\"usd\")"
fi

if [ "$ASSUME_YES" -ne 1 ] && [ "$DRY_RUN" -ne 1 ]; then
    if [ "$SKIP_BACKUP" -eq 1 ]; then
        prompt="This will stop $SERVICE and wipe fiat-rate data WITHOUT taking a DB backup. Continue? [y/N] "
    else
        prompt="This will stop $SERVICE and wipe fiat-rate data. Continue? [y/N] "
    fi
    read -r -p "$prompt" ans
    case "$ans" in y|Y|yes|YES) ;; *) echo "aborted"; exit 1 ;; esac
fi

# --- stop blockbook -----------------------------------------------------------

log "stopping $SERVICE"
run "systemctl stop '$SERVICE'"

# --- backup -------------------------------------------------------------------

BACKUP=""
if [ "$SKIP_BACKUP" -eq 1 ]; then
    log "SKIPPING backup (--skip-backup). There will be NO way to restore the DB if this goes wrong."
else
    BACKUP="${DB}.backup-$(date +%F-%H%M%S)"
    log "backing up $DB -> $BACKUP"
    run "rsync -a --delete '$DB/' '$BACKUP/'"
fi

# --- verify the fiatRates column family exists -------------------------------

log "listing column families"
if [ "$DRY_RUN" -eq 0 ]; then
    cf_list="$(sudo -u "$BB_USER" "$LDB" --db="$DB" list_column_families)"
    printf '%s\n' "$cf_list"
    # ldb prints the whole list inside ONE pair of braces, comma-separated, e.g.
    #   Column families in /.../db:
    #   {default, height, ..., fiatRates, ...}
    # (tools/ldb_cmd.cc ListColumnFamiliesCommand::DoCommand). Match with
    # word-boundary so it works regardless of position in that list.
    if ! printf '%s\n' "$cf_list" | grep -qw fiatRates; then
        echo "fiatRates column family not found in $DB — aborting" >&2
        log "restart $SERVICE manually once the cause is understood"
        exit 1
    fi
fi

# --- delete special-ticker + bootstrap keys in the default CF -----------------

for key in CurrentTickers HourlyTickers FiveMinutesTickers \
           HistoricalFiatBootstrapComplete HistoricalFiatBootstrapAttempts; do
    log "delete default:$key"
    ldb_as_bb delete "$key" || log "  (key $key not present, ignoring)"
done

# --- wipe the fiatRates column family ----------------------------------------

# fiatRates keys are 14-byte ASCII timestamps (YYYYMMDDhhmmss). Under RocksDB's
# default bytewise comparator, any such key has first byte in [0x30, 0x39], so
# the 1-byte range [0x00, 0xFF) covers all of them. deleterange's end key is
# exclusive, which is fine here — no real key equals 0xFF.
#
# NOTE: the ldb subcommand is spelled `deleterange` (no underscore) — see
# `ldb --help`. The `--hex` flag REQUIRES the "0x" prefix on keys
# (tools/ldb_cmd.cc HexToString: "Invalid hex input ... Must start with 0x").
log "deleterange on fiatRates CF (all historical rates)"
ldb_as_bb --column_family=fiatRates --hex deleterange 0x00 0xFF

# Optional compaction so the space is actually reclaimed before restart.
log "compact on fiatRates CF"
ldb_as_bb --column_family=fiatRates compact || log "  (compact failed, non-fatal)"

# --- start blockbook ----------------------------------------------------------

log "starting $SERVICE"
run "systemctl start '$SERVICE'"

if [ -n "$BACKUP" ]; then
    log "done. backup kept at: $BACKUP"
else
    log "done. (no backup was taken)"
fi
log "note: historical bootstrap runs on the next downloader cycle as long as"
log "      current UTC hour > 0 (fiat/fiat_rates.go:479,540). If you restarted"
log "      before 01:00 UTC, expect the historical pass at the top of the hour."
log "      Current/hourly/5-min tickers refresh on the normal schedule."
