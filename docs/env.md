# Environment variables

Some behavior of Blockbook can be modified by environment variables. The variables usually start with a coin shortcut to allow to run multiple Blockbooks on a single server.

-   `<coin shortcut>_WS_GETACCOUNTINFO_LIMIT` - Limits the number of `getAccountInfo` requests per websocket connection to reduce server abuse. Accepts number as input.

-   `<coin shortcut>_WS_ALLOWED_ORIGINS` - Comma-separated list of allowed WebSocket origins (e.g. `https://example.com`, `http://localhost:3000`). If omitted, all origins are allowed and it is the operator's responsibility to enforce origin access (for example via proxy).

-   `<network>_TRUSTED_PROXIES` - Comma-separated list of trusted proxy CIDRs whose `X-Real-Ip` header should be used as the client IP for public REST API and WebSocket rate limiting.
    Blockbook always trusts `X-Real-Ip` from loopback, RFC1918/private, and link-local peers, so this variable is only needed for additional non-local proxies.

    If this variable and its legacy alias are unset, Blockbook keeps the default Cloudflare behavior and uses `CF-Connecting-IPv6` first, then `CF-Connecting-IP`, when either header contains a valid IP address. Whether those headers are trusted from any peer or only from verified Cloudflare peers is controlled by `<network>_CLOUDFLARE_IPS` (see below).

    If this variable is set, Blockbook switches to generic trusted-proxy mode: `CF-Connecting-IP` and `CF-Connecting-IPv6` are ignored, and `X-Real-Ip` is used only when the TCP peer is a built-in trusted proxy or matches one of the configured CIDRs. In this mode the proxy must overwrite or strip any client-supplied `X-Real-Ip` header before forwarding requests to Blockbook.

    Do not set this variable for a normal Cloudflare-only deployment unless the proxy in front of Blockbook sets `X-Real-Ip` to the real visitor IP. Otherwise all clients may collapse to the proxy or Cloudflare address for rate limiting.

    To avoid unsafe configuration, Blockbook fails startup if a configured prefix is too broad (`/<8` for IPv4, `/<16` for IPv6), malformed, or uses IPv4-mapped IPv6 notation. Use regular IPv4 CIDR notation instead, for example `198.51.100.0/24` rather than `::ffff:198.51.100.0/120`.

    For backwards compatibility, `<network>_WS_TRUSTED_PROXIES` is accepted as a legacy alias when `<network>_TRUSTED_PROXIES` is unset. Prefer the shared variable because the same client attribution is used by REST and WebSocket limiters.

-   `<network>_CLOUDFLARE_IPS` - Controls how the `CF-Connecting-IP` / `CF-Connecting-IPv6` headers are trusted in the default (no `<network>_TRUSTED_PROXIES`) mode. Because those headers are client-settable, they are only meaningful if the origin can prove the connection actually came from Cloudflare.
    - Unset with no legacy alias, or `builtin` (default): Blockbook trusts the `CF-Connecting-*` headers only when the TCP peer is inside Cloudflare's published edge ranges (a built-in list, as of 2026-06) or is a loopback/private proxy fronting Cloudflare. A direct public non-Cloudflare peer cannot spoof a client IP past the per-IP limiter or the IP blocklist.
    - A comma-separated CIDR list: use these ranges instead of the built-in list (for example if Cloudflare's ranges drift, or for a custom front-end CDN). Loopback/RFC1918/link-local peers are always also accepted. A value that contains no valid CIDRs fails startup rather than silently disabling verification; only the explicit `off` spellings disable it.
    - `off` (or `none`/`false`/`0`): disable verification and trust `CF-Connecting-*` from any peer (the historical behavior). Only safe when the origin is firewalled to Cloudflare ranges out of band. With verification off, the IP auto-block never acts on a `CF-Connecting-*`-derived address (it would be spoofable), so it only blocks direct TCP peers.

    For backwards compatibility, `<network>_WS_CLOUDFLARE_IPS` is accepted as a legacy alias when `<network>_CLOUDFLARE_IPS` is unset. An explicit shared value, including `off`, does not fall back.

-   `<network>_WS_MESSAGE_RATE_LIMIT` - Maximum number of messages a single WebSocket connection may send within `<network>_WS_MESSAGE_RATE_WINDOW` before the connection is closed and (when blockable) its client key is added to the IP blocklist. Accepts a non-negative integer; default `2500`, `0` disables the per-connection message rate limit entirely. The default of 2500 messages / 10 minutes is above the maximum burst produced by a Trezor Suite client, so it only trips clearly abusive traffic.

-   `<network>_WS_MESSAGE_RATE_WINDOW` - Trailing sliding window for `<network>_WS_MESSAGE_RATE_LIMIT`, as a Go duration string (e.g. `10m`, `600s`). Default `10m`.

-   `<network>_WS_IP_BLOCK_DURATION` - How long a client key (an IPv4 address, or an IPv6 `/64` prefix) is blocked from opening new WebSocket connections after a connection trips the message rate limit, as a Go duration string (e.g. `12h`). Default `12h`; `0` disables IP blocking (an offending connection is still closed). The blocklist is visible at `/admin/ws-limit-exceeding-ips`, and the `blockbook_websocket_blocked_ips` / `blockbook_websocket_blocked_connections` metrics track it.

    Blocking keys on the same client IP attribution as the per-IP limiter. Loopback/private/link-local addresses and any configured trusted-proxy or Cloudflare edge range are never blocked, so a misconfiguration that collapses many clients onto a shared address cannot block them all. Behind Cloudflare, keep `<network>_CLOUDFLARE_IPS` at its default (or set it to your CDN ranges) so blocks key on the real visitor IP. The default Cloudflare peer verification plus a block-safety guard (a `CF-Connecting-*`-derived address is only blocked when the peer was verified as Cloudflare) prevent a forged `CF-Connecting-IP` from being used to block an innocent visitor.

-   `<network>_REST_RATE_LIMIT` - Maximum number of public REST API requests a single client key may start within `<network>_REST_RATE_WINDOW`. Accepts a non-negative integer; default `600`, `0` disables request-rate limiting. The client key uses the shared REST/WebSocket attribution rules: an IPv4 address, or an IPv6 `/64` prefix.

-   `<network>_REST_RATE_WINDOW` - Token-bucket refill window for `<network>_REST_RATE_LIMIT`, as a Go duration string (e.g. `1m`, `60s`). Default `1m`.

-   `<network>_REST_BURST` - REST API token-bucket burst size for one client key. Default `120`; must be positive when request-rate limiting is enabled.

-   `<network>_REST_MAX_CONCURRENT` - Maximum number of in-flight public REST API requests accepted from one client key. Default `24`; `0` disables the per-client concurrency limit. This protects slow or expensive API handlers held open concurrently from one source.

-   `<network>_REST_STATE_TTL` - How long idle REST API limiter state is retained for one client key, as a Go duration string. Default `10m`.

-   `<network>_REST_BLOCK_DURATION` - Optional temporary block duration for a client key after repeated REST API rate/concurrency breaches, as a Go duration string. Default `0`, which disables temporary blocking and only returns `429 Too Many Requests` while over the configured limits. When enabled, loopback/private/link-local addresses and configured trusted-proxy or Cloudflare edge ranges are never blocked, and a `CF-Connecting-*`-derived address is blockable only when the TCP peer was verified as Cloudflare. The `blockbook_rest_api_rate_limit_rejections`, `blockbook_rest_api_active_ips`, `blockbook_rest_api_max_active_requests_per_ip`, and `blockbook_rest_api_blocked_ips` metrics track the limiter.

-   `<coin shortcut>_STAKING_POOL_CONTRACT` - The pool name and contract used for Ethereum staking. The format of the variable is `<pool name>/<pool contract>`. If missing, staking support is disabled.

-   `INFURA_API_KEY` - API key for the Infura alternative EIP-1559 fee provider. Archive EVM configs using Infura poll once per minute and keep serving the last successful fee data for up to 30 failed polls before falling back to native fee estimation.

-   `COINGECKO_API_KEY`, `<network>_COINGECKO_API_KEY`, or `<coin shortcut>_COINGECKO_API_KEY` - API key for making requests to CoinGecko in the paid tier.
    If any of these variables is set, it must be non-empty (empty value is treated as a configuration error and Blockbook fails on startup).
    Lookup priority is:
    1. `<network>_COINGECKO_API_KEY`
    2. `<coin shortcut>_COINGECKO_API_KEY`
    3. `COINGECKO_API_KEY`
    Example: for Optimism, `network=OP` and `coin shortcut=ETH`, so `OP_COINGECKO_API_KEY` is preferred over `ETH_COINGECKO_API_KEY`.

-   `<coin shortcut>_ALLOWED_RPC_CALL_TO` - Addresses to which `rpcCall` websocket requests can be made, as a comma-separated list. If omitted, `rpcCall` is enabled for all addresses.

-   `<network or coin shortcut>_ALTERNATIVE_SENDTX_URLS` - Comma-separated list of alternative EVM `eth_sendRawTransaction` providers, used for private/MEV-protected transaction submission. The prefix is the configured `network` value when present (for example `OP`, `BASE`, `POL`, `BSC`, `ARB`, `AVAX`), otherwise the coin shortcut (for example `ETH`). If omitted, Blockbook sends transactions through the normal backend RPC.

-   `<network or coin shortcut>_ALTERNATIVE_SENDTX_ONLY` - Set to `TRUE` to use only the alternative send transaction provider and avoid fallback to the normal backend RPC if alternative submission fails.

-   `<network or coin shortcut>_ALTERNATIVE_FETCH_MEMPOOL_TX` - Set to `TRUE` to fetch and cache transactions submitted through the alternative provider, so Blockbook can expose them as pending even if they are not visible in the public backend mempool. When the alternative provider is enabled, the default alternative cache timeout is 5 minutes and the default Blockbook EVM mempool timeout is 10 minutes; both can be overridden in coin config with `alternativeMempoolTxTimeout` and `mempoolTxTimeout`.

    WebSocket `sendTransaction` can bypass the alternative provider for a single request by setting `disableAlternativeRPC` to `true`.

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
