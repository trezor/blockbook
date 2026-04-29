# Environment variables

Some behavior of Blockbook can be modified by environment variables. The variables usually start with a coin shortcut to allow to run multiple Blockbooks on a single server.

-   `<coin shortcut>_WS_GETACCOUNTINFO_LIMIT` - Limits the number of `getAccountInfo` requests per websocket connection to reduce server abuse. Accepts number as input.

-   `<coin shortcut>_WS_ALLOWED_ORIGINS` - Comma-separated list of allowed WebSocket origins (e.g. `https://example.com`, `http://localhost:3000`). If omitted, all origins are allowed and it is the operator's responsibility to enforce origin access (for example via proxy).

-   `<network>_WS_TRUSTED_PROXIES` - Comma-separated list of trusted proxy CIDRs whose `X-Real-Ip` header should be used as the WebSocket client IP. This IP is used by per-IP WebSocket connection and connection-attempt limits.
    Blockbook always trusts `X-Real-Ip` from loopback, RFC1918/private, and link-local peers, so this variable is only needed for additional non-local proxies.

    If this variable is unset, Blockbook keeps the default Cloudflare behavior and uses `CF-Connecting-IPv6` first, then `CF-Connecting-IP`, when either header contains a valid IP address. This is intended for deployments where the origin only accepts traffic from Cloudflare IP ranges, for example enforced by nginx or a firewall. Blockbook does not validate the TCP peer against Cloudflare ranges itself.

    If this variable is set, Blockbook switches to generic trusted-proxy mode: `CF-Connecting-IP` and `CF-Connecting-IPv6` are ignored, and `X-Real-Ip` is used only when the TCP peer is a built-in trusted proxy or matches one of the configured CIDRs. In this mode the proxy must overwrite or strip any client-supplied `X-Real-Ip` header before forwarding requests to Blockbook.

    Do not set this variable for a normal Cloudflare-only deployment unless the proxy in front of Blockbook sets `X-Real-Ip` to the real visitor IP. Otherwise all clients may collapse to the proxy or Cloudflare address for rate limiting.

    To avoid unsafe configuration, Blockbook fails startup if a configured prefix is too broad (`/<8` for IPv4, `/<16` for IPv6), malformed, or uses IPv4-mapped IPv6 notation. Use regular IPv4 CIDR notation instead, for example `198.51.100.0/24` rather than `::ffff:198.51.100.0/120`.

-   `<coin shortcut>_STAKING_POOL_CONTRACT` - The pool name and contract used for Ethereum staking. The format of the variable is `<pool name>/<pool contract>`. If missing, staking support is disabled.

-   `COINGECKO_API_KEY`, `<network>_COINGECKO_API_KEY`, or `<coin shortcut>_COINGECKO_API_KEY` - API key for making requests to CoinGecko in the paid tier.
    If any of these variables is set, it must be non-empty (empty value is treated as a configuration error and Blockbook fails on startup).
    Lookup priority is:
    1. `<network>_COINGECKO_API_KEY`
    2. `<coin shortcut>_COINGECKO_API_KEY`
    3. `COINGECKO_API_KEY`
    Example: for Optimism, `network=OP` and `coin shortcut=ETH`, so `OP_COINGECKO_API_KEY` is preferred over `ETH_COINGECKO_API_KEY`.

-   `<coin shortcut>_ALLOWED_RPC_CALL_TO` - Addresses to which `rpcCall` websocket requests can be made, as a comma-separated list. If omitted, `rpcCall` is enabled for all addresses.

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
