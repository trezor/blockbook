# Agent instructions

## Testing

Test your changes before declaring done. Start with unit tests :
```
contrib/tests/run-unit-tests.sh [go args]
```
Then connectivity tests :
```
contrib/tests/run-integration-tests.sh -run 'TestIntegration/.*/connectivity' # make sure we can reach backends
```
Then continue with integration tests, but start narrow, broaden only when needed :
```
contrib/tests/run-integration-tests.sh -run 'TestIntegration/ethereum=main/rpc/GetBlock'
contrib/tests/run-integration-tests.sh -run 'TestIntegration/ethereum=main/rpc'
contrib/tests/run-integration-tests.sh -run 'TestIntegration/ethereum=main'
```
Test path convention :
```
TestIntegration/<coin>=main|test[#NN]/<connectivity|rpc|sync|api>/<subtest>[/<sub>]
```

- Avoid bitcoin during iteration : `bitcoin=main`'s `MempoolSync` and `GetTransactionForMempool` walk mainnet's
mempool and are slow, prefer `bcash`.
- Prefer `ethereum`, `bsc`, `tron`, and `avalanche` from the EVM family.
- The coin segment comes from a `tests/tests.json` key: keys containing
`_testnet` map to `<prefix>=test`, the rest to `<key>=main`
(`bitcoin_regtest` → `bitcoin_regtest=main`). Collisions are disambiguated
with `#01`, `#02`, ... — today `bitcoin=test` is testnet, `bitcoin=test#01`
is testnet4.
- in case of unexpected integration test failures, you can run `contrib/scripts/blockbook_status.sh` or `contrib/scripts/backend_status.sh`
scripts to check health of particular blockbook/backend instance
- `contrib/gh-vars.sh` has a `_BB_GH_CACHE_VERSION` variable that must be bumped when the cache file format changes (e.g. the schema header or the structure of the exported env file) to invalidate stale caches
- `contrib/tests/run-all-tests.sh` is for CI/CD only — too slow for an agent feedback loop, do not run it.
- `-count=1` bypasses the test cache. Use it when you suspect stale results: `go test`
  fingerprints test binary + args, but it does NOT notice when GitHub Actions repository
  variables (the URLs/credentials your tests dial) change between runs, so a previously
  cached PASS can mask a real failure.

## Profiling

Profiling is enabled only on Dev blockbooks. When troubleshooting performance, slow sync,
large mempool handling, stuck goroutines, or suspected deadlocks, use:
```
contrib/scripts/blockbook_profile.sh <coin> [--profile cpu|heap|goroutine|allocs|threadcreate]
```

The script loads Dev Blockbook URLs via `contrib/gh-vars.sh`, derives the pprof port from
the coin config, prints a compact sync/metrics snapshot, downloads the selected pprof
profile, and runs `go tool pprof -top`. Start with CPU for throughput issues and
`--profile goroutine` for deadlock/stall investigations.

## Metrics

Prometheus metrics and the Grafana dashboard share one source of truth, `configs/metrics.yaml`.

- **Add a metric:** add an entry to `configs/metrics.yaml` (stable key + `name`/`type`/`help`;
  `labels` for `*_vec`, `buckets` for histograms), then a `Metrics` field in `common/metrics.go` tagged `metric:"<key>"`.
- **Add a panel:** add the viz skeleton (type/`fieldConfig`/`options`, new `id` + a semantic
  `x-panel-key`, and an `x-query-key` per target — no `gridPos` or `datasource`) to
  `configs/grafana/template.json`, then its `title`/`description`/`queries` under that `x-panel-key`
  in `configs/grafana/panels.yaml` (queries keyed by `x-query-key`, each with `promql`/`legend`;
  write metric names as `{{name:<key>}}`). Panels pack left-to-right in `template.json` order at 8×8;
  set `width`/`height` in the panels.yaml entry to override.
- Prefer stable panel keys like `<section>.<subject>[_stat]` (for example `rpc.request_duration_p95`)
  and query keys that name the plotted series (`requests`, `errors`, `p95`, `total`, `threshold`).
- After any of these, run `python3 contrib/scripts/render_grafana.py` (CI gates with `--check`).

## Facts to keep in mind to avoid regressions and waste

- Blockbook instance should be able to : 
 - handle at least 20 000 websocket connections from trezor suite
 - index and catchup with fast L2 chains like Arbitrum or Base 
- ignore `tests/openapi/node_modules/` and `tests/openapi/package-lock.json` when searching the codebase
