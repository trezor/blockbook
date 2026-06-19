//go:build integration

package connectivity

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/rpc"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins"
)

const connectivityTimeout = 10 * time.Second

// backendConnectivityEnvVar gates the raw backend/node RPC reachability checks.
const backendConnectivityEnvVar = "BB_TEST_BACKEND_CONNECTIVITY"

// backendConnectivityEnabled reports whether the raw backend/node RPC reachability
// checks should run in addition to the Blockbook API checks. Dialing the node RPC
// endpoints directly only works from the CI/CD network, so these checks are gated
// behind BB_TEST_BACKEND_CONNECTIVITY and skipped by default for local runs, which
// still verify Blockbook reachability.
func backendConnectivityEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(backendConnectivityEnvVar))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

type connectivityCfg struct {
	CoinName string `json:"coin_name"`
	RpcUrl   string `json:"rpc_url"`
	RpcUrlWs string `json:"rpc_url_ws"`
	RpcUser  string `json:"rpc_user"`
	RpcPass  string `json:"rpc_pass"`
}

// IntegrationTest runs connectivity checks for the requested modes (e.g., ["http","ws"]).
// HTTP mode verifies both backend RPC and Blockbook HTTP API accessibility.
// WS mode verifies both backend WS RPC and Blockbook websocket accessibility.
func IntegrationTest(t *testing.T, coin string, _ bchain.BlockChain, _ bchain.Mempool, testConfig json.RawMessage) {
	t.Helper()

	modes, err := parseConnectivityModes(testConfig)
	if err != nil {
		t.Fatalf("invalid connectivity config for %s: %v", coin, err)
	}

	backendEnabled := backendConnectivityEnabled()
	if !backendEnabled {
		t.Logf("%s: skipping backend/node RPC connectivity (set %s=1 to enable, e.g. in CI); checking Blockbook only",
			coin, backendConnectivityEnvVar)
	}

	for _, mode := range modes {
		switch mode {
		case "http":
			if backendEnabled {
				HTTPIntegrationTest(t, coin, nil, nil, nil)
			}
			BlockbookHTTPIntegrationTest(t, coin, nil, nil, nil)
		case "ws":
			if backendEnabled {
				WSIntegrationTest(t, coin, nil, nil, nil)
			}
			BlockbookWSIntegrationTest(t, coin, nil, nil, nil)
		default:
			t.Fatalf("unsupported connectivity mode %q for %s", mode, coin)
		}
	}
}

func parseConnectivityModes(testConfig json.RawMessage) ([]string, error) {
	var modes []string
	if err := json.Unmarshal(testConfig, &modes); err != nil {
		return nil, err
	}
	if len(modes) == 0 {
		return nil, errors.New("empty connectivity list")
	}
	return modes, nil
}

func HTTPIntegrationTest(t *testing.T, coin string, _ bchain.BlockChain, _ bchain.Mempool, _ json.RawMessage) {
	t.Helper()

	rawCfg, cfg := loadConnectivityCfg(t, coin)
	if cfg.RpcUrl == "" {
		t.Fatalf("empty rpc_url for %s", coin)
	}

	if isUTXO(cfg) {
		if cfg.CoinName == "" {
			t.Fatalf("empty coin_name for %s", coin)
		}
		factory, ok := coins.BlockChainFactories[cfg.CoinName]
		if !ok {
			t.Fatalf("blockchain factory not found for %s", cfg.CoinName)
		}
		chain, err := factory(rawCfg, func(bchain.NotificationType) {})
		if err != nil {
			t.Fatalf("init chain %s: %v", cfg.CoinName, err)
		}
		if _, err := chain.GetChainInfo(); err != nil {
			t.Fatalf("GetChainInfo %s: %v", cfg.CoinName, err)
		}
		return
	}

	evmHTTPConnectivity(t, cfg.RpcUrl)
}

func WSIntegrationTest(t *testing.T, coin string, _ bchain.BlockChain, _ bchain.Mempool, _ json.RawMessage) {
	t.Helper()

	_, cfg := loadConnectivityCfg(t, coin)
	if cfg.RpcUrlWs == "" {
		t.Fatalf("empty rpc_url_ws for %s", coin)
	}

	evmWSConnectivity(t, cfg.RpcUrlWs)
}

func loadConnectivityCfg(t *testing.T, coin string) (json.RawMessage, connectivityCfg) {
	t.Helper()

	rawCfg, err := bchain.LoadBlockchainCfgRaw(coin)
	if err != nil {
		t.Fatalf("load blockchain config for %s: %v", coin, err)
	}
	var cfg connectivityCfg
	if err := json.Unmarshal(rawCfg, &cfg); err != nil {
		t.Fatalf("unmarshal blockchain config for %s: %v", coin, err)
	}
	return rawCfg, cfg
}

func isUTXO(cfg connectivityCfg) bool {
	return cfg.RpcUser != "" || cfg.RpcPass != ""
}

func evmHTTPConnectivity(t *testing.T, httpURL string) {
	t.Helper()

	rpcClient, err := rpc.DialOptions(context.Background(), httpURL)
	if err != nil {
		t.Fatalf("dial rpc_url %s: %v", httpURL, err)
	}
	defer rpcClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), connectivityTimeout)
	defer cancel()

	var version string
	if err := rpcClient.CallContext(ctx, &version, "web3_clientVersion"); err != nil {
		t.Fatalf("CallContext web3_clientVersion failed: %v", err)
	}
	if version == "" {
		t.Fatalf("empty web3_clientVersion")
	}
}

func evmWSConnectivity(t *testing.T, wsURL string) {
	t.Helper()

	rpcClient, err := rpc.DialOptions(context.Background(), wsURL, rpc.WithWebsocketMessageSizeLimit(0))
	if err != nil {
		t.Fatalf("dial rpc_url_ws %s: %v", wsURL, err)
	}
	defer rpcClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), connectivityTimeout)
	defer cancel()

	var version string
	if err := rpcClient.CallContext(ctx, &version, "web3_clientVersion"); err != nil {
		t.Fatalf("CallContext web3_clientVersion failed: %v", err)
	}
	if version == "" {
		t.Fatalf("empty web3_clientVersion")
	}

	subCtx, subCancel := context.WithTimeout(context.Background(), connectivityTimeout)
	defer subCancel()

	sub, err := rpcClient.EthSubscribe(subCtx, make(chan interface{}, 1), "newHeads")
	if err != nil {
		t.Fatalf("EthSubscribe newHeads failed: %v", err)
	}
	sub.Unsubscribe()
}
