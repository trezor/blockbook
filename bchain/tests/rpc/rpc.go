// +build integration

package rpc

import (
	"blockbook/bchain"
	"encoding/json"
	"math/rand"
	"reflect"
	"testing"
	"time"
)

type TestConfig struct {
	URL  string `json:"url"`
	User string `json:"user"`
	Pass string `json:"pass"`
}

type TestData struct {
	BlockHeight uint32                `json:"blockHeight"`
	BlockHash   string                `json:"blockHash"`
	BlockHex    string                `json:"blockHex"`
	BlockTxs    []string              `json:"blockTxs"`
	TxDetails   map[string]*bchain.Tx `json:"txDetails"`
}

type Test struct {
	Client   bchain.BlockChain
	TestData *TestData
}

type TestChainFactoryFunc func(json.RawMessage) (bchain.BlockChain, error)

func NewTest(coin string, factory TestChainFactoryFunc) (*Test, error) {
	cfg, err := LoadRPCConfig(coin)
	if err != nil {
		return nil, err
	}
	cli, err := factory(cfg)
	if err != nil {
		return nil, err
	}
	td, err := LoadTestData(coin)
	if err != nil {
		return nil, err
	}

	if td.TxDetails != nil {
		parser := cli.GetChainParser()

		for _, tx := range td.TxDetails {
			err := setTxAddresses(parser, tx)
			if err != nil {
				return nil, err
			}
		}
	}

	return &Test{Client: cli, TestData: td}, nil
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
				tx.Vout[i].Address = tmp.Vout[i].Address
			}
		}
	}
	return err
}
func (rt *Test) TestGetBlockHash(t *testing.T) {
	hash, err := rt.Client.GetBlockHash(rt.TestData.BlockHeight)
	if err != nil {
		t.Error(err)
		return
	}

	if hash != rt.TestData.BlockHash {
		t.Errorf("GetBlockHash() got %q, want %q", hash, rt.TestData.BlockHash)
	}
}

func (rt *Test) TestGetBlock(t *testing.T) {
	blk, err := rt.Client.GetBlock(rt.TestData.BlockHash, 0)
	if err != nil {
		t.Error(err)
		return
	}

	if len(blk.Txs) != len(rt.TestData.BlockTxs) {
		t.Errorf("GetBlock() number of transactions: got %d, want %d", len(blk.Txs), len(rt.TestData.BlockTxs))
	}

	for ti, tx := range blk.Txs {
		if tx.Txid != rt.TestData.BlockTxs[ti] {
			t.Errorf("GetBlock() transaction %d: got %s, want %s", ti, tx.Txid, rt.TestData.BlockTxs[ti])
		}
	}

}

func (rt *Test) TestGetTransaction(t *testing.T) {
	for txid, want := range rt.TestData.TxDetails {
		got, err := rt.Client.GetTransaction(txid)
		if err != nil {
			t.Error(err)
			return
		}
		// Confirmations is variable field, we just check if is set and reset it
		if got.Confirmations > 0 {
			got.Confirmations = 0
		} else {
			t.Errorf("GetTransaction() has empty Confirmations field")
			continue
		}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("GetTransaction() got %v, want %v", got, want)
		}
	}
}

func (rt *Test) getMempool(t *testing.T) []string {
	var (
		txs []string
		err error
	)
	// attempts to get transactions for 2 min
	for i := 0; i < 8; i++ {
		txs, err = rt.Client.GetMempool()
		if err != nil {
			t.Fatal(err)
		}
		if len(txs) == 0 {
			time.Sleep(15 * time.Second)
			continue
		}

		// done
		break
	}
	if len(txs) == 0 {
		t.Skipf("Skipping test, all attempts to get mempool failed")
	}

	return txs
}

func (rt *Test) getMempoolTransaction(t *testing.T, txid string) *bchain.Tx {
	tx, err := rt.Client.GetTransactionForMempool(txid)
	if err != nil {
		t.Fatal(err)
	}
	if tx.Confirmations > 0 {
		t.Skip("Skipping test, transaction moved away from mepool")
	}

	return tx
}

func (rt *Test) TestGetTransactionForMempool(t *testing.T) {
	txs := rt.getMempool(t)
	txid := txs[rand.Intn(len(txs))]
	got := rt.getMempoolTransaction(t, txid)

	want, err := rt.Client.GetTransaction(txid)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("GetTransactionForMempool() got %v, want %v", got, want)
	}
}

func (rt *Test) getMempoolAddresses(t *testing.T, txs []string) map[string][]string {
	txid2addrs := map[string][]string{}
	for i := 0; i < len(txs); i++ {
		tx := rt.getMempoolTransaction(t, txs[i])
		addrs := []string{}
		for _, vin := range tx.Vin {
			for _, a := range vin.Addresses {
				addrs = append(addrs, a)
			}
		}
		for _, vout := range tx.Vout {
			for _, a := range vout.ScriptPubKey.Addresses {
				addrs = append(addrs, a)
			}
		}
		if len(addrs) > 0 {
			txid2addrs[tx.Txid] = addrs
		}
	}
	return txid2addrs
}

func (rt *Test) TestMempoolSync(t *testing.T) {
	for i := 0; i < 3; i++ {
		txs := rt.getMempool(t)
		txid2addrs := rt.getMempoolAddresses(t, txs)
		if len(txid2addrs) == 0 {
			t.Fatal("No transaction in mempool has any address")
		}

		n, err := rt.Client.ResyncMempool(nil)
		if err != nil {
			t.Fatal(err)
		}

		if tmp := rt.getMempool(t); len(txs) != len(tmp) {
			// mempool reset
			continue
		}

		if len(txs) != n {
			t.Fatalf("ResyncMempool() returned different number of transactions than backend call")
		}

		for txid, addrs := range txid2addrs {
			for _, a := range addrs {
				txs, err := rt.Client.GetMempoolTransactions(a)
				if err != nil {
					t.Fatal(err)
				}
				if !containsString(txs, txid) {
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

func containsString(slice []string, s string) bool {
	for i := 0; i < len(slice); i++ {
		if slice[i] == s {
			return true
		}
	}
	return false
}

func (rt *Test) TestGetMempoolEntry(t *testing.T) {
	for i := 0; i < 3; i++ {
		txs := rt.getMempool(t)
		h, err := rt.Client.GetBestBlockHeight()
		if err != nil {
			t.Fatal(err)
		}

		tx := rt.getMempoolTransaction(t, txs[rand.Intn(len(txs))])
		e, err := rt.Client.GetMempoolEntry(tx.Txid)
		if err != nil {
			if err, ok := err.(*bchain.RPCError); ok && err.Code == -5 {
				// mempool reset
				continue
			}
		}

		if e.Height != h {
			t.Errorf("GetMempoolEntry() got height %d, want %d", e.Height, h)
		}
		if e.Size <= 0 {
			t.Errorf("GetMempoolEntry() got zero or negative size %d", e.Size)
		}
		if e.Fee <= 0 {
			t.Errorf("GetMempoolEntry() got zero or negative fee %f", e.Fee)
		}

		// done
		return
	}
	t.Skip("Skipping test, all attempts to get mempool entry failed due to network state changes")
}

func (rt *Test) TestSendRawTransaction(t *testing.T) {
	for txid, tx := range rt.TestData.TxDetails {
		_, err := rt.Client.SendRawTransaction(tx.Hex)
		if err != nil {
			if err, ok := err.(*bchain.RPCError); ok && err.Code == -27 {
				continue
			}
		}
		t.Errorf("SendRawTransaction() for %s returned unexpected error: %#v", txid, err)
	}
}

func (rt *Test) TestEstimateSmartFee(t *testing.T) {
	for _, blocks := range []int{1, 2, 3, 5, 10} {
		fee, err := rt.Client.EstimateSmartFee(blocks, true)
		if err != nil {
			t.Error(err)
		}
		if fee != -1 && (fee < 0 || fee > 1) {
			t.Errorf("EstimateSmartFee() returned unexpected fee rate: %f", fee)
		}
	}
}

func (rt *Test) TestEstimateFee(t *testing.T) {
	for _, blocks := range []int{1, 2, 3, 5, 10} {
		fee, err := rt.Client.EstimateFee(blocks)
		if err != nil {
			t.Error(err)
		}
		if fee != -1 && (fee < 0 || fee > 1) {
			t.Errorf("EstimateFee() returned unexpected fee rate: %f", fee)
		}
	}
}
