# Environment variables

Some behavior of Blockbook can be modified by environment variables. The variables usually start with a coin shortcut to allow to run multiple Blockbooks on a single server.

-   `<coin shortcut>_WS_GETACCOUNTINFO_LIMIT` - Limits the number of `getAccountInfo` requests per websocket connection to reduce server abuse. Accepts number as input.

-   `<coin shortcut>_WS_ALLOWED_ORIGINS` - Comma-separated list of allowed WebSocket origins (e.g. `https://example.com`, `http://localhost:3000`). If omitted, all origins are allowed and it is the operator's responsibility to enforce origin access (for example via proxy).

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
