//go:build integration

package sync

import (
	"fmt"
	"math/big"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/db"
)

func testHandleFork(t *testing.T, h *TestHandler) {
	for _, rng := range h.TestData.HandleFork.SyncRanges {
		withRocksDBAndSyncWorker(t, h, rng.Lower, func(d *db.RocksDB, sw *db.SyncWorker, ch chan os.Signal) {
			fakeBlocks := getFakeBlocks(h, rng)
			chain, err := makeFakeChain(h.Chain, fakeBlocks, rng.Upper)
			if err != nil {
				t.Fatal(err)
			}

			db.SetBlockChain(sw, chain)

			sw.ConnectBlocksParallel(rng.Lower, rng.Upper)

			height, _, err := d.GetBestBlock()
			if err != nil {
				t.Fatal(err)
			}
			if height != rng.Upper {
				t.Fatalf("Upper block height mismatch: %d != %d", height, rng.Upper)
			}

			fakeTxs, err := getTxs(h, d, rng, fakeBlocks)
			if err != nil {
				t.Fatal(err)
			}
			fakeAddr2txs := getAddr2TxsMap(fakeTxs)

			verifyTransactions2(t, d, rng, fakeAddr2txs, true)
			verifyAddresses2(t, d, h.Chain, fakeBlocks)

			chain.returnFakes = false

			upperHash := fakeBlocks[len(fakeBlocks)-1].Hash
			db.HandleFork(sw, rng.Upper, upperHash, func(hash string, height uint32) {
				if hash == upperHash {
					close(ch)
				}
			}, true)

			realBlocks := getRealBlocks(h, rng)
			realTxs, err := getTxs(h, d, rng, realBlocks)
			if err != nil {
				t.Fatal(err)
			}
			realAddr2txs := getAddr2TxsMap(realTxs)

			verifyTransactions2(t, d, rng, fakeAddr2txs, false)
			verifyTransactions2(t, d, rng, realAddr2txs, true)
			verifyAddresses2(t, d, h.Chain, realBlocks)
		})
	}
}

func verifyAddresses2(t *testing.T, d *db.RocksDB, chain bchain.BlockChain, blks []BlockID) {
	parser := chain.GetChainParser()

	for _, b := range blks {
		txs, err := getBlockTxs(chain, b.Hash)
		if err != nil {
			t.Fatal(err)
		}

		for _, tx := range txs {
			ta, err := d.GetTxAddresses(tx.Txid)
			if err != nil {
				t.Fatal(err)
			}
			if ta == nil {
				t.Errorf("Tx %s: not found in TxAddresses", tx.Txid)
				continue
			}

			txInfo := getTxInfo(&tx)
			taInfo, err := getTaInfo(parser, ta)
			if err != nil {
				t.Fatal(err)
			}

			if ta.Height != b.Height {
				t.Errorf("Tx %s: block height mismatch: %d != %d", tx.Txid, ta.Height, b.Height)
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

func verifyTransactions2(t *testing.T, d *db.RocksDB, rng Range, addr2txs map[string][]string, exist bool) {
	noErrs := 0
	for addr, txs := range addr2txs {
		checkMap := make(map[string]bool, len(txs))
		for _, txid := range txs {
			checkMap[txid] = false
		}

		err := d.GetTransactions(addr, rng.Lower, rng.Upper, func(txid string, height uint32, indexes []int32) error {
			for _, index := range indexes {
				if index >= 0 {
					checkMap[txid] = true
					break
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}

		for _, txid := range txs {
			if checkMap[txid] != exist {
				auxverb := "wasn't"
				if !exist {
					auxverb = "was"
				}
				t.Errorf("%s: transaction %s %s found [expected = %t]", addr, txid, auxverb, exist)
				noErrs++
				if noErrs >= 10 {
					t.Fatal("Too many errors")
				}
			}
		}
	}
}

func getFakeBlocks(h *TestHandler, rng Range) []BlockID {
	blks := make([]BlockID, 0, rng.Upper-rng.Lower+1)
	for i := rng.Lower; i <= rng.Upper; i++ {
		if b, found := h.TestData.HandleFork.FakeBlocks[i]; found {
			blks = append(blks, b)
		}
	}
	return blks
}

func getRealBlocks(h *TestHandler, rng Range) []BlockID {
	blks := make([]BlockID, 0, rng.Upper-rng.Lower+1)
	for _, b := range h.TestData.HandleFork.RealBlocks {
		if b.Height >= rng.Lower && b.Height <= rng.Upper {
			blks = append(blks, b)
		}
	}
	return blks
}

func makeFakeChain(chain bchain.BlockChain, blks []BlockID, upper uint32) (*fakeBlockChain, error) {
	if blks[len(blks)-1].Height != upper {
		return nil, fmt.Errorf("Range must end with fake block in order to emulate fork [%d != %d]", blks[len(blks)-1].Height, upper)
	}
	mBlks := make(map[uint32]BlockID, len(blks))
	for i := range blks {
		mBlks[blks[i].Height] = blks[i]
	}
	return &fakeBlockChain{
		BlockChain:  chain,
		returnFakes: true,
		fakeBlocks:  mBlks,
		bestHeight:  upper,
	}, nil
}

func getTxs(h *TestHandler, d *db.RocksDB, rng Range, blks []BlockID) ([]bchain.Tx, error) {
	res := make([]bchain.Tx, 0, (rng.Upper-rng.Lower+1)*2000)

	for _, b := range blks {
		bi, err := d.GetBlockInfo(b.Height)
		if err != nil {
			return nil, err
		}
		if bi.Hash != b.Hash {
			return nil, fmt.Errorf("Block hash mismatch: %s != %s", bi.Hash, b.Hash)
		}

		txs, err := getBlockTxs(h.Chain, b.Hash)
		if err != nil {
			return nil, err
		}
		res = append(res, txs...)
	}

	return res, nil
}

func getBlockTxs(chain bchain.BlockChain, hash string) ([]bchain.Tx, error) {
	b, err := chain.GetBlock(hash, 0)
	if err != nil {
		return nil, fmt.Errorf("GetBlock: %s", err)
	}
	parser := chain.GetChainParser()
	for i := range b.Txs {
		err := setTxAddresses(parser, &b.Txs[i])
		if err != nil {
			return nil, fmt.Errorf("setTxAddresses [%s]: %s", b.Txs[i].Txid, err)
		}
	}
	return b.Txs, nil
}

func getAddr2TxsMap(txs []bchain.Tx) map[string][]string {
	addr2txs := make(map[string][]string)
	for i := range txs {
		for j := range txs[i].Vout {
			for k := range txs[i].Vout[j].ScriptPubKey.Addresses {
				addr := txs[i].Vout[j].ScriptPubKey.Addresses[k]
				txid := txs[i].Txid
				addr2txs[addr] = append(addr2txs[addr], txid)
			}
		}
	}
	return addr2txs
}
