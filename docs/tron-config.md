# Tron Configuration

This document describes the Tron-specific backend configuration used by Blockbook.

## Overview

Tron uses three backend HTTP surfaces:

* JSON-RPC, typically exposed on `8545/jsonrpc`
* Full node HTTP API, typically exposed on `8090`
* Solidity node HTTP API, typically exposed on `8091`

Blockbook uses the JSON-RPC endpoint for Ethereum-compatible RPC calls and the two Tron HTTP APIs for Tron-specific lookups such as `/wallet/...` and `/walletsolidity/...`.

## Config Fields

The primary Tron backend endpoint is configured in `ipc.rpc_url_template` in:

* [`configs/coins/tron.json`](/configs/coins/tron.json)
* [`configs/coins/tron_testnet_nile.json`](/configs/coins/tron_testnet_nile.json)

Example:

```json
"ipc": {
    "rpc_url_template": "http://127.0.0.1:{{.Ports.BackendRPC}}/jsonrpc"
}
```

Optional Tron-specific HTTP endpoints can also be defined via `blockbook.block_chain.additional_params`:

* `tron_fullnode_http_url_template`
  Used for full node HTTP endpoints such as `/wallet/getblockbynum` and `/wallet/gettransactioninfobyid`.
* `tron_solidity_http_url_template`
  Used for solidity node HTTP endpoints such as `/walletsolidity/getblockbynum` and `/walletsolidity/gettransactioninfobyid`.

## Fallback Behavior

If `tron_fullnode_http_url_template` and `tron_solidity_http_url_template` are omitted, Blockbook derives them from `rpc_url`.

It keeps the same scheme and host and uses:

* port `8090` for the full node HTTP API
* port `8091` for the solidity node HTTP API

Example:

* `rpc_url = http://tron-node.example:8545/jsonrpc`
* full node HTTP URL = `http://tron-node.example:8090`
* solidity node HTTP URL = `http://tron-node.example:8091`

This makes the common deployment case work with a single override:

## When To Set Explicit Tron HTTP URLs

Most setups should rely on the fallback.

Set explicit `tron_fullnode_http_url_template` and `tron_solidity_http_url_template` only when:

* the full node and solidity APIs are exposed on different hosts
* the scheme differs from the JSON-RPC endpoint
* the deployment uses non-standard ports

## Related Docs

* [Config](/docs/config.md)
* [Testing](/docs/testing.md)
