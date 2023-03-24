//go:build integration

package rpc

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	mapset "github.com/deckarep/golang-set"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
)

var testMap = map[string]func(t *testing.T, th *TestHandler){
	"GetBlockHash":             testGetBlockHash,
	"GetBlock":                 testGetBlock,
	"GetTransaction":           testGetTransaction,
	"GetTransactionForMempool": testGetTransactionForMempool,
	"MempoolSync":              testMempoolSync,
	"EstimateSmartFee":         testEstimateSmartFee,
	"EstimateFee":              testEstimateFee,
	"GetBestBlockHash":         testGetBestBlockHash,
	"GetBestBlockHeight":       testGetBestBlockHeight,
	"GetBlockHeader":           testGetBlockHeader,
}

type TestHandler struct {
	Chain    bchain.BlockChain
	Mempool  bchain.Mempool
	TestData *TestData
}

type TestData struct {
	BlockHeight uint32                `json:"blockHeight"`
	BlockHash   string                `json:"blockHash"`
	BlockTime   int64                 `json:"blockTime"`
	BlockSize   int                   `json:"blockSize"`
	BlockTxs    []string              `json:"blockTxs"`
	TxDetails   map[string]*bchain.Tx `json:"txDetails"`
}

func IntegrationTest(t *testing.T, coin string, chain bchain.BlockChain, mempool bchain.Mempool, testConfig json.RawMessage) {
	tests, err := getTests(testConfig)
	if err != nil {
		t.Fatalf("Failed loading of test list: %s", err)
	}

	parser := chain.GetChainParser()
	td, err := loadTestData(coin, parser)
	if err != nil {
		t.Fatalf("Failed loading of test data: %s", err)
	}

	h := TestHandler{
		Chain:    chain,
		Mempool:  mempool,
		TestData: td,
	}

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
	path := filepath.Join("rpc/testdata", coin+".json")
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v TestData
	err = json.Unmarshal(b, &v)
	if err != nil {
		return nil, err
	}
	for _, tx := range v.TxDetails {
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

	return &v, nil
}

func setTxAddresses(parser bchain.BlockChainParser, tx *bchain.Tx) error {
	for i := range tx.Vout {
		ad, err := parser.GetAddrDescFromVout(&tx.Vout[i])
		if err != nil {
			return err
		}
		addrs := []string{}
		a, s, err := parser.GetAddressesFromAddrDesc(ad)
		if err == nil && s {
			addrs = append(addrs, a...)
		}
		tx.Vout[i].ScriptPubKey.Addresses = addrs
	}
	return nil
}

func testGetBlockHash(t *testing.T, h *TestHandler) {
	hash, err := h.Chain.GetBlockHash(h.TestData.BlockHeight)
	if err != nil {
		t.Error(err)
		return
	}

	if hash != h.TestData.BlockHash {
		t.Errorf("GetBlockHash() got %q, want %q", hash, h.TestData.BlockHash)
	}
}

func testGetBlock(t *testing.T, h *TestHandler) {
	blk, err := h.Chain.GetBlock(h.TestData.BlockHash, 0)
	if err != nil {
		t.Error(err)
		return
	}

	if len(blk.Txs) != len(h.TestData.BlockTxs) {
		t.Errorf("GetBlock() number of transactions: got %d, want %d", len(blk.Txs), len(h.TestData.BlockTxs))
	}

	for ti, tx := range blk.Txs {
		if tx.Txid != h.TestData.BlockTxs[ti] {
			t.Errorf("GetBlock() transaction %d: got %s, want %s", ti, tx.Txid, h.TestData.BlockTxs[ti])
		}
	}
}

func testGetTransaction(t *testing.T, h *TestHandler) {
	for txid, want := range h.TestData.TxDetails {
		got, err := h.Chain.GetTransaction(txid)
		if err != nil {
			t.Error(err)
			return
		}
		// Confirmations is variable field, we just check if is set and reset it
		if got.Confirmations <= 0 {
			t.Errorf("GetTransaction() got struct with invalid Confirmations field")
			continue
		}
		got.Confirmations = 0
		// CoinSpecificData are not specified in the fixtures
		got.CoinSpecificData = nil

		normalizeAddresses(want, h.Chain.GetChainParser())
		normalizeAddresses(got, h.Chain.GetChainParser())

		if !reflect.DeepEqual(got, want) {
			t.Errorf("GetTransaction() got %+#v, want %+#v", got, want)
		}
	}
}

func testGetTransactionForMempool(t *testing.T, h *TestHandler) {
	for txid, want := range h.TestData.TxDetails {
		// reset fields that are not parsed by BlockChainParser
		want.Confirmations, want.Blocktime, want.Time, want.CoinSpecificData = 0, 0, 0, nil

		got, err := h.Chain.GetTransactionForMempool(txid)
		if err != nil {
			t.Fatal(err)
		}

		normalizeAddresses(want, h.Chain.GetChainParser())
		normalizeAddresses(got, h.Chain.GetChainParser())

		// transactions parsed from JSON may contain additional data
		got.Confirmations, got.Blocktime, got.Time, got.CoinSpecificData = 0, 0, 0, nil
		if !reflect.DeepEqual(got, want) {
			t.Errorf("GetTransactionForMempool() got %+#v, want %+#v", got, want)
		}
	}
}

// empty slice can be either []slice{} or nil; reflect.DeepEqual treats them as different value
// remove checksums from ethereum addresses
func normalizeAddresses(tx *bchain.Tx, parser bchain.BlockChainParser) {
	for i := range tx.Vin {
		if len(tx.Vin[i].Addresses) == 0 {
			tx.Vin[i].Addresses = nil
		} else {
			if parser.GetChainType() == bchain.ChainEthereumType {
				for j := range tx.Vin[i].Addresses {
					tx.Vin[i].Addresses[j] = strings.ToLower(tx.Vin[i].Addresses[j])
				}
			}
		}
	}
	for i := range tx.Vout {
		if len(tx.Vout[i].ScriptPubKey.Addresses) == 0 {
			tx.Vout[i].ScriptPubKey.Addresses = nil
		} else {
			if parser.GetChainType() == bchain.ChainEthereumType {
				for j := range tx.Vout[i].ScriptPubKey.Addresses {
					tx.Vout[i].ScriptPubKey.Addresses[j] = strings.ToLower(tx.Vout[i].ScriptPubKey.Addresses[j])
				}
			}
		}
	}
}

func testMempoolSync(t *testing.T, h *TestHandler) {
	for i := 0; i < 3; i++ {
		txs := getMempool(t, h)

		n, err := h.Mempool.Resync()
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			// no transactions to test
			continue
		}

		txs = intersect(txs, getMempool(t, h))
		if len(txs) == 0 {
			// no transactions to test
			continue
		}

		txid2addrs := getTxid2addrs(t, h, txs)
		if len(txid2addrs) == 0 {
			t.Skip("Skipping test, no addresses in mempool")
		}

		for txid, addrs := range txid2addrs {
			for _, a := range addrs {
				got, err := h.Mempool.GetTransactions(a)
				if err != nil {
					t.Fatalf("address %q: %s", a, err)
				}
				if !containsTx(got, txid) {
					t.Errorf("ResyncMempool() - for address %s, transaction %s wasn't found in mempool", a, txid)
					return
				}
			}
		}

		// done
		return
	}
	t.Skip("Skipping test, all attempts to sync mempool failed due to network state changes")
}

func testEstimateSmartFee(t *testing.T, h *TestHandler) {
	for _, blocks := range []int{1, 2, 3, 5, 10} {
		fee, err := h.Chain.EstimateSmartFee(blocks, true)
		if err != nil {
			t.Error(err)
		}
		if fee.Sign() == -1 {
			sf := h.Chain.GetChainParser().AmountToDecimalString(&fee)
			if sf != "-1" {
				t.Errorf("EstimateSmartFee() returned unexpected fee rate: %v", sf)
			}
		}
	}
}

func testEstimateFee(t *testing.T, h *TestHandler) {
	for _, blocks := range []int{1, 2, 3, 5, 10} {
		fee, err := h.Chain.EstimateFee(blocks)
		if err != nil {
			t.Error(err)
		}
		if fee.Sign() == -1 {
			sf := h.Chain.GetChainParser().AmountToDecimalString(&fee)
			if sf != "-1" {
				t.Errorf("EstimateFee() returned unexpected fee rate: %v", sf)
			}
		}
	}
}

func testGetBestBlockHash(t *testing.T, h *TestHandler) {
	for i := 0; i < 3; i++ {
		hash, err := h.Chain.GetBestBlockHash()
		if err != nil {
			t.Fatal(err)
		}

		height, err := h.Chain.GetBestBlockHeight()
		if err != nil {
			t.Fatal(err)
		}
		hh, err := h.Chain.GetBlockHash(height)
		if err != nil {
			t.Fatal(err)
		}
		if hash != hh {
			time.Sleep(time.Millisecond * 100)
			continue
		}

		// we expect no next block
		_, err = h.Chain.GetBlock("", height+1)
		if err != nil {
			if err != bchain.ErrBlockNotFound {
				t.Error(err)
			}
			return
		}
	}
	t.Error("GetBestBlockHash() didn't get the best hash")
}

func testGetBestBlockHeight(t *testing.T, h *TestHandler) {
	for i := 0; i < 3; i++ {
		height, err := h.Chain.GetBestBlockHeight()
		if err != nil {
			t.Fatal(err)
		}

		// we expect no next block
		_, err = h.Chain.GetBlock("", height+1)
		if err != nil {
			if err != bchain.ErrBlockNotFound {
				t.Error(err)
			}
			return
		}
	}
	t.Error("GetBestBlockHeight() didn't get the best height")
}

func testGetBlockHeader(t *testing.T, h *TestHandler) {
	want := &bchain.BlockHeader{
		Hash:   h.TestData.BlockHash,
		Height: h.TestData.BlockHeight,
		Time:   h.TestData.BlockTime,
		Size:   h.TestData.BlockSize,
	}

	got, err := h.Chain.GetBlockHeader(h.TestData.BlockHash)
	if err != nil {
		t.Fatal(err)
	}

	// Confirmations is variable field, we just check if is set and reset it
	if got.Confirmations <= 0 {
		t.Fatalf("GetBlockHeader() got struct with invalid Confirmations field")
	}
	got.Confirmations = 0

	got.Prev, got.Next = "", ""

	if !reflect.DeepEqual(got, want) {
		t.Errorf("GetBlockHeader() got=%+#v, want=%+#v", got, want)
	}
}

func getMempool(t *testing.T, h *TestHandler) []string {
	txs, err := h.Chain.GetMempoolTransactions()
	if err != nil {
		t.Fatal(err)
	}
	if len(txs) == 0 {
		t.Skip("Skipping test, mempool is empty")
	}

	return txs
}

func getTxid2addrs(t *testing.T, h *TestHandler, txs []string) map[string][]string {
	txid2addrs := map[string][]string{}
	for i := range txs {
		tx, err := h.Chain.GetTransactionForMempool(txs[i])
		if err != nil {
			if err == bchain.ErrTxNotFound {
				continue
			}
			t.Fatal(err)
		}
		setTxAddresses(h.Chain.GetChainParser(), tx)
		addrs := []string{}
		for j := range tx.Vout {
			for _, a := range tx.Vout[j].ScriptPubKey.Addresses {
				addrs = append(addrs, a)
			}
		}
		if len(addrs) > 0 {
			txid2addrs[tx.Txid] = addrs
		}
	}
	return txid2addrs
}

func intersect(a, b []string) []string {
	setA := mapset.NewSet()
	for _, v := range a {
		setA.Add(v)
	}
	setB := mapset.NewSet()
	for _, v := range b {
		setB.Add(v)
	}
	inter := setA.Intersect(setB)
	res := make([]string, 0, inter.Cardinality())
	for v := range inter.Iter() {
		res = append(res, v.(string))
	}
	return res
}

func containsTx(o []bchain.Outpoint, tx string) bool {
	for i := range o {
		if o[i].Txid == tx {
			return true
		}
	}
	return false
}
