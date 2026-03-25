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
	tests := []struct {
		name     string
		buildEnv string
		httpEnv  string
		wsEnv    string
	}{
		{
			name:    "default-dev",
			httpEnv: "BB_DEV_RPC_URL_HTTP_ethereum",
			wsEnv:   "BB_DEV_RPC_URL_WS_ethereum",
		},
		{
			name:     "prod",
			buildEnv: "prod",
			httpEnv:  "BB_PROD_RPC_URL_HTTP_ethereum",
			wsEnv:    "BB_PROD_RPC_URL_WS_ethereum",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("BB_BUILD_ENV", tt.buildEnv)
			t.Setenv(tt.httpEnv, wantHTTP)
			t.Setenv(tt.wsEnv, wantWS)

			cfg := bchain.LoadBlockchainCfg(t, "ethereum")
			if cfg.RpcUrl != wantHTTP {
				t.Fatalf("expected rpc_url %q, got %q", wantHTTP, cfg.RpcUrl)
			}
			if cfg.RpcUrlWs != wantWS {
				t.Fatalf("expected rpc_url_ws %q, got %q", wantWS, cfg.RpcUrlWs)
			}
		})
	}
}
