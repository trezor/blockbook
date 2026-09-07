//go:build unittest

package api

import (
	"testing"

	"github.com/trezor/blockbook/bchain/coins/btc"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

// setupSpendingWorker builds a worker over the two-block Fakecoin fixture. The
// fixture already contains the shape this test needs: TxidB1T1 pays the same
// SatB1T1A2 to Addr2 twice (vout 1 and vout 2) and only vout 1 is ever spent.
func setupSpendingWorker(t *testing.T, extendedIndex bool) *Worker {
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
	d, err := db.NewRocksDB(t.TempDir(), 100000, -1, parser, nil, extendedIndex)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	is, err := d.LoadInternalState(&common.Config{CoinName: "Fakecoin", CoinShortcut: "FAKE"})
	if err != nil {
		t.Fatal(err)
	}
	d.SetInternalState(is)

	block1 := dbtestdata.GetTestBitcoinTypeBlock1(parser)
	block2 := dbtestdata.GetTestBitcoinTypeBlock2(parser)
	// BlockTimes is indexed by height, so pad up to the first fixture block
	is.BlockTimes = make([]uint32, block1.Height)
	if err := d.ConnectBlock(block1); err != nil {
		t.Fatal(err)
	}
	if err := d.ConnectBlock(block2); err != nil {
		t.Fatal(err)
	}
	is.FinishedSync(block2.Height)

	m := newRefreshTestMetrics(t)
	// caching disabled, as in server/public_test.go: the fixture txs carry no Hex,
	// which BitcoinLikeParser.PackTx needs, so a cached tx cannot be read back
	txCache, err := db.NewTxCache(d, chain, m, is, false)
	if err != nil {
		t.Fatal(err)
	}
	// nil mempool: GetSpendingTxid never touches it for confirmed transactions
	w, err := NewWorker(d, chain, nil, txCache, m, is, nil)
	if err != nil {
		t.Fatal(err)
	}
	return w
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
			w := setupSpendingWorker(t, extendedIndex)

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

			// bypass the unspent guard in GetSpendingTxid to pin the outpoint match
			// itself: TxidB2T1's input matches vout 2 on txid, address and value, and
			// only its outpoint (vout 1) tells the two sibling outputs apart
			t.Run("scan rejects a candidate spending a sibling outpoint", func(t *testing.T) {
				tx, err := w.getTransaction(dbtestdata.TxidB1T1, false, false, nil)
				if err != nil {
					t.Fatal(err)
				}
				if err := w.setSpendingTxToVout(&tx.Vout[2], tx.Txid, uint32(tx.Blockheight)); err != nil {
					t.Fatal(err)
				}
				if tx.Vout[2].SpentTxID != "" {
					t.Errorf("setSpendingTxToVout matched %q for unspent vout 2, want no match", tx.Vout[2].SpentTxID)
				}
			})
		})
	}
}
