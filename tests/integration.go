//go:build integration

package tests

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins"
	build "github.com/trezor/blockbook/build/tools"
	"github.com/trezor/blockbook/tests/rpc"
	"github.com/trezor/blockbook/tests/sync"
)

type TestFunc func(t *testing.T, coin string, chain bchain.BlockChain, mempool bchain.Mempool, testConfig json.RawMessage)

var integrationTests = map[string]TestFunc{
	"rpc":  rpc.IntegrationTest,
	"sync": sync.IntegrationTest,
}

var notConnectedError = errors.New("Not connected to backend server")

func runIntegrationTests(t *testing.T) {
	tests, err := loadTests("tests.json")
	if err != nil {
		t.Fatal(err)
	}

	keys := make([]string, 0, len(tests))
	for k := range tests {
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

	bc, m, err := makeBlockChain(coin)
	if err != nil {
		if err == notConnectedError {
			t.Fatal(err)
		}
		t.Fatalf("Cannot init blockchain: %s", err)
	}

	for test, c := range cfg {
		if fn, found := integrationTests[test]; found {
			t.Run(test, func(t *testing.T) { fn(t, coin, bc, m, c) })
		} else {
			t.Errorf("Test not found: %s", test)
		}
	}
}

func makeBlockChain(coin string) (bchain.BlockChain, bchain.Mempool, error) {
	c, err := build.LoadConfig("../configs", coin)
	if err != nil {
		return nil, nil, err
	}

	outputDir, err := ioutil.TempDir("", "integration_test")
	if err != nil {
		return nil, nil, err
	}
	defer os.RemoveAll(outputDir)

	err = build.GeneratePackageDefinitions(c, "../build/templates", outputDir)
	if err != nil {
		return nil, nil, err
	}

	b, err := ioutil.ReadFile(filepath.Join(outputDir, "blockbook", "blockchaincfg.json"))
	if err != nil {
		return nil, nil, err
	}

	var cfg json.RawMessage
	err = json.Unmarshal(b, &cfg)
	if err != nil {
		return nil, nil, err
	}

	coinName, err := getName(cfg)
	if err != nil {
		return nil, nil, err
	}

	return initBlockChain(coinName, cfg)
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

func initBlockChain(coinName string, cfg json.RawMessage) (bchain.BlockChain, bchain.Mempool, error) {
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

func isNetError(err error) bool {
	if _, ok := err.(net.Error); ok {
		return true
	}
	return false
}
