//go:build unittest

package db

import (
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/linxGnu/grocksdb"
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

// The best height drops before a disconnect commits, so a cached transaction can briefly sit
// above it. Recomputing confirmations on a cache hit must then not underflow the unsigned
// subtraction into a bogus finality count.
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

// txCacheTestChain serves one canned backend transaction to TxCache.
type txCacheTestChain struct {
	bchain.BlockChain
	tx *bchain.Tx
}

func (c *txCacheTestChain) GetTransaction(txid string) (*bchain.Tx, error) {
	return c.tx, nil
}

// indexedStubParser is confirmationsStubParser with a hook that runs inside PutTx, which is
// where a concurrent disconnect can land between the index check and the cache write.
type indexedStubParser struct {
	confirmationsStubParser
	onPack func()
}

func (p *indexedStubParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	if p.onPack != nil {
		p.onPack()
	}
	return p.confirmationsStubParser.PackTx(tx, height, blockTime)
}

func putTestBlockHash(t *testing.T, d *RocksDB, height uint32, hash string) {
	t.Helper()
	wb := grocksdb.NewWriteBatch()
	defer wb.Destroy()
	if err := d.writeHeight(wb, height, &BlockInfo{Hash: hash, Height: height}, opInsert); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteBatch(wb); err != nil {
		t.Fatal(err)
	}
}

func newEthTestTx(txid, blockHash string, height uint32, withReceipt bool) *bchain.Tx {
	csd := bchain.EthereumSpecificData{
		Tx: &bchain.RpcTransaction{Hash: txid, BlockNumber: hexutil.EncodeUint64(uint64(height)), BlockHash: blockHash},
	}
	if withReceipt {
		csd.Receipt = &bchain.RpcReceipt{}
	}
	return &bchain.Tx{Txid: txid, Blocktime: 1600000000, Confirmations: 3, CoinSpecificData: csd}
}

func TestTxCacheGetTransaction_CachesOnlyIndexedEthereumTxs(t *testing.T) {
	const height = 18000010
	const txid = "0x0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
	const indexedHash = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const otherHash = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	tests := []struct {
		name   string
		tx     *bchain.Tx
		cached bool
	}{
		{"block hash matches the index", newEthTestTx(txid, indexedHash, height, true), true},
		{"block hash differs from the index", newEthTestTx(txid, otherHash, height, true), false},
		{"height above the index", newEthTestTx(txid, indexedHash, height+1, true), false},
		{"tron tx without block hash", newEthTestTx(txid, "", height, true), true},
		{"tron unsolidified tx without receipt", newEthTestTx(txid, "", height, false), false},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := &confirmationsStubParser{EthereumParser: eth.NewEthereumParser(1, true), height: height}
			d := setupRocksDB(t, parser)
			defer closeAndDestroyRocksDB(t, d)
			putTestBlockHash(t, d, height, indexedHash)
			metrics, err := common.GetMetrics(fmt.Sprintf("coin-unittest-txcache-indexed-%d", i))
			if err != nil {
				t.Fatal(err)
			}
			c := &TxCache{db: d, chain: &txCacheTestChain{tx: tt.tx}, metrics: metrics, is: d.is, enabled: true, chainType: bchain.ChainEthereumType}

			if _, _, err := c.GetTransaction(txid); err != nil {
				t.Fatalf("GetTransaction error %v", err)
			}
			cached, _, err := d.GetTx(txid)
			if err != nil {
				t.Fatal(err)
			}
			if (cached != nil) != tt.cached {
				t.Errorf("cached = %v, want %v", cached != nil, tt.cached)
			}
		})
	}
}

// A disconnect that commits after the index check but before PutTx evicts nothing, the
// second check after the write must drop the entry instead.
func TestTxCacheGetTransaction_DropsTxDisconnectedDuringPut(t *testing.T) {
	const height = 18000010
	const txid = "0x0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
	const indexedHash = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const reorgedHash = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	parser := &indexedStubParser{confirmationsStubParser: confirmationsStubParser{EthereumParser: eth.NewEthereumParser(1, true), height: height}}
	d := setupRocksDB(t, parser)
	defer closeAndDestroyRocksDB(t, d)
	putTestBlockHash(t, d, height, indexedHash)
	parser.onPack = func() { putTestBlockHash(t, d, height, reorgedHash) }
	metrics, err := common.GetMetrics("coin-unittest-txcache-disconnect")
	if err != nil {
		t.Fatal(err)
	}
	tx := newEthTestTx(txid, indexedHash, height, true)
	c := &TxCache{db: d, chain: &txCacheTestChain{tx: tx}, metrics: metrics, is: d.is, enabled: true, chainType: bchain.ChainEthereumType}

	if _, _, err := c.GetTransaction(txid); err != nil {
		t.Fatalf("GetTransaction error %v", err)
	}
	cached, _, err := d.GetTx(txid)
	if err != nil {
		t.Fatal(err)
	}
	if cached != nil {
		t.Error("tx of the disconnected block stayed in the cache")
	}
}

func TestTxCacheGetTransaction_SkipsBitcoinTxWithoutTxAddresses(t *testing.T) {
	const txid = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"

	d := setupRocksDB(t, bitcoinTestnetParser())
	defer closeAndDestroyRocksDB(t, d)
	metrics, err := common.GetMetrics("coin-unittest-txcache-btc")
	if err != nil {
		t.Fatal(err)
	}
	tx := &bchain.Tx{Txid: txid, BlockHeight: 800000, Blocktime: 1600000000, Confirmations: 3}
	c := &TxCache{db: d, chain: &txCacheTestChain{tx: tx}, metrics: metrics, is: d.is, enabled: true, chainType: bchain.ChainBitcoinType}

	_, h, err := c.GetTransaction(txid)
	if err != nil {
		t.Fatalf("GetTransaction error %v", err)
	}
	if h != 800000 {
		t.Errorf("height = %d, want 800000", h)
	}
	cached, _, err := d.GetTx(txid)
	if err != nil {
		t.Fatal(err)
	}
	if cached != nil {
		t.Error("tx without txAddresses was cached")
	}
}
