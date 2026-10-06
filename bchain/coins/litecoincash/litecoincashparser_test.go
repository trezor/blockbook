package litecoincash

import (
	"encoding/hex"
	"testing"

	"github.com/trezor/blockbook/bchain/coins/btc"
)

func TestGetAddrDescFromAddressLegacyP2SHRegtest(t *testing.T) {
	parser := NewLitecoinCashParser(GetChainParams("regtest"), &btc.Configuration{})

	const want = "a914d5517e329cf067d1f02176b466af9c4777d6406387"

	tests := []struct {
		name    string
		address string
	}{
		{
			name:    "current SCRIPT_ADDRESS2 prefix 58",
			address: "Qg3ufPWSawdf2x5R1P3C74GbZBPjsC7a9g",
		},
		{
			name:    "legacy SCRIPT_ADDRESS prefix 196",
			address: "2NCh9YNeCZqaZtmKNPH1B2MqAQnwuymKQsF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.GetAddrDescFromAddress(tt.address)
			if err != nil {
				t.Fatalf("GetAddrDescFromAddress(%q): %v", tt.address, err)
			}

			if gotHex := hex.EncodeToString(got); gotHex != want {
				t.Fatalf("GetAddrDescFromAddress(%q) = %s, want %s", tt.address, gotHex, want)
			}
		})
	}
}

func TestGetAddrDescFromAddressLegacyP2SHMainnet(t *testing.T) {
	parser := NewLitecoinCashParser(GetChainParams("main"), &btc.Configuration{})

	const want = "a914d5517e329cf067d1f02176b466af9c4777d6406387"

	tests := []struct {
		name    string
		address string
	}{
		{
			name:    "current SCRIPT_ADDRESS2 prefix 50",
			address: "MTM5nX88uVveVUxip2NeE46JX9LC6XGTx7",
		},
		{
			name:    "legacy SCRIPT_ADDRESS prefix 5",
			address: "3M8wUdiAxP5Dgygpi9PJQQquCSjkA83gve",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.GetAddrDescFromAddress(tt.address)
			if err != nil {
				t.Fatalf("GetAddrDescFromAddress(%q): %v", tt.address, err)
			}

			if gotHex := hex.EncodeToString(got); gotHex != want {
				t.Fatalf("GetAddrDescFromAddress(%q) = %s, want %s", tt.address, gotHex, want)
			}
		})
	}
}
