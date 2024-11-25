# Blockbook Introduction

-   backend for `Suite`
-   essential for `Suite` to work
-   supports multiple coins - around 80 officially, and much more unofficially
-   github repository is quite popular - 700 forks, 700 stars - https://github.com/trezor/blockbook
-   public UI - https://btc1.trezor.io/

# Building blocks of Blockbook

## Go language

-   developed by Google
-   statically typed, compiled language
-   can do concurrent programming (goroutines)
-   good bindings for RocksDB
-   error handling is explicit - no exceptions, no try-catch

---

## ZeroMQ

-   messaging library
-   listens for `hashblock` and `hashtx` events/notifications/messages from `bitcoind` and runs a callback function (sends notifications to subscribers)
-   used in [bchain/mq.go](../bchain/mq.go)

---

## RocksDB

-   used in [db/rocksdb.go](../db/rocksdb.go)

---

## Bitcoin RPC (bitcoind)

-   used in [bchain/coins/btc/bitcoinrpc.go](../bchain/coins/btc/bitcoinrpc.go)

# Backend software components/parts

-   programming language
    -   Go
    -   good for concurrent programming - goroutines, channels
    -   support for RocksDB
    -   explicit error handling - no exceptions
    -   capital letters for public functions, variables, etc.
-   main entrypoint/loop
    -   according to CLI args, does some things
    -   `blockbook.go`
-   configurations
    -   one config per coin - `configs/coins/bitcoin.json` - RPC details, ports
    -   `build/blockchaincfg.json` - coin + server configurations
    -   `blockbook.go` taking CLI args
-   modularization
    -   the core is blockchain agnostic, bitcoin specified an RPC client interface, all coins implement this interface - with the possibility of default implementations
-   db
    -   RocksDB - `db/rocksdb.go`
    -   column families, no tables
    -   key-value store of bytes, very fast and efficient
-   data source
    -   bitcoind RPC for blockchain data, ZeroMQ for notifications
    -   coingecko API for fiat rates
-   data synchronization
    -   initial sync
    -   incremental sync
    -   reorg handling
-   application states
    -   synchronizing, ready for requests, DB inconsistent
-   dev environment
    -   dev server, local development
-   logging
    -   glog ... INFO, WARNING, ERROR
-   monitoring
    -   external, via internal API
-   reporting / metrics
    -   Prometheus - metrics, Grafana - dashboards
    -   `common/metrics.go`
-   REST API
    -   `/server/public.go` - `ConnectFullPublicInterface`
-   websocket API
    -   `server/websocket.go` - `requestHandlers`
-   admin interface / internal dashboard
    -   very small now
    -   `server/internal.go`
-   frontend / UI
    -   `/`, `/test-websocket.html`
-   cron jobs / scheduled / periodic tasks
    -   fetching fiat rates
-   authentication/authorization
    -   not used, only for the admin interface
-   caching
    -   saving used transactions in RocksDB
-   testing
    -   unit tests, integration tests
    -   `go test -tags=unittest ./db`
-   deployment
    -   Debian packages, Debian servers, bare-metal, no cloud
-   CI/CD
    -   Github Actions, previously Gitlab CI
-   security
-   backups
-   scaling
-   performance
-   error handling
-   rate limiting
-   load balancing
-   cloudflare
-   messaging systems
-   localization
-   documentation
    -   `docs` folder
-   support
-   server resources
-   reliability
-   developers
-   contributors
-   repository
-   users
-   clients

---

Schedule

-   1. Diagram introduction - 5 minutes
-   2. Backend terms brainstorming - 10 minutes
-   3. Backend terms discussing + code + diagrams - 45 minutes
       Break - 5 minutes
-   4. Installing dependencies - 15 minutes
-   5. Installing and running local regtest bitcoind - 15 minutes
-   6. Running local Blockbook connected to local regtest - 15 minutes
       Break - 5 minutes
-   7. Playing with code, implementing some API and UI changes - 60 minutes
