// +build integration

package rpc

import (
	"blockbook/bchain"
	"encoding/json"
	"reflect"
	"testing"
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
