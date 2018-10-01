package sync

import (
	"blockbook/bchain"
	"blockbook/common"
	"blockbook/db"
	"encoding/json"
	"errors"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

var testMap = map[string]func(t *testing.T, th *TestHandler){
	// "ConnectBlocks": nil,
	"ConnectBlocksParallel": testConnectBlocksParallel,
	// "DisconnectBlocks":      nil,
}

type TestHandler struct {
	Coin     string
	Chain    bchain.BlockChain
	TestData *TestData
}

type TestData struct {
	ConnectBlocksParallel struct {
		SyncWorkers int `json:"syncWorkers"`
		SyncChunk   int `json:"syncChunk"`
	} `json:"connectBlocksParallel"`
	Blocks []BlockInfo `json:"blocks"`
}

type BlockInfo struct {
	Height    uint32       `json:"height"`
	Hash      string       `json:"hash"`
	NoTxs     uint32       `json:"noTxs"`
	TxDetails []*bchain.Tx `json:"txDetails"`
}

func IntegrationTest(t *testing.T, coin string, chain bchain.BlockChain, testConfig json.RawMessage) {
	tests, err := getTests(testConfig)
	if err != nil {
		t.Fatalf("Failed loading of test list: %s", err)
	}

	parser := chain.GetChainParser()
	td, err := loadTestData(coin, parser)
	if err != nil {
		t.Fatalf("Failed loading of test data: %s", err)
	}

	h := TestHandler{Coin: coin, Chain: chain, TestData: td}

	for _, test := range tests {
		if f, found := testMap[test]; found {
			t.Run(test, func(t *testing.T) { f(t, &h) })
		} else {
			t.Errorf("%s: test not found", test)
			continue
		}
	}
}

func getTests(cfg json.RawMessage) ([]string, error) {
	var v []string
	err := json.Unmarshal(cfg, &v)
	if err != nil {
		return nil, err
	}
	if len(v) == 0 {
		return nil, errors.New("No tests declared")
	}
	return v, nil
}

func loadTestData(coin string, parser bchain.BlockChainParser) (*TestData, error) {
	path := filepath.Join("sync/testdata", coin+".json")
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v TestData
	err = json.Unmarshal(b, &v)
	if err != nil {
		return nil, err
	}

	for _, b := range v.Blocks {
		for _, tx := range b.TxDetails {
			// convert amounts in test json to bit.Int and clear the temporary JsonValue
			for i := range tx.Vout {
				vout := &tx.Vout[i]
				vout.ValueSat, err = parser.AmountToBigInt(vout.JsonValue)
				if err != nil {
					return nil, err
				}
				vout.JsonValue = ""
			}

			// get addresses parsed
			err := setTxAddresses(parser, tx)
			if err != nil {
				return nil, err
			}
		}
	}

	sort.Slice(v.Blocks, func(i, j int) bool {
		return v.Blocks[i].Height < v.Blocks[j].Height
	})

	return &v, nil
}

func setTxAddresses(parser bchain.BlockChainParser, tx *bchain.Tx) error {
	// pack and unpack transaction in order to get addresses decoded - ugly but works
	var tmp *bchain.Tx
	b, err := parser.PackTx(tx, 0, 0)
	if err == nil {
		tmp, _, err = parser.UnpackTx(b)
		if err == nil {
			for i := 0; i < len(tx.Vout); i++ {
				tx.Vout[i].ScriptPubKey.Addresses = tmp.Vout[i].ScriptPubKey.Addresses
			}
		}
	}
	return err
}

func makeRocksDB(parser bchain.BlockChainParser, m *common.Metrics, is *common.InternalState) (*db.RocksDB, func(), error) {
	p, err := ioutil.TempDir("", "sync_test")
	if err != nil {
		return nil, nil, err
	}

	d, err := db.NewRocksDB(p, 1<<17, 1<<14, parser, m)
	if err != nil {
		return nil, nil, err
	}

	d.SetInternalState(is)

	closer := func() {
		d.Close()
		os.RemoveAll(p)
	}

	return d, closer, nil
}

func withRocksDBAndSyncWorker(t *testing.T, h *TestHandler, fn func(*db.RocksDB, *db.SyncWorker, chan os.Signal)) {
	m, err := common.GetMetrics(h.Coin)
	if err != nil {
		t.Fatal(err)
	}
	is := &common.InternalState{}

	d, closer, err := makeRocksDB(h.Chain.GetChainParser(), m, is)
	if err != nil {
		t.Fatal(err)
	}
	defer closer()

	if len(h.TestData.Blocks) == 0 {
		t.Fatal("No test data")
	}

	ch := make(chan os.Signal)

	sw, err := db.NewSyncWorker(d, h.Chain, 3, 0, int(h.TestData.Blocks[0].Height), false, ch, m, is)
	if err != nil {
		t.Fatal(err)
	}

	fn(d, sw, ch)
}

func testConnectBlocksParallel(t *testing.T, h *TestHandler) {
	withRocksDBAndSyncWorker(t, h, func(d *db.RocksDB, sw *db.SyncWorker, ch chan os.Signal) {
		lowerHeight := h.TestData.Blocks[0].Height
		upperHeight := h.TestData.Blocks[len(h.TestData.Blocks)-1].Height
		upperHash := h.TestData.Blocks[len(h.TestData.Blocks)-1].Hash

		err := sw.ConnectBlocksParallel(lowerHeight, upperHeight)
		if err != nil {
			t.Fatal(err)
		}

		height, hash, err := d.GetBestBlock()
		if err != nil {
			t.Fatal(err)
		}
		if height != upperHeight {
			t.Fatalf("Upper block height mismatch: %d != %d", height, upperHeight)
		}
		if hash != upperHash {
			t.Fatalf("Upper block hash mismatch: %s != %s", hash, upperHash)
		}

		t.Run("verifyBlockInfo", func(t *testing.T) { verifyBlockInfo(t, d, h) })
		t.Run("verifyTransactions", func(t *testing.T) { verifyTransactions(t, d, h) })
		t.Run("verifyAddresses", func(t *testing.T) { verifyAddresses(t, d, h) })
	})
}

func verifyBlockInfo(t *testing.T, d *db.RocksDB, h *TestHandler) {
	for _, block := range h.TestData.Blocks {
		bi, err := d.GetBlockInfo(block.Height)
		if err != nil {
			t.Errorf("GetBlockInfo(%d) error: %s", block.Height, err)
			continue
		}
		if bi == nil {
			t.Errorf("GetBlockInfo(%d) returned nil", block.Height)
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

func verifyTransactions(t *testing.T, d *db.RocksDB, h *TestHandler) {
	type txInfo struct {
		txid     string
		vout     uint32
		isOutput bool
	}
	addr2txs := make(map[string][]txInfo)
	checkMap := make(map[string][]bool)

	for _, block := range h.TestData.Blocks {
		for _, tx := range block.TxDetails {
			// for _, vin := range tx.Vin {
			// 	if vin.Txid != "" {
			// 		ta, err := d.GetTxAddresses(vin.Txid)
			// 		if err != nil {
			// 			t.Fatal(err)
			// 		}
			// 		if ta != nil {
			// 			if len(ta.Outputs) > int(vin.Vout) {
			// 				output := &ta.Outputs[vin.Vout]
			// 				voutAddr, _, err := output.Addresses(h.Chain.GetChainParser())
			// 				if err != nil {
			// 					t.Fatal(err)
			// 				}
			// 				t.Logf("XXX: %q", voutAddr)
			// 			}
			// 		}
			// 	}
			// }
			for _, vin := range tx.Vin {
				for _, a := range vin.Addresses {
					addr2txs[a] = append(addr2txs[a], txInfo{tx.Txid, vin.Vout, false})
					checkMap[a] = append(checkMap[a], false)
				}
			}
			for _, vout := range tx.Vout {
				for _, a := range vout.ScriptPubKey.Addresses {
					addr2txs[a] = append(addr2txs[a], txInfo{tx.Txid, vout.N, true})
					checkMap[a] = append(checkMap[a], false)
				}
			}
		}
	}

	lowerHeight := h.TestData.Blocks[0].Height
	upperHeight := h.TestData.Blocks[len(h.TestData.Blocks)-1].Height
	for addr, txs := range addr2txs {
		err := d.GetTransactions(addr, lowerHeight, upperHeight, func(txid string, vout uint32, isOutput bool) error {
			for i, tx := range txs {
				if txid == tx.txid && vout == tx.vout && isOutput == tx.isOutput {
					checkMap[addr][i] = true
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

func verifyAddresses(t *testing.T, d *db.RocksDB, h *TestHandler) {
	parser := h.Chain.GetChainParser()

	for _, block := range h.TestData.Blocks {
		for _, tx := range block.TxDetails {
			ta, err := d.GetTxAddresses(tx.Txid)
			if err != nil {
				t.Fatal(err)
			}

			txInfo := getTxInfo(tx)
			taInfo, err := getTaInfo(parser, ta)
			if err != nil {
				t.Fatal(err)
			}

			if ta.Height != block.Height {
				t.Errorf("Tx %s: block height mismatch: %d != %d", tx.Txid, ta.Height, block.Height)
				continue
			}

			if len(txInfo.inputs) > 0 && !reflect.DeepEqual(taInfo.inputs, txInfo.inputs) {
				t.Errorf("Tx %s: inputs mismatch: got %q, want %q", tx.Txid, taInfo.inputs, txInfo.inputs)
			}

			if !reflect.DeepEqual(taInfo.outputs, txInfo.outputs) {
				t.Errorf("Tx %s: outputs mismatch: got %q, want %q", tx.Txid, taInfo.outputs, txInfo.outputs)
			}

			taValIn := satToFloat(parser, &taInfo.valInSat)
			taValOut := satToFloat(parser, &taInfo.valOutSat)
			txValOut := satToFloat(parser, &txInfo.valOutSat)

			if taValOut.Cmp(txValOut) != 0 {
				t.Errorf("Tx %s: total output amount mismatch: got %s, want %s",
					tx.Txid, taValOut.String(), txValOut.String())
			}

			treshold := big.NewFloat(0.0001)
			if new(big.Float).Sub(taValIn, taValOut).Cmp(treshold) > 0 {
				t.Errorf("Tx %s: suspicious amounts: input ∑ [%s] - output ∑ [%s] > %s",
					tx.Txid, taValIn.String(), taValOut.String(), treshold)
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
		addrs, _, err := ta.Inputs[i].Addresses(parser)
		if err != nil {
			return nil, err
		}
		info.inputs = append(info.inputs, addrs...)
	}

	for i := range ta.Outputs {
		info.valOutSat.Add(&info.valOutSat, &ta.Outputs[i].ValueSat)
		addrs, _, err := ta.Outputs[i].Addresses(parser)
		if err != nil {
			return nil, err
		}
		info.outputs = append(info.outputs, addrs...)
	}

	return info, nil
}

func satToFloat(parser bchain.BlockChainParser, sat *big.Int) *big.Float {
	f, ok := new(big.Float).SetString(parser.AmountToDecimalString(sat))
	if !ok {
		return big.NewFloat(-1)
	}
	return f
}
