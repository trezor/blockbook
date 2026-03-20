//go:build integration

package tests

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins"
	apitests "github.com/trezor/blockbook/tests/api"
	"github.com/trezor/blockbook/tests/connectivity"
	"github.com/trezor/blockbook/tests/rpc"
	synctests "github.com/trezor/blockbook/tests/sync"
)

type TestFunc func(t *testing.T, coin string, chain bchain.BlockChain, mempool bchain.Mempool, testConfig json.RawMessage)

type integrationTest struct {
	fn            TestFunc
	requiresChain bool
}

// integrationTests maps test group names from tests.json to their handlers.
// "connectivity" performs lightweight backend reachability checks.
// "rpc" runs per-coin RPC fixtures against a fully initialized chain.
// "sync" exercises block connection/rollback logic and needs a live backend + chain init.
var integrationTests = map[string]integrationTest{
	"rpc":          {fn: rpc.IntegrationTest, requiresChain: true},
	"sync":         {fn: synctests.IntegrationTest, requiresChain: true},
	"connectivity": {fn: connectivity.IntegrationTest, requiresChain: false},
	"api":          {fn: apitests.IntegrationTest, requiresChain: false},
}

var notConnectedError = errors.New("Not connected to backend server")

func runIntegrationTests(t *testing.T) {
	tests, err := loadTests("tests.json")
	if err != nil {
		t.Fatal(err)
	}

	keys := make([]string, 0, len(tests))
	for k, cfg := range tests {
		if !hasConnectivity(cfg) {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, coin := range keys {
		cfg := tests[coin]
		name := getMatchableName(coin)
		t.Run(name, func(t *testing.T) { runTests(t, coin, cfg) })

	}
}

func loadTests(path string) (map[string]map[string]json.RawMessage, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	v := make(map[string]map[string]json.RawMessage)
	err = json.Unmarshal(b, &v)
	return v, err
}

func getMatchableName(coin string) string {
	if idx := strings.Index(coin, "_testnet"); idx != -1 {
		return coin[:idx] + "=test"
	} else {
		return coin + "=main"
	}
}

func runTests(t *testing.T, coin string, cfg map[string]json.RawMessage) {
	if cfg == nil || len(cfg) == 0 {
		t.Skip("No tests to run")
	}
	defer chaincfg.ResetParams()

	var (
		bc       bchain.BlockChain
		m        bchain.Mempool
		initOnce sync.Once
		initErr  error
	)
	needsMempool := requiresMempool(cfg)
	ensureChain := func(t *testing.T) {
		t.Helper()
		initOnce.Do(func() {
			bc, m, initErr = makeBlockChain(coin, needsMempool)
		})
		if initErr != nil {
			if initErr == notConnectedError {
				t.Fatal(initErr)
			}
			t.Fatalf("Cannot init blockchain: %s", initErr)
		}
	}

	for test, c := range cfg {
		if def, found := integrationTests[test]; found {
			t.Run(test, func(t *testing.T) {
				if def.requiresChain {
					ensureChain(t)
				}
				def.fn(t, coin, bc, m, c)
			})
		} else {
			t.Errorf("Test not found: %s", test)
		}
	}
}

func makeBlockChain(coin string, needsMempool bool) (bchain.BlockChain, bchain.Mempool, error) {
	cfg, err := bchain.LoadBlockchainCfgRaw(coin)
	if err != nil {
		return nil, nil, err
	}

	coinName, err := getName(cfg)
	if err != nil {
		return nil, nil, err
	}

	return initBlockChain(coinName, cfg, needsMempool)
}

func getName(raw json.RawMessage) (string, error) {
	var cfg map[string]interface{}
	err := json.Unmarshal(raw, &cfg)
	if err != nil {
		return "", err
	}
	if n, found := cfg["coin_name"]; found {
		switch n := n.(type) {
		case string:
			return n, nil
		default:
			return "", fmt.Errorf("Unexpected type of field `name`: %s", reflect.TypeOf(n))
		}
	} else {
		return "", errors.New("Missing field `name`")
	}
}

func initBlockChain(coinName string, cfg json.RawMessage, initMempool bool) (bchain.BlockChain, bchain.Mempool, error) {
	factory, found := coins.BlockChainFactories[coinName]
	if !found {
		return nil, nil, fmt.Errorf("Factory function not found")
	}

	chain, err := factory(cfg, func(_ bchain.NotificationType) {})
	if err != nil {
		if isNetError(err) {
			return nil, nil, notConnectedError
		}
		return nil, nil, fmt.Errorf("Factory function failed: %s", err)
	}

	for i := 0; ; i++ {
		err = chain.Initialize()
		if err == nil {
			break
		}
		if isNetError(err) {
			return nil, nil, notConnectedError
		}
		// wait max 5 minutes for backend to startup
		if i > 5*60 {
			return nil, nil, fmt.Errorf("BlockChain initialization failed: %s", err)
		}
		time.Sleep(time.Millisecond * 1000)
	}

	if initMempool {
		mempool, err := chain.CreateMempool(chain)
		if err != nil {
			return nil, nil, fmt.Errorf("Mempool creation failed: %s", err)
		}

		err = chain.InitializeMempool(nil, nil, nil)
		if err != nil {
			return nil, nil, fmt.Errorf("Mempool initialization failed: %s", err)
		}

		return chain, mempool, nil
	}

	return chain, nil, nil
}

func isNetError(err error) bool {
	if _, ok := err.(net.Error); ok {
		return true
	}
	return false
}

func requiresMempool(cfg map[string]json.RawMessage) bool {
	tests, ok := cfg["rpc"]
	if !ok || len(tests) == 0 {
		return false
	}
	var rpcTests []string
	if err := json.Unmarshal(tests, &rpcTests); err != nil {
		return true
	}
	for _, test := range rpcTests {
		if test == "MempoolSync" || test == "GetTransactionForMempool" {
			return true
		}
	}
	return false
}

func hasConnectivity(cfg map[string]json.RawMessage) bool {
	_, ok := cfg["connectivity"]
	return ok
}
