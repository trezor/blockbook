# Agent instructions

## Testing

Test your changes before declaring done. Start with unit tests :
```
tests/run-unit-tests.sh [go args]
```
Then connectivity tests :
```
tests/run-integration-tests.sh -run 'TestIntegration/.*/connectivity' # make sure we can reach backends
```
Then continue with integration tests, but start narrow, broaden only when needed :
```
tests/run-integration-tests.sh -run 'TestIntegration/ethereum=main/rpc/GetBlock'
tests/run-integration-tests.sh -run 'TestIntegration/ethereum=main/rpc'
tests/run-integration-tests.sh -run 'TestIntegration/ethereum=main'
```
Test path convention :
```
TestIntegration/<coin>=main|test[#NN]/<connectivity|rpc|sync|api>/<subtest>[/<sub>]
```

- Avoid bitcoin during iteration : `bitcoin=main`'s `MempoolSync` and `GetTransactionForMempool` walk mainnet's
mempool and are slow. Prefer `ethereum`, `bsc`, `tron`, or `bcash`.
- The coin segment comes from a `tests/tests.json` key: keys containing
`_testnet` map to `<prefix>=test`, the rest to `<key>=main`
(`bitcoin_regtest` → `bitcoin_regtest=main`). Collisions are disambiguated
with `#01`, `#02`, ... — today `bitcoin=test` is testnet, `bitcoin=test#01`
is testnet4.
- `tests/run-all-tests.sh` is for CI/CD only — too slow for an agent feedback loop, do not run it.
- `-count=1` bypasses the test cache. Use it when you suspect stale results: `go test`
  fingerprints test binary + args, but it does NOT notice when GitHub Actions repository
  variables (the URLs/credentials your tests dial) change between runs, so a previously
  cached PASS can mask a real failure.

## Facts to keep in mind to avoid regressions

Blockbook instance should be able to : 
 - handle at least 20 000 websocket connections from trezor suite
 - index and catchup with fast L2 chains like Arbitrum or Base 
