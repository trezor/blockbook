//go:build integration

package sync

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
)

var testMap = map[string]func(t *testing.T, th *TestHandler){
	"ConnectBlocks":         testConnectBlocks,
	"ConnectBlocksParallel": testConnectBlocksParallel,
	"HandleFork":            testHandleFork,
}

type TestHandler struct {
	Coin     string
	Chain    bchain.BlockChain
	TestData *TestData
}

type Range struct {
	Lower uint32 `json:"lower"`
	Upper uint32 `json:"upper"`
}

type TestData struct {
	ConnectBlocks struct {
		SyncRanges []Range              `json:"syncRanges"`
		Blocks     map[uint32]BlockInfo `json:"blocks"`
	} `json:"connectBlocks"`
	HandleFork struct {
		SyncRanges []Range            `json:"syncRanges"`
		FakeBlocks map[uint32]BlockID `json:"fakeBlocks"`
		RealBlocks map[uint32]BlockID `json:"realBlocks"`
	} `json:"handleFork"`
}

type BlockID struct {
	Height uint32 `json:"height"`
	Hash   string `json:"hash"`
}

type BlockInfo struct {
	BlockID
	NoTxs     uint32       `json:"noTxs"`
	TxDetails []*bchain.Tx `json:"txDetails"`
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

	for _, test := range tests {
		if f, found := testMap[test]; found {
			h := TestHandler{Coin: coin, Chain: chain, TestData: td}
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

	for _, b := range v.ConnectBlocks.Blocks {
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

	return &v, nil
}

func setTxAddresses(parser bchain.BlockChainParser, tx *bchain.Tx) error {
	for i := range tx.Vout {
		ad, err := parser.GetAddrDescFromVout(&tx.Vout[i])
		if err != nil {
			return err
		}
		a, s, err := parser.GetAddressesFromAddrDesc(ad)
		if err == nil && s {
			tx.Vout[i].ScriptPubKey.Addresses = a
		}
	}
	return nil
}

func makeRocksDB(parser bchain.BlockChainParser, m *common.Metrics, is *common.InternalState) (*db.RocksDB, func(), error) {
	p, err := ioutil.TempDir("", "sync_test")
	if err != nil {
		return nil, nil, err
	}

	d, err := db.NewRocksDB(p, 1<<17, 1<<14, parser, m, false)
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

var metricsRegistry = map[string]*common.Metrics{}

func getMetrics(name string) (*common.Metrics, error) {
	if m, found := metricsRegistry[name]; found {
		return m, nil
	} else {
		m, err := common.GetMetrics(name)
		if err != nil {
			return nil, err
		}
		metricsRegistry[name] = m
		return m, nil
	}
}

func withRocksDBAndSyncWorker(t *testing.T, h *TestHandler, startHeight uint32, fn func(*db.RocksDB, *db.SyncWorker, chan os.Signal)) {
	m, err := getMetrics(h.Coin)
	if err != nil {
		t.Fatal(err)
	}
	is := &common.InternalState{}

	d, closer, err := makeRocksDB(h.Chain.GetChainParser(), m, is)
	if err != nil {
		t.Fatal(err)
	}
	defer closer()

	ch := make(chan os.Signal)

	sw, err := db.NewSyncWorker(d, h.Chain, 8, 0, int(startHeight), false, ch, m, is)
	if err != nil {
		t.Fatal(err)
	}

	fn(d, sw, ch)
}
