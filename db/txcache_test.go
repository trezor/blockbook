//go:build unittest

package db

import (
	"testing"

	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
	"github.com/trezor/blockbook/common"
)

// confirmationsStubParser packs just enough to round-trip a txid at a fixed height, so the
// test does not depend on a complete Ethereum transaction fixture.
type confirmationsStubParser struct {
	*eth.EthereumParser
	height uint32
}

func (p *confirmationsStubParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	return []byte(tx.Txid), nil
}

func (p *confirmationsStubParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	return &bchain.Tx{Txid: string(buf)}, p.height, nil
}

// Ethereum-type transactions are cached at their chain height, which can be above the
// height Blockbook has indexed. Recomputing confirmations on a cache hit must then not
// underflow the unsigned subtraction into a bogus finality count.
func TestTxCacheGetTransaction_HeightAboveIndexedBestHeight(t *testing.T) {
	const txHeight = 18000010
	const txid = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"

	parser := &confirmationsStubParser{EthereumParser: eth.NewEthereumParser(1, true), height: txHeight}
	d := setupRocksDB(t, parser)
	defer closeAndDestroyRocksDB(t, d)

	metrics, err := common.GetMetrics("coin-unittest-txcache")
	if err != nil {
		t.Fatal(err)
	}
	c := &TxCache{db: d, metrics: metrics, is: d.is, enabled: true, chainType: bchain.ChainEthereumType}

	if err := d.PutTx(&bchain.Tx{Txid: txid}, txHeight, 1600000000); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		bestHeight uint32
		want       uint32
	}{
		{"indexed height above tx", txHeight + 4, 5},
		{"indexed height at tx", txHeight, 1},
		{"indexed height behind by 1 block", txHeight - 1, 1},
		{"indexed height behind by many blocks", txHeight - 300, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d.is.BestHeight = tt.bestHeight
			tx, h, err := c.GetTransaction(txid)
			if err != nil {
				t.Fatalf("GetTransaction error %v", err)
			}
			if h != txHeight {
				t.Errorf("GetTransaction height = %d, want %d", h, txHeight)
			}
			if tx.Confirmations != tt.want {
				t.Errorf("Confirmations with best height %d and tx height %d = %d, want %d",
					tt.bestHeight, txHeight, tx.Confirmations, tt.want)
			}
		})
	}
}
