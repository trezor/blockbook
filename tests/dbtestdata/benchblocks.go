package dbtestdata

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/trezor/blockbook/bchain"
)

// EthereumBenchParser is the parser surface needed to turn raw RPC blocks into bchain.Blocks.
type EthereumBenchParser interface {
	bchain.BlockChainParser
	EthTxToTx(tx *bchain.RpcTransaction, receipt *bchain.RpcReceipt, internalData *bchain.EthereumInternalData, blocktime int64, confirmations uint32, fixEIP55 bool) (*bchain.Tx, error)
}

// EthereumBenchBlocksEnv names the directory with block<N>.json and receipts<N>.json,
// raw eth_getBlockByNumber(full txs) and eth_getBlockReceipts responses of consecutive
// blocks, so benchmarks can run on real chain data without committing megabytes of it.
const EthereumBenchBlocksEnv = "BLOCKBOOK_BENCH_BLOCKS"

// LoadEthereumBenchBlocks converts the saved RPC responses into blocks the way the
// indexer does before ConnectBlock. It returns nil when the env var is not set.
func LoadEthereumBenchBlocks(parser EthereumBenchParser) ([]*bchain.Block, error) {
	dir := os.Getenv(EthereumBenchBlocksEnv)
	if dir == "" {
		return nil, nil
	}
	var blocks []*bchain.Block
	for i := 0; ; i++ {
		raw, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("block%d.json", i)))
		if err != nil {
			break
		}
		var block struct {
			Result struct {
				Number       string                   `json:"number"`
				Hash         string                   `json:"hash"`
				Timestamp    string                   `json:"timestamp"`
				Transactions []*bchain.RpcTransaction `json:"transactions"`
			} `json:"result"`
		}
		if err := json.Unmarshal(raw, &block); err != nil {
			return nil, err
		}
		raw, err = os.ReadFile(filepath.Join(dir, fmt.Sprintf("receipts%d.json", i)))
		if err != nil {
			return nil, err
		}
		var receipts struct {
			Result []*bchain.RpcReceipt `json:"result"`
		}
		if err := json.Unmarshal(raw, &receipts); err != nil {
			return nil, err
		}
		if len(receipts.Result) != len(block.Result.Transactions) {
			return nil, fmt.Errorf("block %d: %d txs, %d receipts", i, len(block.Result.Transactions), len(receipts.Result))
		}
		height, err := strconv.ParseUint(block.Result.Number[2:], 16, 32)
		if err != nil {
			return nil, err
		}
		blockTime, err := strconv.ParseInt(block.Result.Timestamp[2:], 16, 64)
		if err != nil {
			return nil, err
		}
		b := &bchain.Block{
			BlockHeader: bchain.BlockHeader{
				Height:        uint32(height),
				Hash:          block.Result.Hash,
				Time:          blockTime,
				Confirmations: 1,
			},
			Txs:              make([]bchain.Tx, len(block.Result.Transactions)),
			CoinSpecificData: &bchain.EthereumBlockSpecificData{},
		}
		for j, tx := range block.Result.Transactions {
			btx, err := parser.EthTxToTx(tx, receipts.Result[j], nil, blockTime, 1, true)
			if err != nil {
				return nil, err
			}
			b.Txs[j] = *btx
		}
		blocks = append(blocks, b)
	}
	if len(blocks) == 0 {
		return nil, fmt.Errorf("no block0.json in %s", dir)
	}
	return blocks, nil
}
