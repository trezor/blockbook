# Environment variables

Some behavior of Blockbook can be modified by environment variables. The variables usually start with a coin shortcut to allow to run multiple Blockbooks on a single server.

-   `<coin shortcut>_WS_GETACCOUNTINFO_LIMIT` - Limits the number of `getAccountInfo` requests per websocket connection to reduce server abuse. Accepts number as input.

-   `<coin shortcut>_STAKING_POOL_CONTRACT` - The pool name and contract used for Ethereum staking. The format of the variable is `<pool name>/<pool contract>`. If missing, staking support is disabled.

-   `COINGECKO_API_KEY` or `<coin shortcut>_COINGECKO_API_KEY` - API key for making requests to CoinGecko in the paid tier.

-   `<coin shortcut>_ALLOWED_RPC_CALL_TO` - Addresses to which `rpcCall` websocket requests can be made, as a comma-separated list. If omitted, `rpcCall` is enabled for all addresses.

-   `<coin shortcut>_ADDR_CONTRACTS_CACHE_MIN_SIZE`  
    Default: `300000`  
    Description: Minimum packed size (bytes) to consider addressContracts hotness/caching. Accepts bytes or `K/M/G/T` suffixes (e.g. `300000`, `300K`, `1MiB`).
-   `<coin shortcut>_ADDR_CONTRACTS_CACHE_ALWAYS_SIZE`  
    Default: `1000000`  
    Description: Always cache addressContracts above this packed size (bytes). Accepts bytes or `K/M/G/T` suffixes.
-   `<coin shortcut>_ADDR_CONTRACTS_CACHE_HOT_MIN_SCORE`  
    Default: `2`  
    Description: Hotness score threshold for caching (float).
-   `<coin shortcut>_ADDR_CONTRACTS_CACHE_HOT_HALF_LIFE`  
    Default: `30m`  
    Description: EWMA half‑life for hotness decay (duration, e.g. `30m`, `2h`).
-   `<coin shortcut>_ADDR_CONTRACTS_CACHE_HOT_EVICT_AFTER`  
    Default: `6h`  
    Description: Evict hotness entries not updated for this duration (e.g. `6h`).
-   `<coin shortcut>_ADDR_CONTRACTS_CACHE_FLUSH_IDLE`  
    Default: `15m`  
    Description: Flush dirty cache entries not updated for this duration.
-   `<coin shortcut>_ADDR_CONTRACTS_CACHE_FLUSH_MAX_AGE`  
    Default: `2h`  
    Description: Flush dirty cache entries older than this duration, even if still hot.

## Build-time variables

-   `BB_RPC_URL_HTTP_<coin alias>` - Overrides `ipc.rpc_url_template` during package/config generation so build and
    integration-test tooling can target hosted HTTP RPC endpoints without editing coin JSON.
-   `BB_RPC_URL_WS_<coin alias>` - Overrides `ipc.rpc_url_ws_template` for WebSocket subscriptions; should point to
    the same host as `BB_RPC_URL_HTTP_<coin alias>`.
-   `BB_RPC_BIND_HOST_<coin alias>` - Overrides backend RPC bind host during package/config generation; when set to
    `0.0.0.0`, RPC stays restricted unless `BB_RPC_ALLOW_IP_<coin alias>` is set.
-   `BB_RPC_ALLOW_IP_<coin alias>` - Overrides backend RPC allow list for UTXO configs (e.g. `rpcallowip`), defaulting
    to `127.0.0.1`.
