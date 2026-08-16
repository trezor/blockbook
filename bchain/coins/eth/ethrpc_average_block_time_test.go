package eth

import (
	"encoding/json"
	"testing"
	"time"
)

func TestConfigurationAverageBlockTimeDuration(t *testing.T) {
	tests := []struct {
		name    string
		config  Configuration
		want    time.Duration
		wantErr bool
	}{
		{
			name:   "ethereum mainnet 12s slot",
			config: Configuration{AverageBlockTimeMs: 12000},
			want:   12 * time.Second,
		},
		{
			name:   "arbitrum sub-second",
			config: Configuration{AverageBlockTimeMs: 250},
			want:   250 * time.Millisecond,
		},
		{
			name:    "unset is rejected",
			config:  Configuration{},
			wantErr: true,
		},
		{
			name:    "negative is rejected",
			config:  Configuration{AverageBlockTimeMs: -1},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.config.AverageBlockTimeDuration()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("AverageBlockTimeDuration() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestNewEthereumRPCRequiresAverageBlockTimeMs(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		wantErr bool
	}{
		{
			name: "missing averageBlockTimeMs is rejected",
			config: `{
				"coin_name":"Ethereum",
				"coin_shortcut":"ETH",
				"rpc_timeout":25,
				"block_addresses_to_keep":600
			}`,
			wantErr: true,
		},
		{
			name: "zero averageBlockTimeMs is rejected",
			config: `{
				"coin_name":"Ethereum",
				"coin_shortcut":"ETH",
				"rpc_timeout":25,
				"block_addresses_to_keep":600,
				"averageBlockTimeMs":0
			}`,
			wantErr: true,
		},
		{
			name: "positive averageBlockTimeMs passes validation",
			config: `{
				"coin_name":"Ethereum",
				"coin_shortcut":"ETH",
				"rpc_timeout":25,
				"block_addresses_to_keep":600,
				"averageBlockTimeMs":12000
			}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewEthereumRPC(json.RawMessage(tt.config), nil)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected averageBlockTimeMs configuration error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
