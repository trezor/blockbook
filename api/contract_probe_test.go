//go:build unittest

package api

import (
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

// contractProbeChain counts what the metadata path reads from the chain. Contracts absent
// from tokens are reported the way a dead contract or an ERC1155 is: no token at the address.
type contractProbeChain struct {
	bchain.BlockChain
	mu          sync.Mutex
	tokens      map[string]*bchain.ContractInfo
	err         error
	singleCalls int
	batchCalls  int
	batchSizes  []int
}

func (c *contractProbeChain) GetContractInfo(contractDesc bchain.AddressDescriptor) (*bchain.ContractInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.singleCalls++
	if c.err != nil {
		return nil, c.err
	}
	return copyContractInfo(c.tokens[string(contractDesc)]), nil
}

func (c *contractProbeChain) EthereumTypeGetContractInfos(contractDescs []bchain.AddressDescriptor) []bchain.EthereumContractInfoResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.batchCalls++
	c.batchSizes = append(c.batchSizes, len(contractDescs))
	results := make([]bchain.EthereumContractInfoResult, len(contractDescs))
	for i, cd := range contractDescs {
		if c.err != nil {
			results[i].Err = c.err
			continue
		}
		results[i].Info = copyContractInfo(c.tokens[string(cd)])
	}
	return results
}

// AverageBlockTimeDuration feeds the wall-clock negative TTL its per-coin block count; without
// it the cache is disabled and every probe repeats.
func (c *contractProbeChain) AverageBlockTimeDuration() (time.Duration, error) {
	return 12 * time.Second, nil
}

func (c *contractProbeChain) counts() (single, batch int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.singleCalls, c.batchCalls
}

// contractProbeChainNoBatch is a chain that offers no batched resolve, e.g. a non-EVM coin.
type contractProbeChainNoBatch struct {
	bchain.BlockChain
	inner *contractProbeChain
}

func (c *contractProbeChainNoBatch) GetContractInfo(contractDesc bchain.AddressDescriptor) (*bchain.ContractInfo, error) {
	return c.inner.GetContractInfo(contractDesc)
}

func (c *contractProbeChainNoBatch) AverageBlockTimeDuration() (time.Duration, error) {
	return c.inner.AverageBlockTimeDuration()
}

// setupContractProbeWorker indexes the two EthereumType test blocks, which leave contract 4a
// described in the index and contracts 0d/cd absent from it - the state that sends the token
// loop to the chain.
func setupContractProbeWorker(t *testing.T, chain bchain.BlockChain) (*Worker, *db.RocksDB, *eth.EthereumParser) {
	t.Helper()
	parser := eth.NewEthereumParser(1, true)
	tmp, err := os.MkdirTemp("", "contract-probe")
	require.NoError(t, err)
	database, err := db.NewRocksDB(tmp, 100000, -1, parser, nil, false)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, database.Close())
		require.NoError(t, os.RemoveAll(tmp))
	})
	is, err := database.LoadInternalState(&common.Config{CoinName: "eth-unittest"})
	require.NoError(t, err)
	database.SetInternalState(is)
	require.NoError(t, database.ConnectBlock(dbtestdata.GetTestEthereumTypeBlock1(parser)))
	require.NoError(t, database.ConnectBlock(dbtestdata.GetTestEthereumTypeBlock2(parser)))
	// db reads ContractInfo through a package-level cache shared by every instance, so rows
	// an earlier test stored would otherwise still be visible here
	db.ResetContractInfoCache()
	w, err := NewWorker(database, chain, nil, nil, nil, is, nil)
	require.NoError(t, err)
	return w, database, parser
}

func newContractProbeChain(t *testing.T, parser *eth.EthereumParser) *contractProbeChain {
	t.Helper()
	fake, err := dbtestdata.NewFakeBlockChainEthereumType(parser)
	require.NoError(t, err)
	return &contractProbeChain{BlockChain: fake, tokens: map[string]*bchain.ContractInfo{}}
}

func addrDesc(t *testing.T, parser *eth.EthereumParser, address string) bchain.AddressDescriptor {
	t.Helper()
	cd, err := parser.GetAddrDescFromAddress("0x" + address)
	require.NoError(t, err)
	return cd
}

// A contract the chain reports as holding no token is never written to the index, so without
// the negative cache every request re-probes it. This is the regression the cache exists for.
func TestGetContractDescriptorInfo_NotATokenIsProbedOnce(t *testing.T) {
	parser := eth.NewEthereumParser(1, true)
	chain := newContractProbeChain(t, parser)
	w, _, parser := setupContractProbeWorker(t, chain)
	cd := addrDesc(t, parser, dbtestdata.EthAddrContract0d)

	first, validFirst, err := w.getContractDescriptorInfo(cd, bchain.ERC20TokenStandard)
	require.NoError(t, err)
	second, validSecond, err := w.getContractDescriptorInfo(cd, bchain.ERC20TokenStandard)
	require.NoError(t, err)

	single, _ := chain.counts()
	require.Equal(t, 1, single, "the second request must not reach the chain")
	require.False(t, validFirst)
	require.False(t, validSecond)
	// a cached verdict has to be indistinguishable from a freshly probed one
	require.Equal(t, first, second)
	require.Equal(t, parser.AmountDecimals(), second.Decimals)
	require.Equal(t, bchain.UnknownTokenStandard, second.Standard)
}

// A read that failed says nothing about the contract: caching it would blank a live token out
// of the response for the whole TTL.
func TestGetContractDescriptorInfo_FailedReadIsNotCached(t *testing.T) {
	parser := eth.NewEthereumParser(1, true)
	chain := newContractProbeChain(t, parser)
	chain.err = errors.New("dial tcp: connection refused")
	w, _, parser := setupContractProbeWorker(t, chain)
	cd := addrDesc(t, parser, dbtestdata.EthAddrContract0d)

	for i := 0; i < 2; i++ {
		_, valid, err := w.getContractDescriptorInfo(cd, bchain.ERC20TokenStandard)
		require.NoError(t, err)
		require.False(t, valid)
	}

	single, _ := chain.counts()
	require.Equal(t, 2, single, "a failed read must leave the contract probeable")
}

// The negative is a safety valve, not a permanent verdict: a contract that gains token code
// later has to be picked up once the TTL passes.
func TestGetContractDescriptorInfo_NegativeCacheExpires(t *testing.T) {
	parser := eth.NewEthereumParser(1, true)
	chain := newContractProbeChain(t, parser)
	w, database, parser := setupContractProbeWorker(t, chain)
	cd := addrDesc(t, parser, dbtestdata.EthAddrContract0d)

	_, _, err := w.getContractDescriptorInfo(cd, bchain.ERC20TokenStandard)
	require.NoError(t, err)
	_, bestHeight, _, _ := database.GetInternalState().GetSyncState()
	// 15 minutes of 12s blocks, plus one to pass the expiry
	database.GetInternalState().UpdateBestHeight(bestHeight + blocksForDuration(contractProbeNegativeTTL, 12*time.Second) + 1)
	_, _, err = w.getContractDescriptorInfo(cd, bchain.ERC20TokenStandard)
	require.NoError(t, err)

	single, _ := chain.counts()
	require.Equal(t, 2, single)
}

// A resolved token heals through the index instead of the cache, and stays resolved.
func TestGetContractDescriptorInfo_ResolvedTokenIsStored(t *testing.T) {
	parser := eth.NewEthereumParser(1, true)
	chain := newContractProbeChain(t, parser)
	w, database, parser := setupContractProbeWorker(t, chain)
	cd := addrDesc(t, parser, dbtestdata.EthAddrContract0d)
	chain.tokens[string(cd)] = &bchain.ContractInfo{
		Contract: eth.EIP55Address(cd),
		Name:     "Mint Token",
		Symbol:   "MTT",
		Decimals: 8,
	}

	got, valid, err := w.getContractDescriptorInfo(cd, bchain.ERC20TokenStandard)
	require.NoError(t, err)
	require.True(t, valid)
	require.Equal(t, "MTT", got.Symbol)
	require.Equal(t, bchain.ERC20TokenStandard, got.Standard)

	stored, err := database.GetContractInfo(cd, bchain.UnknownTokenStandard)
	require.NoError(t, err)
	require.NotNil(t, stored, "a resolved token must reach the index")

	_, _, err = w.getContractDescriptorInfo(cd, bchain.ERC20TokenStandard)
	require.NoError(t, err)
	single, _ := chain.counts()
	require.Equal(t, 1, single)
}

// The pre-pass covers exactly the contracts the loop would have probed: the ones the index
// does not describe.
func TestPrefetchContractInfos_BatchesUnknownContracts(t *testing.T) {
	parser := eth.NewEthereumParser(1, true)
	chain := newContractProbeChain(t, parser)
	w, database, parser := setupContractProbeWorker(t, chain)
	ca, err := database.GetAddrDescContracts(addrDesc(t, parser, dbtestdata.EthAddr7b))
	require.NoError(t, err)

	probes := w.prefetchContractInfos(ca.Contracts, nil)

	single, batch := chain.counts()
	require.Equal(t, 1, batch, "one batched call for the whole address")
	require.Equal(t, 0, single)
	require.Equal(t, []int{2}, chain.batchSizes, "contract 4a is already in the index")
	require.Contains(t, probes, string(addrDesc(t, parser, dbtestdata.EthAddrContract0d)))
	require.Contains(t, probes, string(addrDesc(t, parser, dbtestdata.EthAddrContractCd)))
	require.NotContains(t, probes, string(addrDesc(t, parser, dbtestdata.EthAddrContract4a)))
}

// Contracts a recent probe already settled stay out of the batch; one contract left is not
// worth a batched call at all.
func TestPrefetchContractInfos_SkipsCachedNegatives(t *testing.T) {
	parser := eth.NewEthereumParser(1, true)
	chain := newContractProbeChain(t, parser)
	w, database, parser := setupContractProbeWorker(t, chain)
	ca, err := database.GetAddrDescContracts(addrDesc(t, parser, dbtestdata.EthAddr7b))
	require.NoError(t, err)
	bestHeight, reorgGen := w.contractProbeCacheState()
	w.contractProbeCache.add(string(addrDesc(t, parser, dbtestdata.EthAddrContract0d)), bestHeight, 100, reorgGen)

	require.Nil(t, w.prefetchContractInfos(ca.Contracts, nil))

	_, batch := chain.counts()
	require.Equal(t, 0, batch)
}

// A conclusive not-a-token from the batch is as cacheable as one from a single read - that is
// what keeps the batch from being re-run on every request.
func TestGetProbedContractDescriptorInfo_CachesBatchedNegative(t *testing.T) {
	parser := eth.NewEthereumParser(1, true)
	chain := newContractProbeChain(t, parser)
	w, database, parser := setupContractProbeWorker(t, chain)
	ca, err := database.GetAddrDescContracts(addrDesc(t, parser, dbtestdata.EthAddr7b))
	require.NoError(t, err)
	cd := addrDesc(t, parser, dbtestdata.EthAddrContract0d)

	probes := w.prefetchContractInfos(ca.Contracts, nil)
	_, valid, err := w.getProbedContractDescriptorInfo(cd, bchain.ERC20TokenStandard, probes)
	require.NoError(t, err)
	require.False(t, valid)
	// the next request has neither a batch nor a probe to run
	_, _, err = w.getContractDescriptorInfo(cd, bchain.ERC20TokenStandard)
	require.NoError(t, err)

	single, batch := chain.counts()
	require.Equal(t, 1, batch)
	require.Equal(t, 0, single)
}

// A batched read that failed is not a verdict either, so the contract stays probeable.
func TestGetProbedContractDescriptorInfo_BatchedErrorIsNotCached(t *testing.T) {
	parser := eth.NewEthereumParser(1, true)
	chain := newContractProbeChain(t, parser)
	chain.err = errors.New("context deadline exceeded")
	w, database, parser := setupContractProbeWorker(t, chain)
	ca, err := database.GetAddrDescContracts(addrDesc(t, parser, dbtestdata.EthAddr7b))
	require.NoError(t, err)
	cd := addrDesc(t, parser, dbtestdata.EthAddrContract0d)

	probes := w.prefetchContractInfos(ca.Contracts, nil)
	_, _, err = w.getProbedContractDescriptorInfo(cd, bchain.ERC20TokenStandard, probes)
	require.NoError(t, err)
	_, _, err = w.getContractDescriptorInfo(cd, bchain.ERC20TokenStandard)
	require.NoError(t, err)

	single, _ := chain.counts()
	require.Equal(t, 1, single, "the failed batch must leave the contract probeable")
}

// The whole point of the pre-pass: an address whose contracts the index does not describe
// costs one batched call, not up to three serialized eth_calls per contract.
func TestGetEthereumTypeAddressBalances_ResolvesMetadataInOneBatch(t *testing.T) {
	parser := eth.NewEthereumParser(1, true)
	chain := newContractProbeChain(t, parser)
	w, _, parser := setupContractProbeWorker(t, chain)
	cd := addrDesc(t, parser, dbtestdata.EthAddr7b)
	cdCd := addrDesc(t, parser, dbtestdata.EthAddrContractCd)
	chain.tokens[string(cdCd)] = &bchain.ContractInfo{
		Contract: eth.EIP55Address(cdCd),
		Name:     "Nifty",
		Symbol:   "NFT",
	}

	_, data, err := w.getEthereumTypeAddressBalances(cd, AccountDetailsTokens, &AddressFilter{Vout: AddressFilterVoutOff}, "")
	require.NoError(t, err)

	single, batch := chain.counts()
	require.Equal(t, 1, batch)
	require.Equal(t, 0, single)
	names := map[string]string{}
	for _, token := range data.tokens {
		names[token.Contract] = token.Name
	}
	require.Equal(t, "Nifty", names[eth.EIP55Address(cdCd)])
	require.Equal(t, "Contract 74", names[eth.EIP55Address(addrDesc(t, parser, dbtestdata.EthAddrContract4a))], "the indexed contract keeps its stored metadata")
}

// Without a batched resolve the per-contract path still works, and still caches its verdicts.
func TestPrefetchContractInfos_NoBatchResolver(t *testing.T) {
	parser := eth.NewEthereumParser(1, true)
	inner := newContractProbeChain(t, parser)
	w, database, parser := setupContractProbeWorker(t, &contractProbeChainNoBatch{BlockChain: inner.BlockChain, inner: inner})
	ca, err := database.GetAddrDescContracts(addrDesc(t, parser, dbtestdata.EthAddr7b))
	require.NoError(t, err)

	require.Nil(t, w.prefetchContractInfos(ca.Contracts, nil))

	cd := addrDesc(t, parser, dbtestdata.EthAddrContract0d)
	for i := 0; i < 2; i++ {
		_, _, err := w.getContractDescriptorInfo(cd, bchain.ERC20TokenStandard)
		require.NoError(t, err)
	}
	single, batch := inner.counts()
	require.Equal(t, 0, batch)
	require.Equal(t, 1, single)
}
