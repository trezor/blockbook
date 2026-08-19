package api

import (
	"os"
	"testing"

	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

// setupSpendingWorker builds a worker over the two-block Fakecoin fixture. The
// fixture already contains the shape this test needs: TxidB1T1 pays the same
// SatB1T1A2 to Addr2 twice (vout 1 and vout 2) and only vout 1 is ever spent.
func setupSpendingWorker(t *testing.T, extendedIndex bool) (*Worker, func()) {
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
	tmp, err := os.MkdirTemp("", "testdb-spending")
	if err != nil {
		t.Fatal(err)
	}
	d, err := db.NewRocksDB(tmp, 100000, -1, parser, nil, extendedIndex)
	if err != nil {
		t.Fatal(err)
	}
	cleanup := func() {
		d.Close()
		os.RemoveAll(tmp)
	}
	config := &common.Config{CoinName: "Fakecoin", CoinShortcut: "FAKE"}
	is, err := d.LoadInternalState(config)
	if err != nil {
		cleanup()
		t.Fatal(err)
	}
	d.SetInternalState(is)

	bestHeight, err := chain.GetBestBlockHeight()
	if err != nil {
		cleanup()
		t.Fatal(err)
	}
	block1, err := chain.GetBlock("", bestHeight-1)
	if err != nil {
		cleanup()
		t.Fatal(err)
	}
	for i := uint32(0); i < block1.Height; i++ {
		is.BlockTimes = append(is.BlockTimes, 0)
	}
	if err := d.ConnectBlock(block1); err != nil {
		cleanup()
		t.Fatal(err)
	}
	block2, err := chain.GetBlock("", bestHeight)
	if err != nil {
		cleanup()
		t.Fatal(err)
	}
	if err := d.ConnectBlock(block2); err != nil {
		cleanup()
		t.Fatal(err)
	}
	is.FinishedSync(block2.Height)

	// the tx cache dereferences metrics unconditionally, so they must be real;
	// the name must be unique or the prometheus registration collides
	m, err := common.GetMetrics("Fakecoin-api-spending-" + t.Name())
	if err != nil {
		cleanup()
		t.Fatal(err)
	}
	// caching disabled, as in server/public_test.go: the fixture txs carry no Hex,
	// which BitcoinLikeParser.PackTx needs, so a cached tx cannot be read back
	txCache, err := db.NewTxCache(d, chain, m, is, false)
	if err != nil {
		cleanup()
		t.Fatal(err)
	}
	w, err := NewWorker(d, chain, bchain.NewMempoolBitcoinType(chain, 1, 1, 0, "", false, 1), txCache, m, is, nil)
	if err != nil {
		cleanup()
		t.Fatal(err)
	}
	return w, cleanup
}

// TestGetSpendingTxid covers trezor/blockbook#1029: an output must not inherit
// the spender of a sibling output that happens to share its address and value.
func TestGetSpendingTxid(t *testing.T) {
	for _, extendedIndex := range []bool{false, true} {
		name := "legacyIndex"
		if extendedIndex {
			name = "extendedIndex"
		}
		t.Run(name, func(t *testing.T) {
			w, cleanup := setupSpendingWorker(t, extendedIndex)
			defer cleanup()

			tests := []struct {
				name string
				txid string
				n    int
				want string
			}{
				{
					// spent by TxidB2T1 input 1
					name: "spent output resolves to its spender",
					txid: dbtestdata.TxidB1T1,
					n:    1,
					want: dbtestdata.TxidB2T1,
				},
				{
					// same address and same value as vout 1, but unspent: the
					// address+value scan used to match TxidB2T1 here
					name: "unspent sibling of a spent output is not spent",
					txid: dbtestdata.TxidB1T1,
					n:    2,
					want: "",
				},
				{
					name: "unspent output of another tx is not spent",
					txid: dbtestdata.TxidB1T1,
					n:    0,
					want: "",
				},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					got, err := w.GetSpendingTxid(tt.txid, tt.n)
					if err != nil {
						t.Fatalf("GetSpendingTxid(%s, %d) error %v", tt.txid, tt.n, err)
					}
					if got != tt.want {
						t.Errorf("GetSpendingTxid(%s, %d) = %q, want %q", tt.txid, tt.n, got, tt.want)
					}
				})
			}

			if _, err := w.GetSpendingTxid(dbtestdata.TxidB1T1, 99); err == nil {
				t.Error("GetSpendingTxid with out-of-range vout: expected an error")
			}
		})
	}
}
