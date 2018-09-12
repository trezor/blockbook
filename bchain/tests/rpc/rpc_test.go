// +build integration

package rpc

import (
	"blockbook/bchain"
	"blockbook/bchain/coins"
	"blockbook/build/tools"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/deckarep/golang-set"
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
	Client    bchain.BlockChain
	TestData  *TestData
	connected bool
}

var notConnectedError = errors.New("Not connected to backend server")

func TestRPCIntegration(t *testing.T) {
	src := os.Getenv("BLOCKBOOK_SRC")
	if src == "" {
		t.Fatalf("Missing environment variable BLOCKBOOK_SRC")
	}

	configsDir := filepath.Join(src, "configs")
	templateDir := filepath.Join(src, "build/templates")

	noTests := 0
	skippedTests := make([]string, 0, 10)

	err := filepath.Walk(filepath.Join(configsDir, "coins"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || info.Name()[0] == '.' {
			return nil
		}

		n := strings.TrimSuffix(info.Name(), ".json")
		c, err := build.LoadConfig(configsDir, n)
		if err != nil {
			t.Errorf("%s: cannot load configuration: %s", n, err)
			return nil
		}
		if len(c.IntegrationTests["rpc"]) == 0 {
			return nil
		}

		cfg, err := makeBlockChainConfig(c, templateDir)
		if err != nil {
			t.Errorf("%s: cannot make blockchain config: %s", n, err)
			return nil
		}

		t.Run(c.Coin.Alias, func(t *testing.T) {
			noTests += 1
			err := runTests(t, c.Coin.Name, c.Coin.Alias, cfg, c.IntegrationTests["rpc"])
			if err != nil {
				if err == notConnectedError {
					skippedTests = append(skippedTests, c.Coin.Alias)
					t.Skip(err)
				}
				t.Fatal(err)
			}
		})

		return nil
	})

	if err != nil {
		t.Fatal(err)
	}

	if len(skippedTests) > 0 {
		t.Errorf("Too many skipped tests due to connection issues: %q", skippedTests)
	}
}

func makeBlockChainConfig(c *build.Config, templateDir string) (json.RawMessage, error) {
	outputDir, err := ioutil.TempDir("", "rpc_test")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(outputDir)

	err = build.GeneratePackageDefinitions(c, templateDir, outputDir)
	if err != nil {
		return nil, err
	}

	b, err := ioutil.ReadFile(filepath.Join(outputDir, "blockbook", "blockchaincfg.json"))
	if err != nil {
		return nil, err
	}

	var v json.RawMessage
	err = json.Unmarshal(b, &v)
	if err != nil {
		return nil, err
	}

	return v, nil
}

func runTests(t *testing.T, coinName, coinAlias string, cfg json.RawMessage, tests []string) error {
	cli, err := initBlockChain(coinName, cfg)
	if err != nil {
		if err == notConnectedError {
			return err
		}
		t.Fatal(err)
	}
	td, err := LoadTestData(coinAlias, cli.GetChainParser())
	if err != nil {
		t.Fatalf("Test data loading failed: %s", err)
	}

	if td.TxDetails != nil {
		parser := cli.GetChainParser()

		for _, tx := range td.TxDetails {
			err := setTxAddresses(parser, tx)
			if err != nil {
				t.Fatalf("Test data loading failed: %s", err)
			}
		}
	}

	h := TestHandler{Client: cli, TestData: td}

	for _, test := range tests {
		if f, found := testMap[test]; found {
			t.Run(test, func(t *testing.T) { f(t, &h) })
		} else {
			t.Errorf("%s: test not found", test)
			continue
		}
	}

	return nil
}

func initBlockChain(coinName string, cfg json.RawMessage) (bchain.BlockChain, error) {
	factory, found := coins.BlockChainFactories[coinName]
	if !found {
		return nil, fmt.Errorf("Factory function not found")
	}

	cli, err := factory(cfg, func(_ bchain.NotificationType) {})
	if err != nil {
		if isNetError(err) {
			return nil, notConnectedError
		}
		return nil, fmt.Errorf("Factory function failed: %s", err)
	}

	err = cli.Initialize()
	if err != nil {
		if isNetError(err) {
			return nil, notConnectedError
		}
		return nil, fmt.Errorf("BlockChain initialization failed: %s", err)
	}

	return cli, nil
}

func isNetError(err error) bool {
	if _, ok := err.(net.Error); ok {
		return true
	}
	return false
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

func testGetBlockHash(t *testing.T, h *TestHandler) {
	hash, err := h.Client.GetBlockHash(h.TestData.BlockHeight)
	if err != nil {
		t.Error(err)
		return
	}

	if hash != h.TestData.BlockHash {
		t.Errorf("GetBlockHash() got %q, want %q", hash, h.TestData.BlockHash)
	}
}
func testGetBlock(t *testing.T, h *TestHandler) {
	blk, err := h.Client.GetBlock(h.TestData.BlockHash, 0)
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
		got, err := h.Client.GetTransaction(txid)
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

		if !reflect.DeepEqual(got, want) {
			t.Errorf("GetTransaction() got %+v, want %+v", got, want)
		}
	}
}
func testGetTransactionForMempool(t *testing.T, h *TestHandler) {
	for txid, want := range h.TestData.TxDetails {
		// reset fields that are not parsed by BlockChainParser
		want.Confirmations, want.Blocktime, want.Time = 0, 0, 0

		got, err := h.Client.GetTransactionForMempool(txid)
		if err != nil {
			t.Fatal(err)
		}
		// transactions parsed from JSON may contain additional data
		got.Confirmations, got.Blocktime, got.Time = 0, 0, 0
		if !reflect.DeepEqual(got, want) {
			t.Errorf("GetTransactionForMempool() got %+v, want %+v", got, want)
		}
	}
}
func testMempoolSync(t *testing.T, h *TestHandler) {
	for i := 0; i < 3; i++ {
		txs := getMempool(t, h)

		n, err := h.Client.ResyncMempool(nil)
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

		txid2addrs := getMempoolAddresses(t, h, txs)
		if len(txid2addrs) == 0 {
			t.Skip("Skipping test, no addresses in mempool")
		}

		for txid, addrs := range txid2addrs {
			for _, a := range addrs {
				got, err := h.Client.GetMempoolTransactions(a)
				if err != nil {
					t.Fatal(err)
				}
				if !containsString(got, txid) {
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
		fee, err := h.Client.EstimateSmartFee(blocks, true)
		if err != nil {
			t.Error(err)
		}
		if fee.Sign() == -1 {
			sf := h.Client.GetChainParser().AmountToDecimalString(&fee)
			if sf != "-1" {
				t.Errorf("EstimateSmartFee() returned unexpected fee rate: %v", sf)
			}
		}
	}
}
func testEstimateFee(t *testing.T, h *TestHandler) {
	for _, blocks := range []int{1, 2, 3, 5, 10} {
		fee, err := h.Client.EstimateFee(blocks)
		if err != nil {
			t.Error(err)
		}
		if fee.Sign() == -1 {
			sf := h.Client.GetChainParser().AmountToDecimalString(&fee)
			if sf != "-1" {
				t.Errorf("EstimateFee() returned unexpected fee rate: %v", sf)
			}
		}
	}
}
func testGetBestBlockHash(t *testing.T, h *TestHandler) {
	for i := 0; i < 3; i++ {
		hash, err := h.Client.GetBestBlockHash()
		if err != nil {
			t.Fatal(err)
		}

		height, err := h.Client.GetBestBlockHeight()
		if err != nil {
			t.Fatal(err)
		}
		hh, err := h.Client.GetBlockHash(height)
		if err != nil {
			t.Fatal(err)
		}
		if hash != hh {
			time.Sleep(time.Millisecond * 100)
			continue
		}

		// we expect no next block
		_, err = h.Client.GetBlock("", height+1)
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
		height, err := h.Client.GetBestBlockHeight()
		if err != nil {
			t.Fatal(err)
		}

		// we expect no next block
		_, err = h.Client.GetBlock("", height+1)
		if err != nil {
			if err != bchain.ErrBlockNotFound {
				t.Error(err)
			}
			return
		}
	}
	t.Error("GetBestBlockHeigh() didn't get the the best heigh")
}
func testGetBlockHeader(t *testing.T, h *TestHandler) {
	want := &bchain.BlockHeader{
		Hash:   h.TestData.BlockHash,
		Height: h.TestData.BlockHeight,
		Time:   h.TestData.BlockTime,
	}

	got, err := h.Client.GetBlockHeader(h.TestData.BlockHash)
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
		t.Errorf("GetBlockHeader() got=%+v, want=%+v", got, want)
	}
}

func getMempool(t *testing.T, h *TestHandler) []string {
	txs, err := h.Client.GetMempool()
	if err != nil {
		t.Fatal(err)
	}
	if len(txs) == 0 {
		t.Skip("Skipping test, mempool is empty")
	}

	return txs
}

func getMempoolAddresses(t *testing.T, h *TestHandler, txs []string) map[string][]string {
	txid2addrs := map[string][]string{}
	for i := 0; i < len(txs); i++ {
		tx, err := h.Client.GetTransactionForMempool(txs[i])
		if err != nil {
			t.Fatal(err)
		}
		addrs := []string{}
		for _, vin := range tx.Vin {
			for _, a := range vin.Addresses {
				if isSearchableAddr(a) {
					addrs = append(addrs, a)
				}
			}
		}
		for _, vout := range tx.Vout {
			for _, a := range vout.ScriptPubKey.Addresses {
				if isSearchableAddr(a) {
					addrs = append(addrs, a)
				}
			}
		}
		if len(addrs) > 0 {
			txid2addrs[tx.Txid] = addrs
		}
	}
	return txid2addrs
}

func isSearchableAddr(addr string) bool {
	return len(addr) > 3 && addr[:3] != "OP_"
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

func containsString(slice []string, s string) bool {
	for i := 0; i < len(slice); i++ {
		if slice[i] == s {
			return true
		}
	}
	return false
}
