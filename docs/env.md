# Environment variables

Some behavior of Blockbook can be modified by environment variables. The variables usually start with a coin shortcut to allow to run multiple Blockbooks on a single server.

Blockbook reads these from its process environment. When installed from the Debian package, the systemd unit loads them from an optional `EnvironmentFile=-/etc/blockbook/blockbook.env` (one `KEY=value` per line). The file is optional: if it is absent the service starts normally and any variables provided by other means (e.g. systemd `DefaultEnvironment`) still apply. The file is read by the service manager at startup, so it only needs to be readable by root.

-   `BB_ADMIN_USER` / `BB_ADMIN_PASSWORD` - **Global (not coin-prefixed).** HTTP Basic-auth credentials required to reach the internal server's `/admin` endpoints (the administrative pages and the state-mutating POST handlers such as internal-data refetch and contract-info updates). Basic auth is used so the admin pages and forms work directly in a browser via its native login prompt (and `curl -u user:pass` for scripts). The admin surface is **fail-closed**: unless **both** variables are set, every `/admin` route returns `503` and the endpoints are unusable; `/metrics`, the status page (`/`) and static assets are unaffected. A request with missing or wrong credentials gets `401`. Leading/trailing whitespace in either value is ignored, so a stray space or newline in `blockbook.env` will not lock you out. The internal server binds all interfaces by default (`internal_binding_template` is `:<port>`), so set these on every host. Note that the packaged service serves the internal port over HTTPS using the **bundled self-signed certificate** (`cert/blockbook.{crt,key}`, symlinked to the repo's `testcert`), whose private key is public — so that TLS protects the credentials against passive sniffing but not against an active man-in-the-middle. This is acceptable on a trusted, firewalled internal segment (the intended deployment); from a shell you can reach it as e.g. `curl -k -u "$BB_ADMIN_USER:$BB_ADMIN_PASSWORD" https://host:<internal port>/admin/...`. If the internal network is not trusted, terminate real TLS at a reverse proxy and/or restrict the internal port to trusted peers. Do not expose it directly to the internet.

-   `<coin shortcut>_WS_GETACCOUNTINFO_LIMIT` - Limits the number of `getAccountInfo` requests per websocket connection to reduce server abuse. Accepts number as input. Defaults to `42` (matching the Trezor Suite client concurrency limit).

-   `<network>_WS_BALANCE_HISTORY_MAX_TXS` / `<network>_REST_BALANCE_HISTORY_MAX_TXS` - Maximum number of transactions a single balance-history request (for an address or an xpub) may aggregate, set independently for the WebSocket `getBalanceHistory` method and the REST `/api/v2/balancehistory/...` endpoint. Each aggregated transaction costs a database read, so an unbounded request over an address or xpub with a very large history (e.g. an exchange address) is a cheap-to-send, expensive-to-serve request. Past the cap the request is rejected with `400` and a message asking to narrow the `from`/`to` range, rather than returning a truncated (and therefore wrong) history. Accepts a non-negative integer; `0` disables the cap.

    The WebSocket cap defaults to `1000000` and the REST cap to `250000`. The split exists because Trezor Suite talks to Blockbook over WebSocket only, requests the full account history for its balance graph (no `from`/`to` window), and never derives the displayed balance from balance history — so the WS cap is generous enough not to break even a heavy wallet's graph, while still bounding a single expensive message. The REST API is an open, unauthenticated surface, so its cap is tighter. Neither default affects a normal wallet; raise a cap only if you serve balance history for genuinely high-volume addresses on that surface.

-   `<network>_BALANCE_HISTORY_MAX_TXS` - Backward-compatible shared fallback that sets the default for **both** the WS and REST caps above. A transport-specific variable, when set, overrides this for its surface. Accepts a non-negative integer; `0` disables the cap. Unset, the per-surface defaults (`1000000` WS / `250000` REST) apply.

-   `<coin shortcut>_WS_ALLOWED_ORIGINS` - Comma-separated list of allowed WebSocket origins (e.g. `https://example.com`, `http://localhost:3000`). If omitted, all origins are allowed and it is the operator's responsibility to enforce origin access (for example via proxy).

-   `<network>_TRUSTED_PROXIES` - Comma-separated list of trusted proxy CIDRs whose `X-Real-Ip` header should be used as the client IP for public REST API and WebSocket rate limiting.
    Blockbook always trusts `X-Real-Ip` from loopback and RFC1918/private peers, so this variable is only needed for additional non-local proxies. This implicit trust assumes the private network segment in front of Blockbook is not attacker-reachable. Link-local peers (`fe80::/10`) are **not** implicitly trusted — a link-local source address is reachable and forgeable by any node on the same link — so a link-local proxy must be listed explicitly here (e.g. `fe80::1/128`).

    If this variable and its legacy alias are unset, Blockbook keeps the default Cloudflare behavior and uses `CF-Connecting-IP` as the client IP when it contains a valid address. `CF-Connecting-IP` is the only `cf-*` request header Cloudflare always overwrites with the verified visitor IP; `CF-Connecting-IPv6` is forwarded unchanged from the client unless the Cloudflare zone runs "Pseudo IPv4: Overwrite Headers", so it is ignored by default and honored only when `<network>_CLOUDFLARE_PSEUDO_IPV4` is set (see below). Whether `CF-Connecting-IP` is trusted from any peer or only from verified Cloudflare peers is controlled by `<network>_CLOUDFLARE_IPS` (see below).

    If this variable is set, Blockbook switches to generic trusted-proxy mode: `CF-Connecting-IP` and `CF-Connecting-IPv6` are ignored, and `X-Real-Ip` is used only when the TCP peer is a built-in trusted proxy or matches one of the configured CIDRs. In this mode the proxy must overwrite or strip any client-supplied `X-Real-Ip` header before forwarding requests to Blockbook.

    Do not set this variable for a normal Cloudflare-only deployment unless the proxy in front of Blockbook sets `X-Real-Ip` to the real visitor IP. Otherwise all clients may collapse to the proxy or Cloudflare address for rate limiting.

    To avoid unsafe configuration, Blockbook fails startup if a configured prefix is too broad (`/<8` for IPv4, `/<16` for IPv6), malformed, or uses IPv4-mapped IPv6 notation. Use regular IPv4 CIDR notation instead, for example `198.51.100.0/24` rather than `::ffff:198.51.100.0/120`.

    For backwards compatibility, `<network>_WS_TRUSTED_PROXIES` is accepted as a legacy alias when `<network>_TRUSTED_PROXIES` is unset. Prefer the shared variable because the same client attribution is used by REST and WebSocket limiters.

-   `<network>_CLOUDFLARE_IPS` - Controls how the `CF-Connecting-IP` / `CF-Connecting-IPv6` headers are trusted in the default (no `<network>_TRUSTED_PROXIES`) mode. Because those headers are client-settable, they are only meaningful if the origin can prove the connection actually came from Cloudflare.
    - Unset with no legacy alias, or `builtin` (default): Blockbook trusts the `CF-Connecting-*` headers only when the TCP peer is inside Cloudflare's published edge ranges (loaded at startup from `server/cloudflare_ips.txt` compiled into the binary, as of 2026-06) or is a loopback/private proxy fronting Cloudflare. A direct public non-Cloudflare peer cannot spoof a client IP past the per-IP limiter or the IP blocklist.
    - `@/path/to/file`: load the ranges from the given file at startup instead of the compiled-in list (one CIDR per line; blank lines, commas, and `#` comments are allowed). Useful to track Cloudflare's published ranges without rebuilding. An unreadable file or a file with no CIDRs fails startup.
    - A comma-separated CIDR list: use these ranges instead of the built-in list (for example if Cloudflare's ranges drift, or for a custom front-end CDN). Loopback/RFC1918 peers are always also accepted (link-local peers are not — list them explicitly in `<network>_TRUSTED_PROXIES` if a link-local proxy fronts Cloudflare). A value that contains no valid CIDRs fails startup rather than silently disabling verification; only the explicit `off` spellings disable it.
    - `off` (or `none`/`false`/`0`): disable verification and trust `CF-Connecting-*` from any peer (the historical behavior). Only safe when the origin is firewalled to Cloudflare ranges out of band. With verification off, the IP auto-block never acts on a `CF-Connecting-*`-derived address (it would be spoofable), so it only blocks direct TCP peers.

    For backwards compatibility, `<network>_WS_CLOUDFLARE_IPS` is accepted as a legacy alias when `<network>_CLOUDFLARE_IPS` is unset. An explicit shared value, including `off`, does not fall back.

    Note that a loopback/private peer counts as "a proxy fronting Cloudflare": `CF-Connecting-*` headers forwarded by a local reverse proxy are trusted and treated as block-safe. If your deployment is NOT behind Cloudflare and a local proxy forwards client-supplied headers verbatim, strip or overwrite `CF-Connecting-IP` and `CF-Connecting-IPv6` at the proxy; otherwise a client could spoof its attribution (and, with the IP auto-block enabled, get an innocent visitor's address blocked).

    Even with verification on, only `CF-Connecting-IP` is trusted by content. Cloudflare overwrites only that one `cf-*` request header; it forwards every other client-supplied `cf-*` header (including `CF-Connecting-IPv6`) to the origin unchanged, so Blockbook never trusts any other `cf-*` header by content unless explicitly opted in via `<network>_CLOUDFLARE_PSEUDO_IPV4`.

-   `<network>_CLOUDFLARE_PSEUDO_IPV4` - Boolean (default `false`). When `true`, Blockbook honors the `CF-Connecting-IPv6` header as the client IP (preferred over `CF-Connecting-IP`, which in this mode holds a synthetic pseudo-IPv4). Enable this **only** if the Cloudflare zone has "Pseudo IPv4: Overwrite Headers" turned on. In that mode Cloudflare sets (and therefore sanitizes) `CF-Connecting-IPv6`; in every other mode `CF-Connecting-IPv6` is a client-spoofable header and enabling this would let any client forge its attributed IP — getting an innocent visitor rate-limited or blocked, or evading the limiter by rotating the value. Leave unset for normal deployments. As defense-in-depth, operators can also add a Cloudflare Managed Transform / request-header rule that strips an incoming `CF-Connecting-IPv6` at the edge, but the code default is already safe without it.

-   `<network>_WS_MESSAGE_RATE_LIMIT` - Maximum number of messages a single WebSocket connection may send within `<network>_WS_MESSAGE_RATE_WINDOW` before the connection is closed and (when blockable) its client key is added to the IP blocklist. Accepts a non-negative integer; default `2500`, `0` disables the per-connection message rate limit entirely. The default of 2500 messages / 10 minutes is above the maximum burst produced by a Trezor Suite client, so it only trips clearly abusive traffic.

-   `<network>_WS_MESSAGE_RATE_WINDOW` - Trailing sliding window for `<network>_WS_MESSAGE_RATE_LIMIT`, as a Go duration string (e.g. `10m`, `600s`). Default `10m`.

-   `<network>_WS_PENDING_REQUESTS_LIMIT` - Maximum number of requests a single WebSocket connection may have executing concurrently ("pending") before the connection is closed with `pending_requests_limit`. Accepts a non-negative integer; default `48`, `0` disables the cap. The default comfortably covers interactive clients such as Trezor Suite; internal batch clients that pipeline hundreds of requests on one connection (e.g. bulk `getAccountInfo` address scans) need a higher value or `0`. The server-wide work limit still applies as a backstop against unbounded concurrency.

-   `<network>_WS_IP_BLOCK_DURATION` - How long a client key (an IPv4 address, or a full IPv6 `/128` address) is blocked from opening new WebSocket connections after a connection trips the message rate limit, as a Go duration string (e.g. `12h`). Default `12h`; `0` disables IP blocking (an offending connection is still closed). The block key is intentionally narrower than the rate-limit key (which aggregates IPv6 to its `/64`): a hard block keys on the individual `/128` so it cannot deny service to an entire shared `/64`, while the connection rate limiter still aggregates to `/64` so an abuser cannot dodge limits by rotating addresses within an owned `/64`. The blocklist is visible at `/admin/ws-limit-exceeding-ips`, and the `blockbook_websocket_blocked_ips` / `blockbook_websocket_blocked_connections` metrics track it.

    Blocking keys on the same client IP attribution as the per-IP limiter. Loopback/private/link-local addresses and any configured trusted-proxy or Cloudflare edge range are never blocked, so a misconfiguration that collapses many clients onto a shared address cannot block them all. Behind Cloudflare, keep `<network>_CLOUDFLARE_IPS` at its default (or set it to your CDN ranges) so blocks key on the real visitor IP. The default Cloudflare peer verification plus a block-safety guard (a `CF-Connecting-*`-derived address is only blocked when the peer was verified as Cloudflare or is a local proxy fronting Cloudflare) prevent a direct public peer from using a forged `CF-Connecting-IP` to block an innocent visitor; if a local proxy forwards client headers and the deployment is not behind Cloudflare, strip `CF-Connecting-*` at the proxy (see `<network>_CLOUDFLARE_IPS`).

    The blocklist is kept in memory only: a restart clears all active blocks. A blocked client key still blocks every other client sharing that exact key — the same IPv4 address behind CGNAT — but IPv6 blocks now key on the individual `/128`, so unrelated neighbors sharing a `/64` are no longer caught by one address's block.

-   `<network>_REST_UI_RATE_LIMIT` - Maximum number of public HTTP requests a single client key may start within `<network>_REST_UI_RATE_WINDOW`. Accepts a non-negative integer; default `20`, `0` disables request-rate limiting. The client key uses the shared REST/WebSocket attribution rules: an IPv4 address, or an IPv6 `/64` prefix. The default is tight because this surface is for individual human use (Suite uses the WebSocket interface).

    Although it is configured through `REST_UI_*` variables, this limiter governs **all dynamic public routes under one shared per-client budget** — both the explorer UI pages (`/address/`, `/xpub/`, `/tx/`, `/search/`, `/block/`, …) and the REST API (`/api/...`). Only static assets (`/static/`, `/favicon.ico`, `/test-websocket.html`), the API docs (`/api-docs`), the OpenAPI spec (`/openapi.yaml`), and the WebSocket endpoint (`/websocket`, which has its own limiter — see `<network>_WS_MESSAGE_RATE_LIMIT`) are exempt. New routes are covered automatically.

    Requests arriving directly from a loopback/private peer (or a configured trusted proxy) **without** a client attribution header are exempt from all REST limits: such a key identifies the operator's own tooling or a proxy that does not forward the client IP (`X-Real-Ip` unset), and limiting it would throttle a whole deployment as one client. To rate limit traffic behind a reverse proxy, configure the proxy to set `X-Real-Ip` (and list it in `<network>_TRUSTED_PROXIES` if it is not on a local network).

-   `<network>_REST_UI_RATE_WINDOW` - Token-bucket refill window for `<network>_REST_UI_RATE_LIMIT`, as a Go duration string (e.g. `1m`, `60s`). Default `1m`.

-   `<network>_REST_UI_BURST` - Token-bucket burst size for one client key. Default `20`; must be positive when request-rate limiting is enabled. Allows a short flurry of page loads (the explorer renders each page as a single request) while `<network>_REST_UI_RATE_LIMIT` caps the sustained rate.

-   `<network>_REST_UI_MAX_CONCURRENT` - Maximum number of in-flight public HTTP requests (explorer UI page or REST API call) accepted from one client key. Default `12`; `0` disables the per-client concurrency limit. This protects slow or expensive handlers held open concurrently from one source.

-   `<network>_REST_UI_STATE_TTL` - How long idle limiter state is retained for one client key, as a Go duration string. Default `10m`.

-   `<network>_REST_UI_BLOCK_DURATION` - Optional temporary block duration for a client key after repeated rate/concurrency breaches, as a Go duration string. Default `0`, which disables temporary blocking and only returns `429 Too Many Requests` while over the configured limits. Breaches count as separate episodes only when at least 10 seconds apart, so a single burst (for example one page firing dozens of parallel requests) cannot trip the block. As with the WebSocket block, the temporary block keys on the individual address (IPv4, or the full IPv6 `/128`) while request-rate limiting still aggregates IPv6 to its `/64`, so a block cannot deny service to an entire shared `/64`. When enabled, loopback/private/link-local addresses and configured trusted-proxy or Cloudflare edge ranges are never blocked, and a `CF-Connecting-*`-derived address is blockable only when the TCP peer was verified as Cloudflare. The `blockbook_rest_ui_rate_limit_rejections`, `blockbook_rest_ui_active_ips`, `blockbook_rest_ui_max_active_requests_per_ip`, and `blockbook_rest_ui_blocked_ips` metrics track the limiter.

-   `<coin shortcut>_STAKING_POOL_CONTRACT` - The pool name and contract used for Ethereum staking. The format of the variable is `<pool name>/<pool contract>`. If missing, staking support is disabled.

-   `INFURA_API_KEY` - API key for the Infura alternative EIP-1559 fee provider. Archive EVM configs using Infura poll once per minute and keep serving the last successful fee data for up to 30 failed polls before falling back to native fee estimation.

-   `COINGECKO_API_KEY`, `<network>_COINGECKO_API_KEY`, or `<coin shortcut>_COINGECKO_API_KEY` - API key for making requests to CoinGecko in the paid tier.
    If any of these variables is set, it must be non-empty (empty value is treated as a configuration error and Blockbook fails on startup).
    Lookup priority is:
    1. `<network>_COINGECKO_API_KEY`
    2. `<coin shortcut>_COINGECKO_API_KEY`
    3. `COINGECKO_API_KEY`
    Example: for Optimism, `network=OP` and `coin shortcut=ETH`, so `OP_COINGECKO_API_KEY` is preferred over `ETH_COINGECKO_API_KEY`.

-   `<network or coin shortcut>_ALLOWED_RPC_CALL_TO` - Addresses to which `rpcCall` websocket requests can be made, as a comma-separated list. The value and its entries are trimmed and empty entries skipped; a set value that contains no addresses at all (a whitespace-only value included) is a configuration error and Blockbook fails on startup. If omitted (and `ALLOWED_EVM_CALL_METHODS` is not set either), `rpcCall` is enabled for all addresses. This is the startup default of a [runtime setting](#runtime-settings): an override stored through the admin API takes precedence.

-   `<network or coin shortcut>_ALLOWED_EVM_CALL_METHODS` - EVM method selectors (first 4 bytes of the calldata, for example `0xdd62ed3e` for ERC-20 `allowance(address,address)`) that `rpcCall` websocket requests may invoke on any address, as a comma-separated list; the `0x` prefix is optional and matching is case-insensitive. Combines with `ALLOWED_RPC_CALL_TO`: a call is allowed when either its target address or its calldata selector is allowed. When only this variable is set, only calls with an allowed selector pass. Malformed calldata (missing `0x` prefix, invalid or odd-length hex, fewer than 4 bytes) never matches a selector. A malformed selector in the list, or a set variable that contains no selectors at all (a whitespace-only value included), is a configuration error and Blockbook fails on startup. This is the startup default of a [runtime setting](#runtime-settings): an override stored through the admin API takes precedence.

-   `<network or coin shortcut>_ALTERNATIVE_SENDTX_URLS` - Comma-separated list of alternative EVM `eth_sendRawTransaction` providers, used for private/MEV-protected transaction submission. The prefix is the configured `network` value when present (for example `OP`, `BASE`, `POL`, `BSC`, `ARB`, `AVAX`), otherwise the coin shortcut (for example `ETH`). If omitted, Blockbook sends transactions through the normal backend RPC. Nonce (`eth_getTransactionCount`) queries are routed to the alternative provider only for addresses that recently (within `alternativeMempoolTxTimeout`) sent a transaction through it; all other addresses are served by the normal backend RPC. The routing state is kept in process memory: with multiple Blockbook instances behind a load balancer, only the instance that accepted the transaction routes the sender's nonce queries to the alternative provider (websocket clients are naturally sticky to one instance; REST pollers may hit another and receive the public backend's view), and a restart clears the state.

-   `<network or coin shortcut>_ALTERNATIVE_SENDTX_ONLY` - Set to `TRUE` to use only the alternative send transaction provider and avoid fallback to the normal backend RPC if alternative submission fails.

-   `<network or coin shortcut>_ALTERNATIVE_FETCH_MEMPOOL_TX` - Set to `TRUE` to fetch and cache transactions submitted through the alternative provider, so Blockbook can expose them as pending even if they are not visible in the public backend mempool. Cached transactions are periodically checked with the alternative provider's `eth_getTransactionByHash`; an empty response or mined transaction removes the local pending entry. When the alternative provider is enabled, the default alternative cache timeout is 5 minutes and the default Blockbook EVM mempool timeout is 10 minutes; both can be overridden in coin config with `alternativeMempoolTxTimeout` and `mempoolTxTimeout`.

    WebSocket `sendTransaction` can bypass the alternative provider for a single request by setting `disableAlternativeRPC` to `true`.

## Runtime settings

The `rpcCall` allowlists can be changed at runtime, without a restart, through the internal server's authenticated admin API (see `BB_ADMIN_USER`/`BB_ADMIN_PASSWORD` above). An override is persisted in the Blockbook database, survives restarts and takes precedence over the corresponding environment variable, which serves only as the startup default. Every change is logged. The `/admin/runtime-settings` page shows the current values and their sources. The endpoint and the page are registered on every chain type — the runtime-settings mechanism is chain-generic; the currently defined settings only affect EVM `rpcCall` and are simply unset elsewhere.

`GET/POST/DELETE /admin/runtime-settings/<KEY>` where `<KEY>` is `ALLOWED_RPC_CALL_TO` or `ALLOWED_EVM_CALL_METHODS`:

-   `GET` returns the effective value and its source (`db` = stored override, `env` = environment default, `unset` = neither): `{"key":"ALLOWED_EVM_CALL_METHODS","value":"0xdd62ed3e","source":"env"}`. A `GET` of the bare collection path `/admin/runtime-settings/` returns all settings as a JSON array.
-   `POST` (or `PUT`) with body `{"value":"0xdd62ed3e,0x70a08231"}` validates the value (invalid values are rejected with `400` and change nothing), stores it in the database and only then applies it to the live allowlists — a database failure returns `500` and leaves the live state unchanged. A `"value"` of exactly `""` is a valid override meaning "explicitly unconfigured" (that allowlist dimension is disabled, as if its environment variable was not set) — it is the only way to un-restrict at runtime while the environment variable has a value. A value containing only whitespace or separators is rejected with `400`, so a botched automation input cannot silently disable the allowlist.
-   `DELETE` removes the stored override and reverts to the environment default. If the environment value is malformed the request is rejected with `400` and the override is kept, so a later restart cannot fail on it.

Old Blockbook versions ignore the stored overrides (the environment applies again after a version rollback) and keep them intact, so rolling forward resumes the override.

The two sources play distinct roles in a replicated deployment. The environment variable is the deploy-managed baseline: it is shipped identically to every replica with the deployment's env file, applies from the first second of the process (before the admin port is even reachable) and — unlike the database, which is wiped when a replica is resynced — survives a database rebuild, so a freshly synced replica never starts with the allowlists silently unconfigured. The stored override is the runtime layer on top: it takes effect without a restart and persists across restarts until the deployment ships an updated environment and the override is removed. Because an override shadows the environment value, the two can drift (a replica that missed an admin update, or an env change rolled out while an override exists); the drift is visible in the `source` field and Blockbook logs a warning at startup when a stored override shadows a different environment value.

## Contract-info admin endpoint

On EVM chains the internal server also exposes `/admin/contract-info/` (same Basic auth) to manage the contract metadata Blockbook caches from the backend node:

-   `GET /admin/contract-info/<address>` returns the contract's metadata as a `ContractInfo` JSON object (fetching it from the backend node and storing it if not cached yet).
-   `GET /admin/contract-info/` (bare collection path) lists the stored records page by page as `{"contracts":[{ContractInfo},…],"next":"<address>"}`. Unlike runtime settings, the collection is unbounded (sync stores a record per contract creation, millions on a busy chain), so the page size is limited: `?limit=<1..10000>` (default `1000`) and `?from=<address>` continue from a cursor; `next` (present only when more records exist) is the `from` of the next page.
-   `POST` (or `PUT`) `/admin/contract-info/` with a JSON array body `[{ContractInfo},…]` updates the stored metadata of the listed contracts; the response is `{"updated":N}`. The write targets the collection path — a `POST` to an address path is rejected with `400`.
-   `DELETE /admin/contract-info/<address>` purges the stored metadata of one contract so it is re-fetched from the backend node on the next read; the response is `{"contract":"<address>","deleted":true|false,"purged":{ContractInfo}}` (`deleted` is `false` and `purged` absent when nothing was stored — the delete is idempotent). Note that the whole record is discarded: the backend re-fetch restores only name/symbol/decimals, not the sync-owned `createdInBlock`/`destructedInBlock` fields, which are otherwise recoverable only by a reindex. The `purged` record in the response (also logged) can be `POST`ed back to restore them.

## Build-time variables

-   `BB_BUILD_ENV` - Selects the active RPC URL override family during package/config generation. Defaults to `dev`.
    Accepted values are `dev` and `prod`.
-   `BB_DEV_RPC_URL_HTTP_<coin alias>` / `BB_PROD_RPC_URL_HTTP_<coin alias>` - Override `ipc.rpc_url_template` during
    package/config generation so build and integration-test tooling can target hosted HTTP RPC endpoints without editing
    coin JSON. Lookup prefers the exact alias and also accepts archive variants like `<alias>_archive` and
    `<prefix>_archive_<suffix>` within the selected env family.
-   `BB_DEV_RPC_URL_WS_<coin alias>` / `BB_PROD_RPC_URL_WS_<coin alias>` - Override `ipc.rpc_url_ws_template` for
    WebSocket subscriptions; should point to the same host as the selected HTTP RPC override and follows the same
    fallback resolution.
-   `BB_DEV_MQ_URL_<coin alias>` / `BB_PROD_MQ_URL_<coin alias>` - Override `ipc.message_queue_binding_template`
    during package/config generation. The value is used as-is, so it should include the full MQ transport URL
    (for example `tcp://backend_hostname:28332`). This follows the same alias/archive fallback resolution as the
    RPC URL overrides.
-   `BB_RPC_BIND_HOST_<coin alias>` - Overrides backend RPC bind host during package/config generation; when set to
    `0.0.0.0`, RPC stays restricted unless `BB_RPC_ALLOW_IP_<coin alias>` is set.
-   `BB_RPC_ALLOW_IP_<coin alias>` - Overrides backend RPC allow list for UTXO configs (e.g. `rpcallowip`), defaulting
    to `127.0.0.1`.

## CI/CD workflow variables

-   `BB_RUNNER_<coin>` - Maps a workflow/config coin name from `configs/coins/<coin>.json` to the self-hosted runner label
    used by the `Build / Deploy` workflow. `production_builder` marks coins that are buildable only in `env=prod`; those builds run on the `production-builder` self-hosted runner label.

-   `BB_PACKAGE_ROOT` - Absolute filesystem path where workflow build jobs stage copied `.deb` packages after build.
    Defaults to `/opt/blockbook-builds` in the workflow.

-   `BB_DEV_API_URL_HTTP_<test name>` - Overrides the HTTP Blockbook API endpoint used by API/e2e tests and the
    post-deploy sync wait step. Uses the test identity (`coin.test_name`, or config filename fallback), not `coin.alias`.

-   `BB_DEV_API_URL_WS_<test name>` - Overrides the WebSocket Blockbook API endpoint used by API/e2e tests. Uses the
    same test identity as `BB_DEV_API_URL_HTTP_<test name>`.
