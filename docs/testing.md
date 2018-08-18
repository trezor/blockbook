# Testing

There are two kinds of tests in Blockbook – unit tests and integration tests. Because integration tests require running
backend daemon so they can't be executed at every build. Blockbook's build system uses conditional compilation to
distinguish which tests should be executed.

To execute unit tests run `make test`. To execute unit tests and integration tests run `make test-all`. You can use
Go's flag *-run* to filter which tests should be executed. Use *ARGS* variable, e.g.
`make test-all ARGS="-run TestBitcoin"`.

## Unit tests

Unit test file must start with constraint `// +build unittest` followed by blank line (constraints are described
[here](https://golang.org/pkg/go/build/#hdr-Build_Constraints)).

Every coin implementation should have unit tests. At least for parser. Usual test suite define real transaction data
and try pack and unpack them. Specialites of particular coin are tested too. See examples in
[bcashparser_test.go](/bchain/coins/bch/bcashparser_test.go),
[bitcoinparser_test.go](/bchain/coins/btc/bitcoinparser_test.go) and
[ethparser_test.go](/bchain/coins/eth/ethparser_test.go).


## Integration tests

Integration test file must start with constraint `// +build integration` followed by blank line (constraints are
described [here](https://golang.org/pkg/go/build/#hdr-Build_Constraints)).

### Blockbook integration tests

TODO

### Back-end RPC integration tests

This kind of tests test *bchain.BlockChain* implementation and its capability to communicate with back-end RPC. Tests
of most of coins are similar so there is single generalized implementation in package *blockbook/bchain/tests/rpc*. Test
functions of particular coin implementation can just initialize test object and call its methods. Configuration of tests
is stored in *blockbook/bchain/tests/rpc/config.json* and consists of back-end URL and credentials. Every test suite also
has fixtures stored in *blockbook/bchain/tests/rpc/testdata*. Content is obvious from existing files.

For example see [bitcoinrpc_test.go](/bchain/coins/btc/bitcoinrpc_test.go).
