# feeaudit

Measures the EIP-1559 fee tiers public Blockbook hosts quote against what transactions on
those chains actually paid, and renders the result as the "EVM Gas Price Monitor" page.
Everything runs against public endpoints only: no API key except an optional 1inch one, no
backend access. Background on why the estimate is hard to get right is in
[docs/evm-fees.md](../../../docs/evm-fees.md).

## Pieces

| path | role |
|---|---|
| `main.go` | collector: websocket `estimateFee` quotes anchored to block heights, REST block fetches for the paid prices, offline `eth_feeHistory` reconstruction for the on-chain counterfactual, optional 1inch polling. Writes `feeaudit.tsv` (one row per quote and tier) and `blocks.tsv` (one row per block). |
| `run-long.sh` | driver for a multi-hour capture. Runs hourly segments so a crash loses at most one hour. |
| `../feewait/main.go` | dumps the per-tier `maxWaitTimeEstimate` each host serves. Those readings are the `WAIT_MS` constants in `dashboard/build_data.py`, so re-read them when the provider config changes. |
| `dashboard/build_data.py` | reduces the TSVs to the chart payload: bucketed series, congestion windows, ETA hit rates, and a replay of the post-PR-1768 tier spec over the recorded reward percentiles (the "on-chain #1768" column). |
| `dashboard/head.part`, `body.part`, `script.part` | the static page, split around the spot where the payload is spliced in. |
| `dashboard/build-dashboard.sh` | merges segments, runs the extractor, assembles the HTML. |

## Reproduce the 12h dashboard

The capture behind the published page is `run12h/` at the repository root (twelve segments,
2026-09-04 to 2026-09-05, nine chains). To rebuild the page from it:

```sh
contrib/scripts/feeaudit/dashboard/build-dashboard.sh run12h evm-gas-monitor.html
```

The output is byte-for-byte the page that was published as the artifact, apart from JSON
serialisation whitespace.

## Run a new capture

```sh
# one-off, 30 minutes, two chains
go run contrib/scripts/feeaudit/main.go -chains eth,pol -duration 30m \
    -oneinch-key-file ~/.config/1inch.key -out /path/to/capture

# long run: 12 x 59 min over all chains, TSVs under ~/feeaudit/<timestamp>/seg-NN/
KEY=~/.config/1inch.key contrib/scripts/feeaudit/run-long.sh

# then
contrib/scripts/feeaudit/dashboard/build-dashboard.sh ~/feeaudit/<timestamp> out.html
```

Before trusting a run on a new chain, check the offline fee-history reconstruction against
a real node once (verified wei-for-wei on coreth and op-reth):

```sh
go run contrib/scripts/feeaudit/main.go -chains avax -validate https://api.avax.network/ext/bc/C/rpc
```

## Things that are hard-coded in `build_data.py`

- `USD` spot prices per native coin, taken on 2026-09-07. Only affect the money columns.
- `WAIT_MS` per-chain ETA targets, read live with `feewait` on 2026-09-07. Only Infura
  populates them; every other source is scored against Suite's own block fallback.
- `SUITE_BLOCKTIME` mirrors Suite's connect-data block times, which decide how many blocks
  an ETA in milliseconds translates to.
- `NEW_SPEC` mirrors `eip1559TierSpec` in `bchain/coins/eth/ethrpc.go`; update it if the
  estimator changes again, otherwise the replayed column describes a Blockbook that no longer
  exists.
