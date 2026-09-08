#!/usr/bin/env bash
# Long-run driver for the fee audit. Splits the window into hourly segments,
# each writing its own TSV, because the harness holds every block in memory and
# only writes on exit: one 12h process would grow unbounded and lose everything
# if it died, while twelve hourly ones lose at most the segment in flight.
set -euo pipefail

SEGMENTS=${SEGMENTS:-12}
SEG_DUR=${SEG_DUR:-59m}
CHAINS=${CHAINS:-eth,bsc,pol,arb,op,base,avax,hype,rhc}
MIN_QUOTE=${MIN_QUOTE:-15s}
KEY=${KEY:-$HOME/.config/1inch.key}

cd "$(dirname "$0")"
here=$(pwd)
root=${OUT:-$HOME/feeaudit/$(date +%Y%m%d-%H%M%S)}
mkdir -p "$root"

if [ ! -x "$here/feeaudit" ]; then
    echo "building..."
    (cd "$here/../../.." && go build -o "$here/feeaudit" ./contrib/scripts/feeaudit/)
fi

# Forward the interrupt to the running segment instead of dying on it, so the
# harness gets to write the TSV for the hour already collected.
child=""
trap 'echo "stopping after this segment"; [ -n "$child" ] && kill -INT "$child" 2>/dev/null; STOP=1' INT TERM
STOP=0

echo "writing to $root"
echo "$SEGMENTS segments x $SEG_DUR, chains=$CHAINS, min-quote=$MIN_QUOTE"

for i in $(seq -w 1 "$SEGMENTS"); do
    seg="$root/seg-$i"
    echo "[$(date +%H:%M:%S)] segment $i/$SEGMENTS -> $seg"
    "$here/feeaudit" \
        -chains "$CHAINS" \
        -duration "$SEG_DUR" \
        -min-quote "$MIN_QUOTE" \
        -oneinch-key-file "$KEY" \
        -out "$seg" > "$root/seg-$i.log" 2>&1 &
    child=$!
    wait "$child" || echo "  segment $i exited non-zero (see seg-$i.log)"
    child=""
    if [ -f "$seg/feeaudit.tsv" ]; then
        echo "  $(( $(wc -l < "$seg/feeaudit.tsv") - 1 )) samples, $(( $(wc -l < "$seg/blocks.tsv") - 1 )) blocks"
    else
        echo "  no output written"
    fi
    [ "$STOP" = 1 ] && break
done

echo "[$(date +%H:%M:%S)] done. total: $(cat "$root"/seg-*/feeaudit.tsv 2>/dev/null | grep -cv '^chain') samples"
echo "$root"
