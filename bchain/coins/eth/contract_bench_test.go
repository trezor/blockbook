//go:build unittest

package eth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/trezor/blockbook/bchain"
)

const benchPaddedTopic = "0x0000000000000000000000002aacf811ac1a60081ea39f7783c0d26c500871a8"

func randomPaddedTopic() string {
	var b [20]byte
	_, _ = rand.Read(b[:])
	return "0x000000000000000000000000" + hex.EncodeToString(b[:])
}

// syntheticTransferLogs builds n ERC-20 Transfer logs with distinct random
// addresses so the parse cost is not skewed by allocator or cache reuse.
func syntheticTransferLogs(n int) []*bchain.RpcLog {
	logs := make([]*bchain.RpcLog, n)
	for i := range logs {
		logs[i] = &bchain.RpcLog{
			Address: randomPaddedTopic()[26:],
			Topics:  []string{tokenTransferEventSignature, randomPaddedTopic(), randomPaddedTopic()},
			Data:    "0x00000000000000000000000000000000000000000000000000000000000f4240",
		}
	}
	return logs
}

// loadBenchReceipts reads an eth_getBlockReceipts response saved to the path in
// BLOCKBOOK_BENCH_RECEIPTS so the block-level benchmark can run on a real block.
func loadBenchReceipts(b *testing.B) []*bchain.RpcReceipt {
	path := os.Getenv("BLOCKBOOK_BENCH_RECEIPTS")
	if path == "" {
		b.Skip("BLOCKBOOK_BENCH_RECEIPTS not set")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	var resp struct {
		Result []*bchain.RpcReceipt `json:"result"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		b.Fatal(err)
	}
	return resp.Result
}

func BenchmarkAddressFromPaddedHex(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := addressFromPaddedHex(benchPaddedTopic); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProcessTransferEvent(b *testing.B) {
	logs := syntheticTransferLogs(1024)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := processTransferEvent(logs[i%len(logs)]); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkContractGetTransfersFromLogSynthetic measures a synthetic block of
// 1000 ERC-20 transfers, roughly a busy BSC block.
func BenchmarkContractGetTransfersFromLogSynthetic(b *testing.B) {
	logs := syntheticTransferLogs(1000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		contractGetTransfersFromLog(logs, "bench")
	}
}

// BenchmarkContractGetTransfersFromLogBlock measures the log-parsing work the
// indexer does for one real block, receipt by receipt as during sync.
func BenchmarkContractGetTransfersFromLogBlock(b *testing.B) {
	receipts := loadBenchReceipts(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, r := range receipts {
			contractGetTransfersFromLog(r.Logs, "bench")
		}
	}
}
