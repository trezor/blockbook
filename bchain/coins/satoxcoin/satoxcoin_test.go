package satoxcoin

import (
	"testing"

	"github.com/trezor/blockbook/bchain/coins/btc"
)

func TestGetChainParams(t *testing.T) {
	// Test mainnet parameters
	mainnetParams := GetChainParams("main")
	if mainnetParams == nil {
		t.Fatal("Mainnet parameters should not be nil")
	}
	if mainnetParams.Net != MainnetMagic {
		t.Errorf("Expected mainnet magic %x, got %x", MainnetMagic, mainnetParams.Net)
	}
	if mainnetParams.PubKeyHashAddrID[0] != 99 {
		t.Errorf("Expected mainnet pubkey hash address ID 99, got %d", mainnetParams.PubKeyHashAddrID[0])
	}

	// Test testnet parameters
	testnetParams := GetChainParams("test")
	if testnetParams == nil {
		t.Fatal("Testnet parameters should not be nil")
	}
	if testnetParams.Net != TestnetMagic {
		t.Errorf("Expected testnet magic %x, got %x", TestnetMagic, testnetParams.Net)
	}
	if testnetParams.PubKeyHashAddrID[0] != 99 {
		t.Errorf("Expected testnet pubkey hash address ID 99, got %d", testnetParams.PubKeyHashAddrID[0])
	}

	// Test regtest parameters
	regtestParams := GetChainParams("regtest")
	if regtestParams == nil {
		t.Fatal("Regtest parameters should not be nil")
	}
	if regtestParams.Net != RegtestMagic {
		t.Errorf("Expected regtest magic %x, got %x", RegtestMagic, regtestParams.Net)
	}
	if regtestParams.PubKeyHashAddrID[0] != 66 {
		t.Errorf("Expected regtest pubkey hash address ID 66, got %d", regtestParams.PubKeyHashAddrID[0])
	}
}

func TestNewSatoxcoinParser(t *testing.T) {
	params := GetChainParams("main")
	// Create a minimal configuration for testing
	config := &btc.Configuration{
		CoinName:     "Satoxcoin",
		CoinShortcut: "SATOX",
	}
	parser := NewSatoxcoinParser(params, config)
	if parser == nil {
		t.Fatal("Parser should not be nil")
	}
}
