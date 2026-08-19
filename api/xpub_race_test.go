//go:build unittest

package api

import (
	"os"
	"runtime"
	"sync"
	"testing"

	"github.com/trezor/blockbook/bchain/coins/btc"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

// setupXpubRaceWorker builds a Bitcoin-type worker backed by the shared test
// fixtures (two connected blocks) so getXpubData exercises the real cache path.
func setupXpubRaceWorker(t *testing.T) (*Worker, func()) {
	t.Helper()
	parser := btc.NewBitcoinParser(
		btc.GetChainParams("test"),
		&btc.Configuration{
			BlockAddressesToKeep:  1,
			XPubMagic:             70617039,
			XPubMagicSegwitP2sh:   71979618,
			XPubMagicSegwitNative: 73342198,
			Slip44:                1,
		})
	chain, err := dbtestdata.NewFakeBlockChain(parser)
	if err != nil {
		t.Fatal(err)
	}
	tmp, err := os.MkdirTemp("", "xpubracedb")
	if err != nil {
		t.Fatal(err)
	}
	d, err := db.NewRocksDB(tmp, 100000, -1, parser, nil, false)
	if err != nil {
		os.RemoveAll(tmp)
		t.Fatal(err)
	}
	is, err := d.LoadInternalState(&common.Config{CoinName: "coin-unittest"})
	if err != nil {
		t.Fatal(err)
	}
	d.SetInternalState(is)
	block1 := dbtestdata.GetTestBitcoinTypeBlock1(parser)
	for i := uint32(0); i < block1.Height; i++ {
		is.BlockTimes = append(is.BlockTimes, 0)
	}
	if err := d.ConnectBlock(block1); err != nil {
		t.Fatal(err)
	}
	block2 := dbtestdata.GetTestBitcoinTypeBlock2(parser)
	if err := d.ConnectBlock(block2); err != nil {
		t.Fatal(err)
	}
	is.FinishedSync(block2.Height)

	metrics, err := common.GetMetrics("Fakecoin-xpubrace")
	if err != nil {
		t.Fatal(err)
	}
	mempool, err := chain.CreateMempool(chain)
	if err != nil {
		t.Fatal(err)
	}
	txCache, err := db.NewTxCache(d, chain, metrics, is, false)
	if err != nil {
		t.Fatal(err)
	}
	w, err := NewWorker(d, chain, mempool, txCache, metrics, is, nil)
	if err != nil {
		t.Fatal(err)
	}
	return w, func() {
		d.Close()
		os.RemoveAll(tmp)
	}
}

func resetXpubCache() {
	cachedXpubsMux.Lock()
	cachedXpubs = nil
	cachedXpubsMux.Unlock()
}

// TestGetXpubAddressConcurrent verifies that concurrent requests for the same
// descriptor do not race on the shared cache entry and all observe the same
// txid history. Run with -race: before the copy-on-write / per-descriptor lock
// fix, the concurrent first-time txid load reported write/write and read/write
// data races in getXpubData.
func TestGetXpubAddressConcurrent(t *testing.T) {
	// the race only manifests with real parallelism; ensure it regardless of
	// the runner's default GOMAXPROCS.
	if runtime.GOMAXPROCS(0) < 4 {
		defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(4))
	}

	w, cleanup := setupXpubRaceWorker(t)
	defer cleanup()

	const gap = 1000
	filter := &AddressFilter{Vout: AddressFilterVoutOff}

	// clean sequential baseline (fully populates then reads the cache)
	resetXpubCache()
	base, err := w.GetXpubAddress(dbtestdata.Xpub, 1, 25, AccountDetailsTxidHistory, filter, gap, "")
	if err != nil {
		t.Fatalf("baseline GetXpubAddress: %v", err)
	}
	if base.Txs == 0 || len(base.Txids) == 0 {
		t.Fatalf("baseline has no txids (Txs=%d, txids=%d); fixture/derivation changed", base.Txs, len(base.Txids))
	}

	const (
		workers = 16
		rounds  = 8
	)
	for r := 0; r < rounds; r++ {
		// reset and warm only with a basic call: addresses get scanned but txids
		// are not loaded, so the burst below all races to load them for the first
		// time.
		resetXpubCache()
		if _, err := w.GetXpubAddress(dbtestdata.Xpub, 1, 25, AccountDetailsBasic, filter, gap, ""); err != nil {
			t.Fatalf("round %d warmup: %v", r, err)
		}

		start := make(chan struct{})
		var wg sync.WaitGroup
		results := make([]*Address, workers)
		errs := make([]error, workers)
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				<-start
				results[idx], errs[idx] = w.GetXpubAddress(dbtestdata.Xpub, 1, 25, AccountDetailsTxidHistory, filter, gap, "")
			}(i)
		}
		close(start)
		wg.Wait()

		for i := 0; i < workers; i++ {
			if errs[i] != nil {
				t.Fatalf("round %d worker %d: %v", r, i, errs[i])
			}
			got := results[i]
			if got.Txs != base.Txs {
				t.Errorf("round %d worker %d: Txs=%d, want %d", r, i, got.Txs, base.Txs)
			}
			if len(got.Txids) != len(base.Txids) {
				t.Errorf("round %d worker %d: %d txids, want %d", r, i, len(got.Txids), len(base.Txids))
				continue
			}
			for j := range got.Txids {
				if got.Txids[j] != base.Txids[j] {
					t.Errorf("round %d worker %d txid %d: %s, want %s", r, i, j, got.Txids[j], base.Txids[j])
				}
			}
		}
	}
}
