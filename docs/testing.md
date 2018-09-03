# Testing

There are two kinds of tests in Blockbook – unit tests and integration tests. Because integration tests require running
back-end daemon so they can't be executed at every build. Blockbook's build system uses conditional compilation to
distinguish which tests should be executed.

To execute unit tests run `make test`. To execute unit tests and integration tests run `make test-all`. You can use
Go's flag *-run* to filter which tests should be executed. Use *ARGS* variable, e.g.
`make test-all ARGS="-run TestBitcoin"`.

## Unit tests

Unit test file must start with constraint `// +build unittest` followed by blank line (constraints are described
[here](https://golang.org/pkg/go/build/#hdr-Build_Constraints)).

Every coin implementation should have unit tests. At least for parser. Usual test suite define real transaction data
and try pack and unpack them. Specialities of particular coin are tested too. See examples in
[bcashparser_test.go](/bchain/coins/bch/bcashparser_test.go),
[bitcoinparser_test.go](/bchain/coins/btc/bitcoinparser_test.go) and
[ethparser_test.go](/bchain/coins/eth/ethparser_test.go).


## Integration tests

Integration test file must start with constraint `// +build integration` followed by blank line (constraints are
described [here](https://golang.org/pkg/go/build/#hdr-Build_Constraints)).

> Tests that cannot connect back-end service are skipped. `go test` doesn't show any information about skipped test,
> so you must run it in verbose mode with flag *-v*.

### Blockbook integration tests

TODO

### Back-end RPC integration tests

This kind of tests test *bchain.BlockChain* implementation and its capability to communicate with back-end RPC. Tests
of most of coins are similar so there is single generalized implementation in package *blockbook/bchain/tests/rpc*. Test
functions of particular coin implementation can just initialize test object and call its methods. Configuration of tests
is stored in *blockbook/bchain/tests/rpc/config.json* and consists of back-end URL and credentials. Every test suite also
has fixtures stored in *blockbook/bchain/tests/rpc/testdata*. Content is obvious from existing files.

Tests listed below just call back-end RPC methods with parameters from fixture file and check results against same
fixture file. So data in fixture file must be related together.

* TestGetBlockHash – Calls *BlockChain.GetBlockHash* with height and checks returned hash.
* TestGetBlockHeader – Calls *BlockChain.GetBlockHeader* with hash and check returned header. Note that only fields
  that are significant are *Hash* and *Height*, they are checked against fixtures.
* TestGetBlock – Calls *BlockChain.GetBlock* with hash and checks returned block (actually number of transactions and
  their txids).
* TestGetTransaction – Calls *BlockChain.GetTransaction* with txid and checks result against transaction object, where
  *txid* is key and* **transaction object* is value of *txDetails* object in fixture file.
* TestGetTransactionForMempool – Calls *BlockChain.GetTransactionForMempool* that should be version of
  *BlockChain.GetTransaction* optimized for mempool. Implementation of test is similar.
* TestGetMempoolEntry – Calls *BlockChain.GetMempoolEntry* and checks result. Because mempool is living structure it
  tries to load entry for random transaction in mempool repeatedly.
* TestEstimateSmartFee – Calls *BlockChain.EstimateSmartFee* for few numbers of blocks and checks if returned fee is
  non-negative.
* TestEstimateFee – Calls *BlockChain.EstimateFee*; implementation is same as *TestEstimateSmartFee*.
* TestGetBestBlockHash – Calls *BlockChain.GetBestBlockHash* and verifies that returned hash matches the really last
  block.
* TestGetBestBlockHeight – Calls *BlockChain.GetBestBlockHeight* and verifies that returned height matches the really
  last block.

TODO: TestMempoolSync should be "Blockbook integration test"

* TestMempoolSync – Synchronize *BlockChain*'s mempool and verify if sync was successful.

For example see [bitcoinrpc_test.go](/bchain/coins/btc/bitcoinrpc_test.go) and
[implementation](/bchain/tests/rpc/rpc.go) of test suite.

