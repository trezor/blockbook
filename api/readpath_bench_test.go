//go:build unittest

package api

import (
	"encoding/json"
	"math/big"
	"sync"
	"testing"

	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

// the prometheus registry is global and refuses a second registration, while the
// testing package calls a benchmark function several times
var (
	benchMetricsOnce sync.Once
	benchMetrics     *common.Metrics
	benchMetricsErr  error
)

func getBenchMetrics() (*common.Metrics, error) {
	benchMetricsOnce.Do(func() {
		benchMetrics, benchMetricsErr = common.GetMetrics("coin-unittest")
	})
	return benchMetrics, benchMetricsErr
}

// benchChain answers the backend calls of the served requests from the fixture blocks
// with constant values, so the benchmarks measure Blockbook and not an RPC round trip.
type benchChain struct {
	bchain.BlockChain
	parser bchain.BlockChainParser
	blocks map[string]*bchain.Block
}

func (c *benchChain) GetChainParser() bchain.BlockChainParser { return c.parser }

func (c *benchChain) EthereumTypeGetBalance(bchain.AddressDescriptor) (*big.Int, error) {
	return big.NewInt(1e18), nil
}

func (c *benchChain) EthereumTypeGetNonces(bchain.AddressDescriptor, bool, ...uint64) (uint64, uint64, bool, error) {
	return 7, 7, true, nil
}

func (c *benchChain) EthereumTypeGetErc20ContractBalances(_ bchain.AddressDescriptor, contracts []bchain.AddressDescriptor) ([]*big.Int, error) {
	r := make([]*big.Int, len(contracts))
	for i := range r {
		r[i] = big.NewInt(1e6)
	}
	return r, nil
}

func (c *benchChain) EthereumTypeGetSupportedStakingPools() []string { return nil }

func (c *benchChain) GetAddressChainExtraData(bchain.AddressDescriptor) (json.RawMessage, error) {
	return nil, nil
}

func (c *benchChain) GetBlockInfo(hash string) (*bchain.BlockInfo, error) {
	block, ok := c.blocks[hash]
	if !ok {
		return nil, bchain.ErrBlockNotFound
	}
	bi := &bchain.BlockInfo{BlockHeader: block.BlockHeader, Txids: make([]string, len(block.Txs))}
	for i := range block.Txs {
		bi.Txids[i] = block.Txs[i].Txid
	}
	return bi, nil
}

type benchMempool struct{ bchain.Mempool }

func (benchMempool) GetAddrDescTransactions(bchain.AddressDescriptor) ([]bchain.Outpoint, error) {
	return nil, nil
}

func (benchMempool) GetTransactionTime(string) uint32 { return 0 }

type ethBenchFixture struct {
	w       *Worker
	busiest string
	// txids of the busiest address, newest first, at most one API page
	txids   []string
	biggest *bchain.Block
}

// setupEthereumBench indexes BLOCKBOOK_BENCH_BLOCKS into a fresh RocksDB and returns a
// worker serving it, skipping the benchmark when the fixture is not available.
func setupEthereumBench(b *testing.B) *ethBenchFixture {
	parser := eth.NewEthereumParser(1, false)
	blocks, err := dbtestdata.LoadEthereumBenchBlocks(parser)
	if err != nil {
		b.Fatal(err)
	}
	if blocks == nil {
		b.Skipf("%s not set", dbtestdata.EthereumBenchBlocksEnv)
	}
	d, err := db.NewRocksDB(b.TempDir(), 100000, -1, parser, nil, false)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { d.Close() })
	is, err := d.LoadInternalState(&common.Config{CoinName: "coin-unittest"})
	if err != nil {
		b.Fatal(err)
	}
	d.SetInternalState(is)
	f := &ethBenchFixture{}
	chain := &benchChain{parser: parser, blocks: make(map[string]*bchain.Block)}
	occurrences := make(map[string]int)
	for _, block := range blocks {
		if err := d.ConnectBlock(block); err != nil {
			b.Fatal(err)
		}
		chain.blocks[block.Hash] = block
		if f.biggest == nil || len(block.Txs) > len(f.biggest.Txs) {
			f.biggest = block
		}
		for i := range block.Txs {
			tx := &block.Txs[i]
			// sync only indexes; the tx body lands in RocksDB on its first API read
			if err := d.PutTx(tx, block.Height, block.Time); err != nil {
				b.Fatal(err)
			}
			for _, a := range tx.Vin[0].Addresses {
				occurrences[a]++
			}
			for _, a := range tx.Vout[0].ScriptPubKey.Addresses {
				occurrences[a]++
			}
			// the index has the contracts, their metadata is normally fetched from the chain
			// on first use; store it up front so no request leaves the database
			csd := tx.CoinSpecificData.(bchain.EthereumSpecificData)
			for _, l := range csd.Receipt.Logs {
				occurrences[l.Address]++
				if err := d.StoreContractInfo(&bchain.ContractInfo{
					Contract: l.Address, Standard: bchain.ERC20TokenStandard, Type: bchain.ERC20TokenStandard,
					Name: "Bench Token", Symbol: "BENCH", Decimals: 18, CreatedInBlock: 1,
				}); err != nil {
					b.Fatal(err)
				}
			}
		}
	}
	for a, n := range occurrences {
		if n > occurrences[f.busiest] {
			f.busiest = a
		}
	}
	addrDesc, err := parser.GetAddrDescFromAddress(f.busiest)
	if err != nil {
		b.Fatal(err)
	}
	err = d.GetAddrDescTransactions(addrDesc, 0, ^uint32(0), func(txid string, height uint32, indexes []int32) error {
		f.txids = append(f.txids, txid)
		return nil
	})
	if err != nil {
		b.Fatal(err)
	}
	if len(f.txids) > 1000 {
		f.txids = f.txids[:1000]
	}
	metrics, err := getBenchMetrics()
	if err != nil {
		b.Fatal(err)
	}
	txCache, err := db.NewTxCache(d, chain, metrics, is, true)
	if err != nil {
		b.Fatal(err)
	}
	f.w = &Worker{
		db:                 d,
		chain:              chain,
		chainParser:        parser,
		chainType:          bchain.ChainEthereumType,
		mempool:            benchMempool{},
		txCache:            txCache,
		is:                 is,
		metrics:            metrics,
		contractProbeCache: newNegativeProbeCache(contractProbeCacheCapacity),
	}
	b.Logf("busiest address %s with %d txs, biggest block %d with %d txs", f.busiest, len(f.txids), f.biggest.Height, len(f.biggest.Txs))
	return f
}

func reportPerTx(b *testing.B, txs int) {
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*txs), "ns/tx")
}

// BenchmarkEthereumAddressTxsPage serves one transaction page of the busiest address the
// way the address handlers do per transaction: RocksDB read, unpack, API conversion and
// JSON encoding, without the account part of the response.
func BenchmarkEthereumAddressTxsPage(b *testing.B) {
	f := setupEthereumBench(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		addresses := make(map[string]struct{})
		for _, txid := range f.txids {
			tx, err := f.w.txFromTxid(txid, 0, AccountDetailsTxHistory, nil, addresses)
			if err != nil {
				b.Fatal(err)
			}
			if _, err := json.Marshal(tx); err != nil {
				b.Fatal(err)
			}
		}
	}
	reportPerTx(b, len(f.txids))
}

func benchGetAccountInfo(b *testing.B, details AccountDetails, pageSize int) {
	f := setupEthereumBench(b)
	filter := &AddressFilter{Vout: AddressFilterVoutOff}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a, err := f.w.GetAddress(f.busiest, 1, pageSize, details, filter, "")
		if err != nil {
			b.Fatal(err)
		}
		if _, err := json.Marshal(a); err != nil {
			b.Fatal(err)
		}
	}
	if details >= AccountDetailsTxHistory {
		reportPerTx(b, min(pageSize, len(f.txids)))
	}
}

// getAccountInfo as the websocket serves it: details=basic, and details=txs with the
// default page of 25 and with the REST maximum of 1000 transactions.
func BenchmarkEthereumGetAccountInfoBasic(b *testing.B) {
	benchGetAccountInfo(b, AccountDetailsBasic, 25)
}

func BenchmarkEthereumGetAccountInfoTxs25(b *testing.B) {
	benchGetAccountInfo(b, AccountDetailsTxHistory, 25)
}

func BenchmarkEthereumGetAccountInfoTxs1000(b *testing.B) {
	benchGetAccountInfo(b, AccountDetailsTxHistory, 1000)
}

// BenchmarkEthereumGetBlock serves /api/v2/block/<hash> of the biggest fixture block, all
// transactions on one page as the REST handler does.
func BenchmarkEthereumGetBlock(b *testing.B) {
	f := setupEthereumBench(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block, err := f.w.GetBlock(f.biggest.Hash, 1, 1000)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := json.Marshal(block); err != nil {
			b.Fatal(err)
		}
	}
	reportPerTx(b, len(f.biggest.Txs))
}
