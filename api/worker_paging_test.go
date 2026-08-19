//go:build unittest

package api

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

// pagingMempool serves the same entries for every address, which is enough to put a
// pending tx in front of the confirmed history of the tested address.
type pagingMempool struct {
	entries []bchain.Outpoint
}

func (m *pagingMempool) Resync() (int, error) { return len(m.entries), nil }
func (m *pagingMempool) GetTransactions(address string) ([]bchain.Outpoint, error) {
	return m.entries, nil
}
func (m *pagingMempool) GetAddrDescTransactions(addrDesc bchain.AddressDescriptor) ([]bchain.Outpoint, error) {
	return m.entries, nil
}
func (m *pagingMempool) GetAllEntries() bchain.MempoolTxidEntries { return nil }
func (m *pagingMempool) GetTransactionTime(txid string) uint32    { return 0 }
func (m *pagingMempool) GetTxidFilterEntries(filterScripts string, fromTimestamp uint32) (bchain.MempoolTxidFilterEntries, error) {
	return bchain.MempoolTxidFilterEntries{}, nil
}

// pagingChain serves the pending txs, which are not in the index and are therefore
// loaded from the backend.
type pagingChain struct {
	bchain.BlockChain
	txs map[string]*bchain.Tx
}

func (c *pagingChain) GetTransaction(txid string) (*bchain.Tx, error) {
	if tx, found := c.txs[txid]; found {
		return tx, nil
	}
	return nil, bchain.ErrTxNotFound
}

func (c *pagingChain) GetTransactionSpecific(tx *bchain.Tx) (json.RawMessage, error) {
	if _, found := c.txs[tx.Txid]; found {
		return json.RawMessage("{}"), nil
	}
	return c.BlockChain.GetTransactionSpecific(tx)
}

// buildPagingWorker indexes confirmedTxs blocks, each with one tx paying Addr1, and puts
// pendingTxs txs for the same address in the mempool. Returns the worker and the txids of
// the pending and the confirmed txs, both newest first - the order the API returns them in.
func buildPagingWorker(t *testing.T, confirmedTxs, pendingTxs int) (*Worker, []string, []string, func()) {
	t.Helper()
	parser := btc.NewBitcoinParser(btc.GetChainParams("test"), &btc.Configuration{BlockAddressesToKeep: 1})
	script := dbtestdata.AddressToPubKeyHex(dbtestdata.Addr1, parser)

	blocks := make([]*bchain.Block, 0, confirmedTxs)
	confirmed := make([]string, 0, confirmedTxs)
	for i := 1; i <= confirmedTxs; i++ {
		tx := fanInTx(fanInHash(uint64(i)), uint32(i), []bchain.Vin{{Coinbase: "01", Sequence: 0xffffffff}}, []bchain.Vout{fanInVout(0, script)})
		blocks = append(blocks, &bchain.Block{
			BlockHeader: bchain.BlockHeader{Hash: fanInHash(uint64(1_000_000 + i)), Height: uint32(i), Time: int64(1_700_000_000 + i)},
			Txs:         []bchain.Tx{*tx},
		})
		// newest first
		confirmed = append([]string{tx.Txid}, confirmed...)
	}

	mempool := &pagingMempool{}
	base, err := dbtestdata.NewFakeBlockChain(parser)
	require.NoError(t, err)
	chain := &pagingChain{BlockChain: base, txs: make(map[string]*bchain.Tx)}
	pending := make([]string, 0, pendingTxs)
	for i := 1; i <= pendingTxs; i++ {
		txid := fanInHash(uint64(2_000_000 + i))
		chain.txs[txid] = &bchain.Tx{
			Txid:          txid,
			Version:       1,
			Vin:           []bchain.Vin{{Coinbase: "01", Sequence: 0xffffffff}},
			Vout:          []bchain.Vout{fanInVout(0, script)},
			Confirmations: 0,
			Time:          int64(1_800_000_000 + i),
			Blocktime:     int64(1_800_000_000 + i),
		}
		mempool.entries = append(mempool.entries, bchain.Outpoint{Txid: txid, Vout: 0})
		pending = append(pending, txid)
	}

	tmp, err := os.MkdirTemp("", "account-paging")
	require.NoError(t, err)
	database, err := db.NewRocksDB(tmp, 100000, -1, parser, nil, false)
	require.NoError(t, err)
	cleanup := func() {
		require.NoError(t, database.Close())
		require.NoError(t, os.RemoveAll(tmp))
	}

	is, err := database.LoadInternalState(&common.Config{CoinName: "coin-unittest"})
	require.NoError(t, err)
	database.SetInternalState(is)

	bulk, err := database.InitBulkConnect()
	require.NoError(t, err)
	for i, block := range blocks {
		require.NoError(t, bulk.ConnectBlock(block, i == len(blocks)-1))
	}
	require.NoError(t, bulk.Close())
	is.FinishedSync(uint32(len(blocks)))

	// caching is switched off, so pending txs are always served by pagingChain
	txCache, err := db.NewTxCache(database, chain, newRefreshTestMetrics(t), is, false)
	require.NoError(t, err)

	w := &Worker{
		db:          database,
		chain:       chain,
		chainParser: parser,
		chainType:   bchain.ChainBitcoinType,
		mempool:     mempool,
		txCache:     txCache,
		is:          is,
	}
	return w, pending, confirmed, cleanup
}

// TestGetAddressPagesMempoolWithConfirmed is the regression test for issue #1099: a page
// must never be longer than requested, and walking the pages must return every tx once.
func TestGetAddressPagesMempoolWithConfirmed(t *testing.T) {
	const confirmedTxs, pendingTxs, txsOnPage = 3, 1, 2
	w, pending, confirmed, cleanup := buildPagingWorker(t, confirmedTxs, pendingTxs)
	defer cleanup()

	want := append(append([]string{}, pending...), confirmed...)
	filter := &AddressFilter{Vout: AddressFilterVoutOff}

	var walked []string
	totalPages := 0
	for page := 1; ; page++ {
		addr, err := w.GetAddress(dbtestdata.Addr1, page, txsOnPage, AccountDetailsTxidHistory, filter, "")
		require.NoError(t, err)
		require.LessOrEqual(t, len(addr.Txids), txsOnPage,
			"page %d returned %d txids, more than the requested pageSize %d", page, len(addr.Txids), txsOnPage)
		walked = append(walked, addr.Txids...)
		totalPages = addr.TotalPages
		if page >= addr.TotalPages {
			break
		}
	}

	require.Equal(t, 2, totalPages, "pending txs must be counted into the total page count")
	require.Equal(t, want, walked, "walking all pages must return every tx exactly once, pending first")
}

// TestGetAddressPagesWithoutMempoolUnchanged pins the paging of an account with no pending
// txs, which must stay exactly as it was before mempool entries joined the paged sequence.
func TestGetAddressPagesWithoutMempoolUnchanged(t *testing.T) {
	const confirmedTxs, txsOnPage = 5, 2
	w, _, confirmed, cleanup := buildPagingWorker(t, confirmedTxs, 0)
	defer cleanup()

	filter := &AddressFilter{Vout: AddressFilterVoutOff}
	var walked []string
	for page := 1; ; page++ {
		addr, err := w.GetAddress(dbtestdata.Addr1, page, txsOnPage, AccountDetailsTxidHistory, filter, "")
		require.NoError(t, err)
		require.LessOrEqual(t, len(addr.Txids), txsOnPage)
		require.Equal(t, 3, addr.TotalPages)
		walked = append(walked, addr.Txids...)
		if page >= addr.TotalPages {
			break
		}
	}
	require.Equal(t, confirmed, walked)
}

// TestGetAddressMempoolOnlyAccount covers an address that is only in the mempool, where
// there is no confirmed history to page against.
func TestGetAddressMempoolOnlyAccount(t *testing.T) {
	w, pending, _, cleanup := buildPagingWorker(t, 0, 2)
	defer cleanup()

	addr, err := w.GetAddress(dbtestdata.Addr1, 1, 25, AccountDetailsTxidHistory, &AddressFilter{Vout: AddressFilterVoutOff}, "")
	require.NoError(t, err)
	require.Equal(t, pending, addr.Txids)
	require.Equal(t, 2, addr.UnconfirmedTxs)
}

// buildXpubPagingWorker indexes the shared bitcoin-type test blocks, which contain
// addresses derived from dbtestdata.Xpub, and puts one pending tx in the mempool.
func buildXpubPagingWorker(t *testing.T) (*Worker, string, func()) {
	t.Helper()
	// the xpub magics are what makes the derived addresses match the indexed test data
	parser := btc.NewBitcoinParser(btc.GetChainParams("test"), &btc.Configuration{
		BlockAddressesToKeep:  1,
		XPubMagic:             70617039,
		XPubMagicSegwitP2sh:   71979618,
		XPubMagicSegwitNative: 73342198,
		Slip44:                1,
	})

	base, err := dbtestdata.NewFakeBlockChain(parser)
	require.NoError(t, err)
	chain := &pagingChain{BlockChain: base, txs: make(map[string]*bchain.Tx)}
	pendingTxid := fanInHash(3_000_001)
	chain.txs[pendingTxid] = &bchain.Tx{
		Txid:          pendingTxid,
		Version:       1,
		Vin:           []bchain.Vin{{Coinbase: "01", Sequence: 0xffffffff}},
		Vout:          []bchain.Vout{fanInVout(0, dbtestdata.AddressToPubKeyHex(dbtestdata.Addr4, parser))},
		Confirmations: 0,
		Time:          1_800_000_000,
		Blocktime:     1_800_000_000,
	}
	mempool := &pagingMempool{entries: []bchain.Outpoint{{Txid: pendingTxid, Vout: 0}}}

	tmp, err := os.MkdirTemp("", "xpub-paging")
	require.NoError(t, err)
	database, err := db.NewRocksDB(tmp, 100000, -1, parser, nil, false)
	require.NoError(t, err)
	cleanup := func() {
		require.NoError(t, database.Close())
		require.NoError(t, os.RemoveAll(tmp))
	}

	is, err := database.LoadInternalState(&common.Config{CoinName: "coin-unittest"})
	require.NoError(t, err)
	database.SetInternalState(is)

	bulk, err := database.InitBulkConnect()
	require.NoError(t, err)
	require.NoError(t, bulk.ConnectBlock(dbtestdata.GetTestBitcoinTypeBlock1(parser), false))
	require.NoError(t, bulk.ConnectBlock(dbtestdata.GetTestBitcoinTypeBlock2(parser), true))
	require.NoError(t, bulk.Close())
	is.FinishedSync(225494)

	metrics := newRefreshTestMetrics(t)
	txCache, err := db.NewTxCache(database, chain, metrics, is, false)
	require.NoError(t, err)

	// the xpub cache is package global, so it must not leak between tests
	cachedXpubsMux.Lock()
	cachedXpubs = nil
	cachedXpubsMux.Unlock()

	w := &Worker{
		db:          database,
		chain:       chain,
		chainParser: parser,
		chainType:   bchain.ChainBitcoinType,
		mempool:     mempool,
		txCache:     txCache,
		is:          is,
		metrics:     metrics,
		xpubConfig:  DefaultXpubConfig(),
	}
	return w, pendingTxid, cleanup
}

// TestGetXpubAddressPagesMempoolWithConfirmed is the xpub endpoint from issue #1099: with a
// pending tx, pageSize was exceeded and a confirmed tx fell between page 1 and page 2.
func TestGetXpubAddressPagesMempoolWithConfirmed(t *testing.T) {
	const txsOnPage = 1
	w, pendingTxid, cleanup := buildXpubPagingWorker(t)
	defer cleanup()

	// the confirmed history alone, in the order the API returns it
	confirmedOnly, err := w.GetXpubAddress(dbtestdata.Xpub, 1, 100, AccountDetailsTxidHistory,
		&AddressFilter{Vout: AddressFilterVoutOff, OnlyConfirmed: true}, 5, "")
	require.NoError(t, err)
	require.NotEmpty(t, confirmedOnly.Txids, "test data must give the xpub some confirmed txs")
	want := append([]string{pendingTxid}, confirmedOnly.Txids...)

	var walked []string
	for page := 1; ; page++ {
		addr, err := w.GetXpubAddress(dbtestdata.Xpub, page, txsOnPage, AccountDetailsTxidHistory,
			&AddressFilter{Vout: AddressFilterVoutOff}, 5, "")
		require.NoError(t, err)
		require.LessOrEqual(t, len(addr.Txids), txsOnPage,
			"page %d returned %d txids, more than the requested pageSize %d", page, len(addr.Txids), txsOnPage)
		walked = append(walked, addr.Txids...)
		if page >= addr.TotalPages {
			require.Equal(t, len(want), addr.TotalPages*txsOnPage, "pending txs must be counted into the total page count")
			break
		}
	}
	require.Equal(t, want, walked, "walking all pages must return every tx exactly once, pending first")
}
