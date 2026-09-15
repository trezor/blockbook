#!/usr/bin/env bash
# Rebuilds the "EVM Gas Price Monitor" page from a feeaudit capture. The run-long.sh
# driver writes one directory per hourly segment; this merges them, reduces the TSVs
# to the chart payload and splices it between the static page fragments.
#
#   dashboard/build-dashboard.sh <capture-root> [out.html]
#
# <capture-root> is either a run-long.sh output root (seg-*/ inside) or a single
# feeaudit -out directory holding feeaudit.tsv and blocks.tsv directly.
set -euo pipefail

root=${1:?capture root required}
out=${2:-evm-gas-monitor.html}
here=$(cd "$(dirname "$0")" && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/feeaudit-dash.XXXXXX")
trap 'rm -rf "$work"' EXIT

if [ -f "$root/feeaudit.tsv" ]; then
    samples="$root/feeaudit.tsv"; blocks="$root/blocks.tsv"
else
    # Segment files each carry a header; keep the first only.
    for f in feeaudit blocks; do
        head -1 "$(ls "$root"/seg-*/$f.tsv | head -1)" > "$work/$f.tsv"
        cat "$root"/seg-*/$f.tsv | grep -v '^chain' >> "$work/$f.tsv"
    done
    samples="$work/feeaudit.tsv"; blocks="$work/blocks.tsv"
fi

python3 "$here/build_data.py" "$samples" "$blocks" "$work/data.json"

# "</" inside a JSON string would close the script element early.
python3 - "$here" "$work/data.json" "$out" <<'PY'
import sys
here, data, out = sys.argv[1:]
payload = open(data).read().replace('</', '<\\/')
with open(out, 'w') as f:
    f.write(open(f'{here}/head.part').read() + '\n' + open(f'{here}/body.part').read()
            + '\n<script>\nconst DATA = ' + payload + ';\n</script>\n'
            + open(f'{here}/script.part').read() + '\n')
PY
echo "wrote $out ($(wc -c < "$out") bytes)"
