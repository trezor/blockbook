package eth

import (
	"encoding/json"
	"testing"
	"time"
)

func TestConfigurationMempoolTxTimeoutDuration(t *testing.T) {
	tests := []struct {
		name                       string
		config                     Configuration
		alternativeProviderEnabled bool
		want                       time.Duration
	}{
		{
			name: "legacy hours without alternative provider",
			config: Configuration{
				MempoolTxTimeoutHours: 12,
			},
			want: 12 * time.Hour,
		},
		{
			name: "alternative provider default",
			config: Configuration{
				MempoolTxTimeoutHours: 12,
			},
			alternativeProviderEnabled: true,
			want:                       10 * time.Minute,
		},
		{
			name: "explicit duration overrides alternative provider default",
			config: Configuration{
				MempoolTxTimeoutHours: 12,
				MempoolTxTimeout:      "15m",
			},
			alternativeProviderEnabled: true,
			want:                       15 * time.Minute,
		},
		{
			name: "legacy zero is preserved",
			config: Configuration{
				MempoolTxTimeoutHours: 0,
			},
			want: 0,
		},
		{
			name: "explicit zero duration is preserved",
			config: Configuration{
				MempoolTxTimeoutHours: 12,
				MempoolTxTimeout:      "0s",
			},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.config.MempoolTxTimeoutDuration(tt.alternativeProviderEnabled)
			if err != nil {
				t.Fatalf("MempoolTxTimeoutDuration() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("MempoolTxTimeoutDuration() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestConfigurationAlternativeMempoolTxTimeoutDuration(t *testing.T) {
	tests := []struct {
		name   string
		config Configuration
		want   time.Duration
	}{
		{
			name: "default",
			want: 5 * time.Minute,
		},
		{
			name: "explicit duration",
			config: Configuration{
				AlternativeMempoolTxTimeout: "7m",
			},
			want: 7 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.config.AlternativeMempoolTxTimeoutDuration()
			if err != nil {
				t.Fatalf("AlternativeMempoolTxTimeoutDuration() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("AlternativeMempoolTxTimeoutDuration() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestNewEthereumRPCRejectsInvalidMempoolTimeouts(t *testing.T) {
	tests := []struct {
		name   string
		config string
	}{
		{
			name: "invalid blockbook mempool timeout",
			config: `{
				"coin_name":"Ethereum",
				"coin_shortcut":"ETH",
				"rpc_timeout":25,
				"mempoolTxTimeout":"not-a-duration",
				"block_addresses_to_keep":600
			}`,
		},
		{
			name: "zero alternative mempool timeout",
			config: `{
				"coin_name":"Ethereum",
				"coin_shortcut":"ETH",
				"rpc_timeout":25,
				"alternativeMempoolTxTimeout":"0s",
				"block_addresses_to_keep":600
			}`,
		},
		{
			name: "negative blockbook mempool timeout",
			config: `{
				"coin_name":"Ethereum",
				"coin_shortcut":"ETH",
				"rpc_timeout":25,
				"mempoolTxTimeout":"-1s",
				"block_addresses_to_keep":600
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewEthereumRPC(json.RawMessage(tt.config), nil)
			if err == nil {
				t.Fatal("expected timeout configuration error")
			}
		})
	}
}

func TestInitAlternativeProvidersUsesAlternativeMempoolTxTimeout(t *testing.T) {
	t.Setenv("ETH_ALTERNATIVE_SENDTX_URLS", "http://localhost:8545")

	tests := []struct {
		name   string
		config Configuration
		want   time.Duration
	}{
		{
			name: "default",
			config: Configuration{
				CoinShortcut: "eth",
				RPCTimeout:   1,
			},
			want: 5 * time.Minute,
		},
		{
			name: "explicit duration",
			config: Configuration{
				CoinShortcut:                "eth",
				RPCTimeout:                  1,
				AlternativeMempoolTxTimeout: "7m",
			},
			want: 7 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &EthereumRPC{
				ChainConfig: &tt.config,
			}
			if err := b.InitAlternativeProviders(); err != nil {
				t.Fatalf("InitAlternativeProviders() error = %v", err)
			}

			if b.alternativeSendTxProvider == nil {
				t.Fatal("alternativeSendTxProvider is nil")
			}
			if got := b.alternativeSendTxProvider.mempoolTxsTimeout; got != tt.want {
				t.Fatalf("mempoolTxsTimeout = %s, want %s", got, tt.want)
			}
		})
	}
}
