//go:build integration

package rpc

import (
	"encoding/hex"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	mapset "github.com/deckarep/golang-set"
	"github.com/ethereum/go-ethereum/common"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
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
	"EthCallBatch":             testEthCallBatch,
	"EthCallErc4626":           testEthCallErc4626,
}

type TestHandler struct {
	Coin     string
	Chain    bchain.BlockChain
	Mempool  bchain.Mempool
	TestData *TestData
}

type EthCallBatchData struct {
	Address         string   `json:"address"`
	Contracts       []string `json:"contracts"`
	BatchSize       int      `json:"batchSize,omitempty"`
	SkipUnavailable bool     `json:"skipUnavailable,omitempty"`
}

type EthCallErc4626Fixture struct {
	Name          string `json:"name,omitempty"`
	Contract      string `json:"contract"`
	ExpectedAsset string `json:"expectedAsset,omitempty"`
}

type EthCallErc4626Data struct {
	Contracts []EthCallErc4626Fixture `json:"contracts"`
}

type TestData struct {
	BlockHeight    uint32                `json:"blockHeight"`
	BlockHash      string                `json:"blockHash"`
	BlockTime      int64                 `json:"blockTime"`
	BlockSize      int                   `json:"blockSize"`
	BlockTxs       []string              `json:"blockTxs"`
	TxDetails      map[string]*bchain.Tx `json:"txDetails"`
	EthCallBatch   *EthCallBatchData     `json:"ethCallBatch,omitempty"`
	EthCallErc4626 *EthCallErc4626Data   `json:"ethCallErc4626,omitempty"`
	// Parsed from txDetails[*].coinSpecificData in fixture JSON.
	TxCoinSpecificData map[string]json.RawMessage `json:"-"`
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
		Coin:     coin,
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
	v.TxCoinSpecificData, err = extractFixtureCoinSpecificData(b)
	if err != nil {
		return nil, err
	}
	for _, tx := range v.TxDetails {
		// convert amounts in test json to bit.Int and clear the temporary JsonValue
		for i := range tx.Vout {
			vout := &tx.Vout[i]
			if shouldConvertFixtureValue(vout.JsonValue.String()) {
				vout.ValueSat, err = parser.AmountToBigInt(vout.JsonValue)
				if err != nil {
					return nil, err
				}
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

func extractFixtureCoinSpecificData(rawFixture []byte) (map[string]json.RawMessage, error) {
	var raw struct {
		TxDetails map[string]json.RawMessage `json:"txDetails"`
	}
	if err := json.Unmarshal(rawFixture, &raw); err != nil {
		return nil, err
	}
	if len(raw.TxDetails) == 0 {
		return nil, nil
	}

	out := make(map[string]json.RawMessage)
	for txid, rawTx := range raw.TxDetails {
		var tx struct {
			CoinSpecificData json.RawMessage `json:"coinSpecificData"`
		}
		if err := json.Unmarshal(rawTx, &tx); err != nil {
			return nil, fmt.Errorf("tx %s: decode fixture tx for coinSpecificData: %w", txid, err)
		}
		if isJSONEmptyOrNull(tx.CoinSpecificData) {
			continue
		}
		out[txid] = append(json.RawMessage(nil), tx.CoinSpecificData...)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func setTxAddresses(parser bchain.BlockChainParser, tx *bchain.Tx) error {
	chainType := parser.GetChainType()
	for i := range tx.Vout {
		ad, err := parser.GetAddrDescFromVout(&tx.Vout[i])
		if err != nil {
			// Ethereum-like chains (including Tron) can legitimately have no "to"
			// address (e.g. contract creation), keep fixture value as-is.
			if chainType == bchain.ChainEthereumType && err == bchain.ErrAddressMissing {
				continue
			}
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
		if wantCoinSpecificData, ok := h.TestData.TxCoinSpecificData[txid]; ok {
			if err := assertCoinSpecificDataContains(got.CoinSpecificData, wantCoinSpecificData); err != nil {
				t.Errorf("GetTransaction() coinSpecificData mismatch for tx %s: %v", txid, err)
				continue
			}
		}
		got.CoinSpecificData = nil
		want.CoinSpecificData = nil

		normalizeAddresses(want, h.Chain.GetChainParser())
		normalizeAddresses(got, h.Chain.GetChainParser())
		normalizeZeroAmounts(want)
		normalizeZeroAmounts(got)

		if !reflect.DeepEqual(got, want) {
			t.Errorf("GetTransaction() got %+#v, want %+#v", got, want)
		}
	}
}

func assertCoinSpecificDataContains(got interface{}, wantRaw json.RawMessage) error {
	if got == nil {
		return errors.New("got coinSpecificData is nil")
	}
	gotRaw, err := json.Marshal(got)
	if err != nil {
		return fmt.Errorf("marshal got coinSpecificData: %w", err)
	}

	var gotJSON interface{}
	if err := json.Unmarshal(gotRaw, &gotJSON); err != nil {
		return fmt.Errorf("decode got coinSpecificData JSON: %w", err)
	}
	var wantJSON interface{}
	if err := json.Unmarshal(wantRaw, &wantJSON); err != nil {
		return fmt.Errorf("decode fixture coinSpecificData JSON: %w", err)
	}
	if !jsonContains(gotJSON, wantJSON) {
		return fmt.Errorf("got %s does not contain expected %s", gotRaw, wantRaw)
	}
	return nil
}

func jsonContains(got, want interface{}) bool {
	switch w := want.(type) {
	case map[string]interface{}:
		gm, ok := got.(map[string]interface{})
		if !ok {
			return false
		}
		for k, wv := range w {
			gv, exists := gm[k]
			if !exists || !jsonContains(gv, wv) {
				return false
			}
		}
		return true
	case []interface{}:
		ga, ok := got.([]interface{})
		if !ok || len(ga) != len(w) {
			return false
		}
		for i := range w {
			if !jsonContains(ga[i], w[i]) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(got, want)
	}
}

func isJSONEmptyOrNull(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null"))
}

func shouldConvertFixtureValue(v string) bool {
	s := strings.TrimSpace(v)
	return s != "" && !strings.EqualFold(s, "null")
}

func testGetTransactionForMempool(t *testing.T, h *TestHandler) {
	for txid, want := range h.TestData.TxDetails {
		// reset fields that are not parsed by BlockChainParser
		want.Confirmations, want.Blocktime, want.Time, want.CoinSpecificData = 0, 0, 0, nil
		// Mempool endpoints may or may not include segwit witness; keep comparisons backend-agnostic.
		stripWitness(want)

		got, err := h.Chain.GetTransactionForMempool(txid)
		if err != nil {
			t.Fatal(err)
		}

		normalizeAddresses(want, h.Chain.GetChainParser())
		normalizeAddresses(got, h.Chain.GetChainParser())
		normalizeZeroAmounts(want)
		normalizeZeroAmounts(got)

		// transactions parsed from JSON may contain additional data
		got.Confirmations, got.Blocktime, got.Time, got.CoinSpecificData = 0, 0, 0, nil
		stripWitness(got)
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

func normalizeZeroAmounts(tx *bchain.Tx) {
	for i := range tx.Vout {
		if tx.Vout[i].ValueSat.Sign() == 0 {
			tx.Vout[i].ValueSat = big.Int{}
		}
	}
}

func stripWitness(tx *bchain.Tx) {
	for i := range tx.Vin {
		tx.Vin[i].Witness = nil
	}
}

func testMempoolSync(t *testing.T, h *TestHandler) {
	for i := 0; i < 3; i++ {
		txs := getMempool(t, h)
		validateMempoolBatchFetch(t, h, txs)

		n, err := h.Mempool.Resync()
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			// no transactions to test
			continue
		}

		beforeIntersect := len(txs)
		txs = intersect(txs, getMempool(t, h))
		if len(txs) == 0 {
			// no transactions to test
			continue
		}
		if beforeIntersect >= 20 {
			ratio := float64(len(txs)) / float64(beforeIntersect)
			if ratio < 0.2 {
				t.Fatalf("mempool intersect too small: after=%d before=%d ratio=%.2f", len(txs), beforeIntersect, ratio)
			}
		}
		const mempoolSyncStride = 5
		txs = sampleEveryNth(txs, mempoolSyncStride)

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

		warmStart := time.Now()
		warmCount, warmErr := h.Mempool.Resync()
		warmDuration := time.Since(warmStart)
		if warmErr != nil {
			t.Logf("Warm resync failed: %v", warmErr)
		} else {
			avgPerTx := time.Duration(0)
			if warmCount > 0 {
				avgPerTx = warmDuration / time.Duration(warmCount)
			}
			t.Logf("Warm resync finished size=%d duration=%s avg_per_tx=%s", warmCount, warmDuration, avgPerTx)
		}

		// done
		return
	}
	t.Skip("Skipping test, all attempts to sync mempool failed due to network state changes")
}

func validateMempoolBatchFetch(t *testing.T, h *TestHandler, txs []string) {
	if mempoolResyncBatchSize(t, h.Coin) > 1 {
		// Validate batch fetch support so the mempool sync test exercises the batched path.
		batcher, ok := h.Chain.(bchain.MempoolBatcher)
		if !ok {
			t.Fatalf("mempool_resync_batch_size > 1 but batch fetch is unavailable for %s", h.Coin)
		}
		sample := txs
		if len(sample) > 5 {
			sample = sample[:5]
		}
		if len(sample) > 0 {
			got, err := batcher.GetRawTransactionsForMempoolBatch(sample)
			if err != nil {
				t.Fatalf("batch getrawtransaction failed for %s: %v", h.Coin, err)
			}
			if len(got) == 0 {
				t.Skip("Skipping test, batch returned no transactions")
			}
			matched := 0
			for _, txid := range sample {
				batchTx := got[txid]
				if batchTx == nil {
					continue
				}
				singleTx, err := h.Chain.GetTransactionForMempool(txid)
				if err != nil {
					if err == bchain.ErrTxNotFound {
						continue
					}
					t.Fatalf("single getrawtransaction failed for %s: %v", h.Coin, err)
				}
				if singleTx == nil {
					t.Fatalf("single getrawtransaction returned nil for %s", h.Coin)
				}
				if batchTx.Txid != txid || singleTx.Txid != txid {
					t.Fatalf("mismatched txid in batch vs single for %s: want %s, batch=%s single=%s", h.Coin, txid, batchTx.Txid, singleTx.Txid)
				}
				matched++
			}
			if matched == 0 {
				t.Skip("Skipping test, no stable mempool transactions to compare")
			}
		}
	}
}

func mempoolResyncBatchSize(t *testing.T, coin string) int {
	t.Helper()

	rawCfg, err := bchain.LoadBlockchainCfgRaw(coin)
	if err != nil {
		t.Fatalf("load blockchain config for %s: %v", coin, err)
	}
	var cfg struct {
		MempoolResyncBatchSize int `json:"mempool_resync_batch_size"`
	}
	if err := json.Unmarshal(rawCfg, &cfg); err != nil {
		t.Fatalf("unmarshal blockchain config for %s: %v", coin, err)
	}
	return cfg.MempoolResyncBatchSize
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
			if err == bchain.ErrBlockNotFound {
				time.Sleep(time.Millisecond * 100)
				continue
			}
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

	// BlockHeader.Size is optional across backends. Some implementations do not
	// include it in getblockheader and leave the decoded value at zero.
	switch {
	case want.Size == 0:
		want.Size = got.Size
	case got.Size == 0:
		t.Logf("Skipping block header size assertion for %s: backend returned size=0 for %s", h.Coin, h.TestData.BlockHash)
		want.Size = 0
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("GetBlockHeader() got=%+#v, want=%+#v", got, want)
	}
}

func testEthCallBatch(t *testing.T, h *TestHandler) {
	data := h.TestData.EthCallBatch
	if data == nil {
		t.Fatal("ethCallBatch fixture missing")
	}
	if data.Address == "" {
		t.Fatal("ethCallBatch.address missing")
	}
	if len(data.Contracts) == 0 {
		t.Fatal("ethCallBatch.contracts missing")
	}

	cfg := bchain.LoadBlockchainCfg(t, h.Coin)
	contracts := make([]common.Address, 0, len(data.Contracts))
	for _, contract := range data.Contracts {
		if contract == "" {
			t.Fatal("ethCallBatch contract address missing")
		}
		contracts = append(contracts, common.HexToAddress(contract))
	}

	bchain.RunERC20BatchBalanceTest(t, bchain.ERC20BatchCase{
		Name:            h.Coin,
		RPCURL:          cfg.RpcUrl,
		RPCURLWS:        cfg.RpcUrlWs,
		Addr:            common.HexToAddress(data.Address),
		Contracts:       contracts,
		BatchSize:       data.BatchSize,
		SkipUnavailable: data.SkipUnavailable,
		NewClient:       eth.NewERC20BatchIntegrationClient,
	})
}

func testEthCallErc4626(t *testing.T, h *TestHandler) {
	data := h.TestData.EthCallErc4626
	if data == nil {
		t.Fatal("ethCallErc4626 fixture missing")
	}
	if len(data.Contracts) == 0 {
		t.Fatal("ethCallErc4626.contracts missing")
	}

	for i := range data.Contracts {
		fixture := data.Contracts[i]
		if fixture.Contract == "" {
			t.Fatalf("ethCallErc4626.contracts[%d].contract missing", i)
		}

		assetRaw, err := h.Chain.EthereumTypeRpcCall("0x38d52e0f", fixture.Contract, "")
		if err != nil {
			t.Fatalf("ethCallErc4626 %s asset() call failed: %v", fixture.Contract, err)
		}
		asset, err := decodeEthCallAddress(assetRaw)
		if err != nil {
			t.Fatalf("ethCallErc4626 %s asset() decode failed: %v", fixture.Contract, err)
		}
		if asset == (common.Address{}) {
			t.Fatalf("ethCallErc4626 %s asset() returned zero address", fixture.Contract)
		}
		if fixture.ExpectedAsset != "" && !strings.EqualFold(asset.Hex(), fixture.ExpectedAsset) {
			t.Fatalf("ethCallErc4626 %s asset() mismatch: got %s want %s", fixture.Contract, asset.Hex(), fixture.ExpectedAsset)
		}

		totalAssetsRaw, err := h.Chain.EthereumTypeRpcCall("0x01e1d114", fixture.Contract, "")
		if err != nil {
			t.Fatalf("ethCallErc4626 %s totalAssets() call failed: %v", fixture.Contract, err)
		}
		if _, err := decodeEthCallUint(totalAssetsRaw); err != nil {
			t.Fatalf("ethCallErc4626 %s totalAssets() decode failed: %v", fixture.Contract, err)
		}
	}
}

func decodeEthCallWord(raw string) ([]byte, error) {
	raw = strings.TrimPrefix(raw, "0x")
	if raw == "" {
		return nil, errors.New("empty eth_call result")
	}
	if len(raw)%2 != 0 {
		return nil, errors.New("invalid eth_call hex length")
	}
	buf, err := hex.DecodeString(raw)
	if err != nil {
		return nil, err
	}
	if len(buf) < 32 {
		return nil, errors.New("eth_call result too short")
	}
	return buf[:32], nil
}

func decodeEthCallAddress(raw string) (common.Address, error) {
	word, err := decodeEthCallWord(raw)
	if err != nil {
		return common.Address{}, err
	}
	return common.BytesToAddress(word[12:]), nil
}

func decodeEthCallUint(raw string) (*big.Int, error) {
	word, err := decodeEthCallWord(raw)
	if err != nil {
		return nil, err
	}
	return new(big.Int).SetBytes(word), nil
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

func sampleEveryNth(txs []string, stride int) []string {
	if stride <= 1 || len(txs) <= stride {
		return txs
	}
	sampled := make([]string, 0, (len(txs)+stride-1)/stride)
	for idx := 0; idx < len(txs); idx += stride {
		sampled = append(sampled, txs[idx])
	}
	return sampled
}

func containsTx(o []bchain.Outpoint, tx string) bool {
	for i := range o {
		if o[i].Txid == tx {
			return true
		}
	}
	return false
}
