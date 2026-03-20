//go:build integration

package tests

import (
	"testing"

	"github.com/trezor/blockbook/bchain"
)

// TestLoadBlockchainCfgEnvOverride verifies env-based overrides land in blockchaincfg.json.
func TestLoadBlockchainCfgEnvOverride(t *testing.T) {
	const wantHTTP = "http://backend_hostname:1234"
	const wantWS = "ws://backend_hostname:1234"
	t.Setenv("BB_RPC_URL_HTTP_ethereum", wantHTTP)
	t.Setenv("BB_RPC_URL_WS_ethereum", wantWS)

	cfg := bchain.LoadBlockchainCfg(t, "ethereum")
	if cfg.RpcUrl != wantHTTP {
		t.Fatalf("expected rpc_url %q, got %q", wantHTTP, cfg.RpcUrl)
	}
	if cfg.RpcUrlWs != wantWS {
		t.Fatalf("expected rpc_url_ws %q, got %q", wantWS, cfg.RpcUrlWs)
	}
}
