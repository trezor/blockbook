//go:build unittest

package eth

import (
	"testing"

	"github.com/trezor/blockbook/tests/dbtestdata"
)

// loadBenchPackedTxs packs the blocks from BLOCKBOOK_BENCH_BLOCKS the same way
// ConnectBlock stores them, so the read-path benchmarks start from what RocksDB
// hands back.
func loadBenchPackedTxs(b *testing.B, p *EthereumParser) [][]byte {
	blocks, err := dbtestdata.LoadEthereumBenchBlocks(p)
	if err != nil {
		b.Fatal(err)
	}
	if blocks == nil {
		b.Skipf("%s not set", dbtestdata.EthereumBenchBlocksEnv)
	}
	var packed [][]byte
	for _, block := range blocks {
		for i := range block.Txs {
			buf, err := p.PackTx(&block.Txs[i], block.Height, block.Time)
			if err != nil {
				b.Fatal(err)
			}
			packed = append(packed, buf)
		}
	}
	return packed
}

func reportPerTx(b *testing.B, txs int) {
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*txs), "ns/tx")
}

// BenchmarkEIP55Address is the cache hit; BenchmarkEIP55Checksum the miss.
func BenchmarkEIP55Address(b *testing.B) {
	desc, _ := NewEthereumParser(1, false).GetAddrDescFromAddress("0x2aacf811ac1a60081ea39f7783c0d26c500871a8")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		EIP55Address(desc)
	}
}

func BenchmarkEIP55Checksum(b *testing.B) {
	desc, _ := NewEthereumParser(1, false).GetAddrDescFromAddress("0x2aacf811ac1a60081ea39f7783c0d26c500871a8")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		eip55Checksum(desc)
	}
}

// BenchmarkUnpackTxBlocks measures db.GetTx's parser share: decoding the stored
// transaction back into bchain.Tx, addresses formatted for the API.
func BenchmarkUnpackTxBlocks(b *testing.B) {
	benchUnpackTxBlocks(b, false)
}

// BenchmarkUnpackTxBlocksColdCache empties the address cache before every pass, so a
// pass pays for the first occurrence of every address like a page served after restart.
func BenchmarkUnpackTxBlocksColdCache(b *testing.B) {
	benchUnpackTxBlocks(b, true)
}

func benchUnpackTxBlocks(b *testing.B, coldCache bool) {
	p := NewEthereumParser(1, false)
	packed := loadBenchPackedTxs(b, p)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if coldCache {
			b.StopTimer()
			eip55Cache.reset()
			b.StartTimer()
		}
		for _, buf := range packed {
			if _, _, err := p.UnpackTx(buf); err != nil {
				b.Fatal(err)
			}
		}
	}
	reportPerTx(b, len(packed))
}

// BenchmarkReadPathBlocks replays the parser work of api.GetTransactionFromBchainTx
// for a confirmed EVM transaction: unpack, sender descriptor, recipient descriptor
// and address, token transfers from the receipt logs.
func BenchmarkReadPathBlocks(b *testing.B) {
	benchReadPathBlocks(b, false)
}

func BenchmarkReadPathBlocksColdCache(b *testing.B) {
	benchReadPathBlocks(b, true)
}

func benchReadPathBlocks(b *testing.B, coldCache bool) {
	p := NewEthereumParser(1, false)
	packed := loadBenchPackedTxs(b, p)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if coldCache {
			b.StopTimer()
			eip55Cache.reset()
			b.StartTimer()
		}
		for _, buf := range packed {
			tx, _, err := p.UnpackTx(buf)
			if err != nil {
				b.Fatal(err)
			}
			for j := range tx.Vin {
				if len(tx.Vin[j].Addresses) > 0 {
					p.GetAddrDescFromAddress(tx.Vin[j].Addresses[0])
				}
			}
			for j := range tx.Vout {
				if desc, err := p.GetAddrDescFromVout(&tx.Vout[j]); err == nil {
					p.GetAddressesFromAddrDesc(desc)
				}
			}
			if _, err := p.EthereumTypeGetTokenTransfersFromTx(tx); err != nil {
				b.Fatal(err)
			}
		}
	}
	reportPerTx(b, len(packed))
}
