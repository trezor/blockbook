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
			name: "alternative provider default is the pending window plus the sweep margin",
			config: Configuration{
				MempoolTxTimeoutHours: 12,
			},
			alternativeProviderEnabled: true,
			want:                       defaultAlternativePendingTxWindow + mempoolRetentionMarginOverPendingWindow,
		},
		{
			name: "a configured pending window carries into the mempool default",
			config: Configuration{
				MempoolTxTimeoutHours:      12,
				AlternativePendingTxWindow: "1h",
			},
			alternativeProviderEnabled: true,
			want:                       time.Hour + mempoolRetentionMarginOverPendingWindow,
		},
		{
			name: "a cache-only override carries into the mempool default too",
			config: Configuration{
				MempoolTxTimeoutHours:       12,
				AlternativePendingTxWindow:  "3h",
				AlternativeMempoolTxTimeout: "20m",
			},
			alternativeProviderEnabled: true,
			want:                       20*time.Minute + mempoolRetentionMarginOverPendingWindow,
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
			name: "default is the pending window",
			want: defaultAlternativePendingTxWindow,
		},
		{
			name: "follows a configured pending window",
			config: Configuration{
				AlternativePendingTxWindow: "90m",
			},
			want: 90 * time.Minute,
		},
		{
			name: "an explicit cache retention overrides the window",
			config: Configuration{
				AlternativePendingTxWindow:  "3h",
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

// TestMempoolRetentionInverted covers the retention-order check: the provider cache must expire
// before the wrapped mempool, whose timeout sweep is the only exit that does not clear the cache.
func TestMempoolRetentionInverted(t *testing.T) {
	tests := []struct {
		name        string
		alternative time.Duration
		mempool     time.Duration
		want        bool
	}{
		{
			name:        "defaults are ordered correctly",
			alternative: defaultAlternativePendingTxWindow,
			mempool:     defaultAlternativePendingTxWindow + mempoolRetentionMarginOverPendingWindow,
			want:        false,
		},
		{
			name:        "cache outliving the mempool is inverted",
			alternative: 30 * time.Minute,
			mempool:     10 * time.Minute,
			want:        true,
		},
		{
			name:        "equal retentions are inverted",
			alternative: 10 * time.Minute,
			mempool:     10 * time.Minute,
			want:        true,
		},
		{
			name:        "a zero mempool retention is inverted",
			alternative: 5 * time.Minute,
			mempool:     0,
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mempoolRetentionInverted(tt.alternative, tt.mempool); got != tt.want {
				t.Fatalf("mempoolRetentionInverted(%s, %s) = %v, want %v", tt.alternative, tt.mempool, got, tt.want)
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
			name: "zero alternative pending window",
			config: `{
				"coin_name":"Ethereum",
				"coin_shortcut":"ETH",
				"rpc_timeout":25,
				"alternativePendingTxWindow":"0s",
				"block_addresses_to_keep":600
			}`,
		},
		{
			name: "unparsable alternative pending window",
			config: `{
				"coin_name":"Ethereum",
				"coin_shortcut":"ETH",
				"rpc_timeout":25,
				"alternativePendingTxWindow":"three hours",
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

// TestCreateMempoolRejectsInvertedRetention pins that an inverted pair fails startup rather than
// warning: inverted, the mempool's timeout sweep drops a private transaction's address index while the
// cache keeps serving its body as pending, the #1573 symptom arrived at silently. Only an explicit
// mempoolTxTimeout can get there, since the default is derived from the cache retention.
func TestCreateMempoolRejectsInvertedRetention(t *testing.T) {
	for _, tt := range []struct {
		name      string
		config    Configuration
		wantError bool
	}{
		{
			name:      "explicit mempool timeout below the pending window",
			config:    Configuration{MempoolTxTimeout: "10m", AlternativePendingTxWindow: "3h"},
			wantError: true,
		},
		{
			name:      "explicit mempool timeout equal to the pending window",
			config:    Configuration{MempoolTxTimeout: "3h", AlternativePendingTxWindow: "3h"},
			wantError: true,
		},
		{
			name:   "explicit mempool timeout above the pending window",
			config: Configuration{MempoolTxTimeout: "4h", AlternativePendingTxWindow: "3h"},
		},
		{
			name:   "derived defaults are never inverted",
			config: Configuration{AlternativePendingTxWindow: "3h"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			alternative, err := tt.config.AlternativeMempoolTxTimeoutDuration()
			if err != nil {
				t.Fatalf("AlternativeMempoolTxTimeoutDuration() error = %v", err)
			}
			b := &EthereumRPC{
				ChainConfig:               &tt.config,
				alternativeSendTxProvider: &AlternativeSendTxProvider{mempoolTxsTimeout: alternative},
			}
			_, err = b.CreateMempool(nil)
			if tt.wantError && err == nil {
				t.Fatal("inverted retention pair was accepted")
			}
			if !tt.wantError && err != nil {
				t.Fatalf("CreateMempool() error = %v", err)
			}
			if tt.wantError {
				// a rejected pair must not leave a mempool behind, or a second call hands it out with
				// the provider never wired to it
				if b.Mempool != nil {
					t.Fatal("rejected configuration left a mempool behind")
				}
				if _, err := b.CreateMempool(nil); err == nil {
					t.Fatal("a second CreateMempool accepted the same inverted pair")
				}
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
			want: defaultAlternativePendingTxWindow,
		},
		{
			name: "configured pending window",
			config: Configuration{
				CoinShortcut:               "eth",
				RPCTimeout:                 1,
				AlternativePendingTxWindow: "90m",
			},
			want: 90 * time.Minute,
		},
		{
			name: "explicit cache retention",
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
