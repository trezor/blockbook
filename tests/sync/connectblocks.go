//go:build integration

package sync

import (
	"math/big"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/db"
)

func testConnectBlocks(t *testing.T, h *TestHandler) {
	for _, rng := range h.TestData.ConnectBlocks.SyncRanges {
		withRocksDBAndSyncWorker(t, h, rng.Lower, func(d *db.RocksDB, sw *db.SyncWorker, ch chan os.Signal) {
			upperHash, err := h.Chain.GetBlockHash(rng.Upper)
			if err != nil {
				t.Fatal(err)
			}

			err = db.ConnectBlocks(sw, func(hash string, height uint32) {
				if hash == upperHash {
					close(ch)
				}
			}, true)
			if err != nil && err != db.ErrOperationInterrupted {
				t.Fatal(err)
			}

			height, _, err := d.GetBestBlock()
			if err != nil {
				t.Fatal(err)
			}
			if height < rng.Upper {
				t.Fatalf("Best block height mismatch: %d < %d", height, rng.Upper)
			}

			t.Run("verifyBlockInfo", func(t *testing.T) { verifyBlockInfo(t, d, h, rng) })
			t.Run("verifyTransactions", func(t *testing.T) { verifyTransactions(t, d, h, rng) })
			t.Run("verifyAddresses", func(t *testing.T) { verifyAddresses(t, d, h, rng) })
		})
	}
}

func testConnectBlocksParallel(t *testing.T, h *TestHandler) {
	for _, rng := range h.TestData.ConnectBlocks.SyncRanges {
		withRocksDBAndSyncWorker(t, h, rng.Lower, func(d *db.RocksDB, sw *db.SyncWorker, ch chan os.Signal) {
			upperHash, err := h.Chain.GetBlockHash(rng.Upper)
			if err != nil {
				t.Fatal(err)
			}

			err = sw.ConnectBlocksParallel(rng.Lower, rng.Upper)
			if err != nil {
				t.Fatal(err)
			}

			height, hash, err := d.GetBestBlock()
			if err != nil {
				t.Fatal(err)
			}
			if height != rng.Upper {
				t.Fatalf("Best block height mismatch: %d != %d", height, rng.Upper)
			}
			if hash != upperHash {
				t.Fatalf("Best block hash mismatch: %s != %s", hash, upperHash)
			}

			t.Run("verifyBlockInfo", func(t *testing.T) { verifyBlockInfo(t, d, h, rng) })
			t.Run("verifyTransactions", func(t *testing.T) { verifyTransactions(t, d, h, rng) })
			t.Run("verifyAddresses", func(t *testing.T) { verifyAddresses(t, d, h, rng) })
		})
	}
}

func verifyBlockInfo(t *testing.T, d *db.RocksDB, h *TestHandler, rng Range) {
	for height := rng.Lower; height <= rng.Upper; height++ {
		block, found := h.TestData.ConnectBlocks.Blocks[height]
		if !found {
			continue
		}

		bi, err := d.GetBlockInfo(height)
		if err != nil {
			t.Errorf("GetBlockInfo(%d) error: %s", height, err)
			continue
		}
		if bi == nil {
			t.Errorf("GetBlockInfo(%d) returned nil", height)
			continue
		}

		if bi.Hash != block.Hash {
			t.Errorf("Block hash mismatch: %s != %s", bi.Hash, block.Hash)
		}

		if bi.Txs != block.NoTxs {
			t.Errorf("Number of transactions in block %s mismatch: %d != %d", bi.Hash, bi.Txs, block.NoTxs)
		}
	}
}

func verifyTransactions(t *testing.T, d *db.RocksDB, h *TestHandler, rng Range) {
	type txInfo struct {
		txid  string
		index int32
	}
	addr2txs := make(map[string][]txInfo)
	checkMap := make(map[string][]bool)

	for height := rng.Lower; height <= rng.Upper; height++ {
		block, found := h.TestData.ConnectBlocks.Blocks[height]
		if !found {
			continue
		}

		for _, tx := range block.TxDetails {
			for _, vin := range tx.Vin {
				for _, a := range vin.Addresses {
					addr2txs[a] = append(addr2txs[a], txInfo{tx.Txid, ^int32(vin.Vout)})
					checkMap[a] = append(checkMap[a], false)
				}
			}
			for _, vout := range tx.Vout {
				for _, a := range vout.ScriptPubKey.Addresses {
					addr2txs[a] = append(addr2txs[a], txInfo{tx.Txid, int32(vout.N)})
					checkMap[a] = append(checkMap[a], false)
				}
			}
		}
	}

	for addr, txs := range addr2txs {
		err := d.GetTransactions(addr, rng.Lower, rng.Upper, func(txid string, height uint32, indexes []int32) error {
			for i, tx := range txs {
				for _, index := range indexes {
					if txid == tx.txid && index == tx.index {
						checkMap[addr][i] = true
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	for addr, txs := range addr2txs {
		for i, tx := range txs {
			if !checkMap[addr][i] {
				t.Errorf("%s: transaction not found %+v", addr, tx)
			}
		}
	}
}

func verifyAddresses(t *testing.T, d *db.RocksDB, h *TestHandler, rng Range) {
	parser := h.Chain.GetChainParser()

	for height := rng.Lower; height <= rng.Upper; height++ {
		block, found := h.TestData.ConnectBlocks.Blocks[height]
		if !found {
			continue
		}

		for _, tx := range block.TxDetails {
			ta, err := d.GetTxAddresses(tx.Txid)
			if err != nil {
				t.Fatal(err)
			}
			if ta == nil {
				t.Errorf("Tx %s: not found in TxAddresses", tx.Txid)
				continue
			}

			txInfo := getTxInfo(tx)
			taInfo, err := getTaInfo(parser, ta)
			if err != nil {
				t.Fatal(err)
			}

			if ta.Height != height {
				t.Errorf("Tx %s: block height mismatch: %d != %d", tx.Txid, ta.Height, height)
				continue
			}

			if len(txInfo.inputs) > 0 && !reflect.DeepEqual(taInfo.inputs, txInfo.inputs) {
				t.Errorf("Tx %s: inputs mismatch: got %q, want %q", tx.Txid, taInfo.inputs, txInfo.inputs)
			}

			if !reflect.DeepEqual(taInfo.outputs, txInfo.outputs) {
				t.Errorf("Tx %s: outputs mismatch: got %q, want %q", tx.Txid, taInfo.outputs, txInfo.outputs)
			}

			if taInfo.valOutSat.Cmp(&txInfo.valOutSat) != 0 {
				t.Errorf("Tx %s: total output amount mismatch: got %s, want %s",
					tx.Txid, taInfo.valOutSat.String(), txInfo.valOutSat.String())
			}

			if len(txInfo.inputs) > 0 {
				treshold := "0.0001"
				fee := new(big.Int).Sub(&taInfo.valInSat, &taInfo.valOutSat)
				if strings.Compare(parser.AmountToDecimalString(fee), treshold) > 0 {
					t.Errorf("Tx %s: suspicious amounts: input ∑ [%s] - output ∑ [%s] > %s",
						tx.Txid, taInfo.valInSat.String(), taInfo.valOutSat.String(), treshold)
				}
			}
		}
	}
}

type txInfo struct {
	inputs    []string
	outputs   []string
	valInSat  big.Int
	valOutSat big.Int
}

func getTxInfo(tx *bchain.Tx) *txInfo {
	info := &txInfo{inputs: []string{}, outputs: []string{}}

	for _, vin := range tx.Vin {
		for _, a := range vin.Addresses {
			info.inputs = append(info.inputs, a)
		}
	}
	for _, vout := range tx.Vout {
		for _, a := range vout.ScriptPubKey.Addresses {
			info.outputs = append(info.outputs, a)
		}
		info.valOutSat.Add(&info.valOutSat, &vout.ValueSat)
	}

	return info
}

func getTaInfo(parser bchain.BlockChainParser, ta *db.TxAddresses) (*txInfo, error) {
	info := &txInfo{inputs: []string{}, outputs: []string{}}

	for i := range ta.Inputs {
		info.valInSat.Add(&info.valInSat, &ta.Inputs[i].ValueSat)
		addrs, s, err := ta.Inputs[i].Addresses(parser)
		if err == nil && s {
			for _, a := range addrs {
				info.inputs = append(info.inputs, a)
			}
		}
	}

	for i := range ta.Outputs {
		info.valOutSat.Add(&info.valOutSat, &ta.Outputs[i].ValueSat)
		addrs, s, err := ta.Outputs[i].Addresses(parser)
		if err == nil && s {
			for _, a := range addrs {
				info.outputs = append(info.outputs, a)
			}
		}
	}

	return info, nil
}
