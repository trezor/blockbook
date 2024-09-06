# Environment variables

Some behavior of Blockbook can be modified by environment variables. The variables usually start with a coin shortcut to allow to run multiple Blockbooks on a single server.

-   `<coin shortcut>_WS_GETACCOUNTINFO_LIMIT` - Limits the number of `getAccountInfo` requests per websocket connection to reduce server abuse. Accepts number as input.

-   `<coin shortcut>_STAKING_POOL_CONTRACT` - The pool name and contract used for Ethereum staking. The format of the variable is `<pool name>/<pool contract>`. If missing, staking support is disabled.

-   `COINGECKO_API_KEY` or `<coin shortcut>_COINGECKO_API_KEY` - API key for making requests to CoinGecko in the paid tier.

-   `<coin shortcut>_ALLOWED_ETH_CALL_CONTRACTS` - Contract addresses for which `ethCall` websocket requests can be made, as a comma-separated list. If omitted, `ethCall` is enabled for all addresses.
