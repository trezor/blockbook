# Testing

There are two kinds of tests in Blockbook – unit tests and integration tests. Because integration tests require running
back-end daemon so they can't be executed at every build. Blockbook's build system uses conditional compilation to
distinguish which tests should be executed.

There are several ways to run tests:

* `make test` – run unit tests only (note that `make deb*` and `make all*` commands always run also *test* target)
* `make test-connectivity` – run connectivity checks only
* `make test-integration` – run RPC and sync integration tests only
* `make test-e2e` – run Blockbook API end-to-end tests only
* `make test-all` – run all tests above

You can use Go's flag *-run* to filter which tests should be executed. Use *ARGS* variable, e.g.
`make test-all ARGS="-run TestBitcoin"`.


## Unit tests

Unit test file must start with constraint `//go:build unittest` followed by blank line (constraints are described
[here](https://golang.org/pkg/go/build/#hdr-Build_Constraints)).

Every coin implementation must have unit tests. At least for parser. Usual test suite define real transaction data
and try pack and unpack them. Specialities of particular coin are tested too. See examples in
[bcashparser_test.go](/bchain/coins/bch/bcashparser_test.go),
[bitcoinparser_test.go](/bchain/coins/btc/bitcoinparser_test.go) and
[ethparser_test.go](/bchain/coins/eth/ethparser_test.go).


## Integration tests

Integration tests test interface between either Blockbook's components or back-end services. Integration tests are
located in *tests* directory and every test suite has its own package. Because RPC, synchronization and Blockbook API
surface are crucial components of Blockbook, it is mandatory that coin implementations have these integration tests
defined. They are implemented in packages `blockbook/tests/rpc`, `blockbook/tests/sync` and `blockbook/tests/api`, and
all of them are declarative. For each coin
there are test definition that enables particular tests of test suite and *testdata* file that contains test fixtures.

Not every coin implementation supports full set of back-end API so it is necessary to define which tests of test suite
are able to run. That is done in test definition file *blockbook/tests/tests.json*. Configuration is hierarchical and
test implementations call each level as separate subtest. Go's *test* command allows filter tests to run by `-run` flag.
It perfectly fits with layered test definitions. For example, you can:

* run connectivity tests for all coins – `make test-connectivity`
* run connectivity tests for a single coin – `make test-connectivity ARGS="-run=TestIntegration/bitcoin=main/connectivity"`
* run tests for single coin – `make test-integration ARGS="-run=TestIntegration/bitcoin/"`
* run single test suite – `make test-integration ARGS="-run=TestIntegration//sync/"`
* run single test – `make test-integration ARGS="-run=TestIntegration//sync/HandleFork"`
* run tests for set of coins – `make test-integration ARGS="-run='TestIntegration/(bcash|bgold|bitcoin|dash|dogecoin|litecoin|snowgem|vertcoin|zcash|zelcash)/'"`
* run e2e tests for all coins – `make test-e2e`
* run e2e tests for single coin – `make test-e2e ARGS="-run=TestIntegration/bitcoin=main/api"`

Integration targets run with `go test -timeout 30m` inside Docker tooling.

Test fixtures are defined in *testdata* directory in package of particular test suite. They are separate JSON files named
by coin. File schemes are very similar with verbose results of CLI tools and are described below. Integration tests
follow the same concept, use live component or service and verify their results with fixtures.

For simplicity, URLs and credentials of back-end services, where are tests going to connect, are loaded
from *blockbook/configs/coins*, the same place from where are production configuration files generated. There are general
URLs that link to *localhost*. If you need run tests against remote servers, there are few options how to do it:

* tests use `BB_BUILD_ENV=dev`
* set `BB_DEV_RPC_URL_HTTP_<coin alias>` to override `rpc_url_template` during template generation (forwarded into Docker by the root `Makefile`)
* set `BB_DEV_RPC_URL_WS_<coin alias>` to override `rpc_url_ws_template` for WebSocket subscriptions when needed
* temporarily change config
* SSH tunneling – `ssh -nNT -L 8030:localhost:8030 remote-server`
* HTTP proxy

### Connectivity integration tests

Connectivity tests are lightweight checks that verify back-end availability before running heavier RPC or sync suites.
They are configured per coin in *blockbook/tests/tests.json* using the `connectivity` list:

* `["http"]` – verify HTTP RPC connectivity
* `["http", "ws"]` – verify HTTP RPC plus WebSocket subscription connectivity

Example:

```
"bitcoin": {
    "connectivity": ["http"]
},
"ethereum": {
    "connectivity": ["http", "ws"]
}
```

HTTP connectivity verifies both back-end and Blockbook accessibility:

* back-end: UTXO chains call `getblockchaininfo`, EVM chains call `web3_clientVersion`
* Blockbook: calls `GET /api/status` (resolved from `BB_DEV_API_URL_HTTP_<test name>` or local `ports.blockbook_public`)

WebSocket connectivity also verifies both surfaces:

* back-end: validates `web3_clientVersion` and opens a `newHeads` subscription
* Blockbook: connects to `/websocket` (or `BB_DEV_API_URL_WS_<test name>`) and calls `getInfo`

### Blockbook API end-to-end tests

Public Blockbook API checks are implemented in package `blockbook/tests/api` and configured per coin by the `api` list
in *blockbook/tests/tests.json*.
Use `make test-e2e` to run this suite only.

Phase 1 covers smoke checks for:

* HTTP: `Status`, `GetBlockIndex`, `GetBlockByHeight`, `GetBlock`, `GetTransaction`, `GetTransactionSpecific`, `GetAddress`, `GetAddressTxids`, `GetAddressTxs`, `GetUtxo`, `GetUtxoConfirmedFilter`
* WebSocket: `WsGetInfo`, `WsGetBlockHash`, `WsGetTransaction`, `WsGetAccountInfo`, `WsGetAccountUtxo`, `WsPing`

Endpoint resolution uses the test name from `coin.test_name` in `configs/coins/<coin>.json`
(or the config file name when `test_name` is omitted) and this precedence:

1. `BB_DEV_API_URL_HTTP_<test name>` and `BB_DEV_API_URL_WS_<test name>`
2. localhost fallback from coin config port `ports.blockbook_public`
3. when WS env var is missing, WS URL is derived from HTTP URL with `/websocket` path

### Synchronization integration tests

Synchronization is crucial part of Blockbook and these tests test whether it is doing well. They sync few blocks from
blockchain and verify them against fixtures. Ranges of blocks to sync are defined in fixtures.

* `ConnectBlocks` – Calls *db.SyncWorker.connectBlocks*, a single-thread method that is called when a new block is detected.
   Sync blocks and checks whether blocks and transactions from fixtures are indexed.
* `ConnectBlocksParallel` – Calls *db.SyncWorker.ConnectBlocksParallel*, a multi-thread method that is used during initial
   synchronization. Uses the same fixtures as ConnectBlocks.
* `HandleFork` – Calls *db.SyncWorker.HandleFork* method that rolls back blockchain if a fork is detected. Test uses two
   sets of blocks with the same heights in fixtures. First set – with fake blocks – is synced initially, than *HandleFork*
   method is called and finally it is checked that index contain only blocks from second set – the real blocks. *Make
   sure that fake blocks have hashes of real blocks out of a sync range. It is important because Blockbook attempts to
   load these blocks and if it is unsuccessful the test fails. A good practice is use blocks with a height about 20 lower
   than `syncRanges.lower` and decreasing.*

### Back-end RPC integration tests

This kind of tests test *bchain.BlockChain* implementation and its capability to communicate with back-end RPC.

Tests listed below just call back-end RPC methods with parameters from fixture file and check results against same
fixture file. So data in fixture file must be related together.

* `GetBlockHash` – Calls *BlockChain.GetBlockHash* with height and checks returned hash.
* `GetBlockHeader` – Calls *BlockChain.GetBlockHeader* with hash and check returned header. Note that only fields
   that are significant are *Hash* and *Height*, they are checked against fixtures. Scheme of transaction data in fixtures
   is very similar to verbose result of *getrawtransaction* command of CLI tools and can be copy-pasted with few
   modifications.
* `GetBlock` – Calls *BlockChain.GetBlock* with hash and checks returned block (actually number of transactions and
   their txids).
* `GetTransaction` – Calls *BlockChain.GetTransaction* with txid and checks result against transaction object, where
   *txid* is key and *transaction object* is value of *txDetails* object in fixture file.
* `GetTransactionForMempool` – Calls *BlockChain.GetTransactionForMempool* that should be version of
   *BlockChain.GetTransaction* optimized for mempool. Implementation of test is similar.
* `GetMempoolEntry` – Calls *BlockChain.GetMempoolEntry* and checks result. Because mempool is living structure it
   tries to load entry for random transaction in mempool repeatedly.
* `EstimateSmartFee` – Calls *BlockChain.EstimateSmartFee* for few numbers of blocks and checks if returned fee is
   non-negative.
* `EstimateFee` – Calls *BlockChain.EstimateFee*; implementation is same as *EstimateSmartFee*.
* `GetBestBlockHash` – Calls *BlockChain.GetBestBlockHash* and verifies that returned hash matches the really last
   block.
* `GetBestBlockHeight` – Calls *BlockChain.GetBestBlockHeight* and verifies that returned height matches the really
   last block.
* `MempoolSync` – Synchronize *BlockChain*'s mempool and verify if sync was successful.
