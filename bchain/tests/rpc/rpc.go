// +build integration

package rpc

import (
	"blockbook/bchain"
	"encoding/json"
	"math/rand"
	"net"
	"reflect"
	"testing"

	"github.com/deckarep/golang-set"
)

type TestConfig struct {
	URL  string `json:"url"`
	User string `json:"user"`
	Pass string `json:"pass"`
}

type TestData struct {
	BlockHeight uint32                `json:"blockHeight"`
	BlockHash   string                `json:"blockHash"`
	BlockTxs    []string              `json:"blockTxs"`
	TxDetails   map[string]*bchain.Tx `json:"txDetails"`
}

type Test struct {
	Client    bchain.BlockChain
	TestData  *TestData
	connected bool
}

type TestChainFactoryFunc func(json.RawMessage) (bchain.BlockChain, error)

func NewTest(coin string, factory TestChainFactoryFunc) (*Test, error) {
	var (
		connected = true
		cli       bchain.BlockChain
		cfg       json.RawMessage
		td        *TestData
		err       error
	)

	cfg, err = LoadRPCConfig(coin)
	if err != nil {
		return nil, err
	}

	cli, err = factory(cfg)
	if err != nil {
		if isNetError(err) {
			connected = false
		} else {
			return nil, err
		}
	} else {
		td, err = LoadTestData(coin)
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

		_, err = cli.GetBlockChainInfo()
		if err != nil && isNetError(err) {
			connected = false
		}
	}

	return &Test{Client: cli, TestData: td, connected: connected}, nil
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
				tx.Vout[i].Address = tmp.Vout[i].Address
			}
		}
	}
	return err
}

func (rt *Test) skipUnconnected(t *testing.T) {
	if !rt.connected {
		t.Skip("Skipping test, not connected to backend service")
	}
}

func (rt *Test) TestGetBlockHash(t *testing.T) {
	rt.skipUnconnected(t)

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
	rt.skipUnconnected(t)

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
	rt.skipUnconnected(t)

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

func (rt *Test) TestGetTransactionForMempool(t *testing.T) {
	rt.skipUnconnected(t)

	for txid, want := range rt.TestData.TxDetails {
		// reset fields that are not parsed
		want.Confirmations = 0
		want.Blocktime = 0
		want.Time = 0

		got, err := rt.Client.GetTransactionForMempool(txid)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("GetTransactionForMempool() got %v, want %v", got, want)
		}
	}
}

func (rt *Test) getMempool(t *testing.T) []string {
	txs, err := rt.Client.GetMempool()
	if err != nil {
		t.Fatal(err)
	}
	if len(txs) == 0 {
		t.Skip("Skipping test, mempool is empty")
	}

	return txs
}

func (rt *Test) getMempoolAddresses(t *testing.T, txs []string) map[string][]string {
	txid2addrs := map[string][]string{}
	for i := 0; i < len(txs); i++ {
		tx, err := rt.Client.GetTransactionForMempool(txs[i])
		if err != nil {
			t.Fatal(err)
		}
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
	rt.skipUnconnected(t)

	for i := 0; i < 3; i++ {
		txs := rt.getMempool(t)

		n, err := rt.Client.ResyncMempool(nil)
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			// no transactions to test
			continue
		}

		txs = intersect(txs, rt.getMempool(t))
		if len(txs) == 0 {
			// no transactions to test
			continue
		}

		txid2addrs := rt.getMempoolAddresses(t, txs)
		if len(txid2addrs) == 0 {
			t.Skip("Skipping test, no addresses in mempool")
		}

		for txid, addrs := range txid2addrs {
			for _, a := range addrs {
				got, err := rt.Client.GetMempoolTransactions(a)
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

func (rt *Test) TestGetMempoolEntry(t *testing.T) {
	rt.skipUnconnected(t)

	for i := 0; i < 3; i++ {
		txs := rt.getMempool(t)
		h, err := rt.Client.GetBestBlockHeight()
		if err != nil {
			t.Fatal(err)
		}

		txid := txs[rand.Intn(len(txs))]
		tx, err := rt.Client.GetTransactionForMempool(txid)
		if err != nil {
			t.Fatal(err)
		}
		if tx.Confirmations > 0 {
			// tx confirmed
			continue
		}

		e, err := rt.Client.GetMempoolEntry(txid)
		if err != nil {
			if err, ok := err.(*bchain.RPCError); ok && err.Code == -5 {
				// tx confirmed
				continue
			}
			t.Fatal(err)
		}

		if d := int(e.Height) - int(h); d < -1 || d > 1 {
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
	rt.skipUnconnected(t)

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
	rt.skipUnconnected(t)

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
	rt.skipUnconnected(t)

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
