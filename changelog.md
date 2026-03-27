# Changelog

## Unreleased

### Performance and Scalability

- **Concurrent Ethereum block processing path** ([#1383](https://github.com/trezor/blockbook/pull/1383)): runs event processing and internal data fetch in parallel; timeout/cancel control moved to caller for earlier aborts, reducing latency and wasted RPC work on failures.
- **Single-pass BTC block JSON parsing** ([#1385](https://github.com/trezor/blockbook/pull/1385)): removes duplicate block unmarshalling by parsing header and transactions in one pass, lowering ingestion overhead.
- **Batched ERC20 balance RPC calls** ([#1388](https://github.com/trezor/blockbook/pull/1388)): replaces per-contract `eth_call` with JSON-RPC batching for fungible token balances, adds configurable batch size and safe fallback for unsupported backends.
- **Dual HTTP/WS RPC client model for EVM chains** ([#1400](https://github.com/trezor/blockbook/pull/1400)): splits transport usage to HTTP for calls and WS for subscriptions.
- **Faster BTC mempool resync with batching + outpoint cache** ([#1403](https://github.com/trezor/blockbook/pull/1403)): adds optional batch tx fetch with bounded concurrency and a temporary resync outpoint cache to avoid repeated parent lookups.
- **Ethereum address-contract indexing micro-optimizations** ([#1417](https://github.com/trezor/blockbook/pull/1417)): adds hot-address LRU/index map to remove O(n) contract scans, bounds cache growth, and improves ERC20 aggregation hot-path overhead.
- **WebSocket/API perf + fiat observability improvements** ([#1423](https://github.com/trezor/blockbook/pull/1423)): batches fiat enrichment with robust fallback reasons, improves client-facing error behavior, and reduces log noise.

### Reliability and Correctness

- **UTXO reorg detection fix in raw-parse path** ([#1398](https://github.com/trezor/blockbook/pull/1398)): populates `BlockHeader.Prev` for raw-parsed blocks to prevent missed fork detection that can stall sync on wrong tips.
- **Base newHeads burst handling fix** ([#1407](https://github.com/trezor/blockbook/pull/1407)): coalesces head notifications as hints and enforces strictly increasing block-number processing with a catch-up loop.
- **Reliable SIGTERM shutdown + clean RocksDB close** ([#1408](https://github.com/trezor/blockbook/pull/1408)): reworks signal fan-out so main shutdown always runs, unblocks workers, and stops periodic state writes during shutdown.
- **Resync recovery on errors** ([#1409](https://github.com/trezor/blockbook/pull/1409)): detects errors in parallel/bulk sync and triggers controlled resync restarts on rollback/reorg to avoid infinite retry stalls.
- **Fixed scientific notation parsing error** ([#1429](https://github.com/trezor/blockbook/pull/1429)): `AmountToBigInt` now safely handles scientific notation (`e`/`E`), keeps a fast path for plain decimals, and rejects pathological exponent expansion.

### Configuration and Deployment

- **Configurable backend RPC endpoints for builds/tests** ([#1392](https://github.com/trezor/blockbook/pull/1392)): adds per-coin `BB_RPC_URL_*` overrides for non-local backends, `BB_RPC_BIND_HOST_*`/`BB_RPC_ALLOW_IP_*` for safer network exposure, plus `rpc_url_ws_template` and `BB_RPC_URL_WS_*` overrides.

### Observability

- **Syncing/caching Prometheus metrics** ([#1420](https://github.com/trezor/blockbook/pull/1420)): introduces many new metrics for syncing throughput and cache behavior.
- **WebSocket/API perf + fiat observability improvements** ([#1423](https://github.com/trezor/blockbook/pull/1423)): adds Prometheus metrics for fiat enrichment and API behavior.

### Fiat Pipeline

- **Fiat worker refactor + broader tests** ([#1424](https://github.com/trezor/blockbook/pull/1424)): extracts fiat logic from a large worker module, improves historical-fetch handling and deadline retry paths, and expands HTTP/WS fiat test coverage.

### Testing

- **API-level E2E suite + deploy workflow** ([#1426](https://github.com/trezor/blockbook/pull/1426)): adds E2E tests against live Blockbook endpoints plus GitHub Actions build/deploy stages that wait for sync and run filtered E2E validation after deploy.

### Security

- **Potential DoS fix for oversized pagination inputs** ([#1363](https://github.com/trezor/blockbook/pull/1363)): validates extreme `page` and `pageSize` values to prevent resource-exhaustion requests.
- **Security hardening: CSP + XSS fixes in templates** ([#1397](https://github.com/trezor/blockbook/pull/1397)): adds CSP headers and fixes XSS vulnerabilities in templates.
- **WebSocket origin allowlist** ([#1421](https://github.com/trezor/blockbook/pull/1421)): adds optional origin checks with explicit logging to reduce cross-origin websocket exposure when not protected by a proxy.
- **Request-size and template hardening** ([#1434](https://github.com/trezor/blockbook/pull/1434)): limits `/api/sendtx` body size, rejects oversized websocket messages, and avoids `template.JSStr`.

### New Features and Chain Support

- **ENS resolver support** ([#1289](https://github.com/trezor/blockbook/pull/1289)).
- **Zcash upgrade** ([#1402](https://github.com/trezor/blockbook/pull/1402)).
- **Tron network support** ([#1273](https://github.com/trezor/blockbook/pull/1273)): adds Tron support to Blockbook.
- **Opt-in ERC-4626 vault enrichment for EVM tokens** ([#1431](https://github.com/trezor/blockbook/pull/1431)): adds REST/WS `includeErc4626`enabled batched vault detection and response enrichment with erc4626 data.

### Backend Compatibility

- **Adjusted ZebraRPC for new zebrad backend version** ([#1377](https://github.com/trezor/blockbook/pull/1377)).
