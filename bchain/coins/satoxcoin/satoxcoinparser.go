/*
 * Satoxcoin Blockbook Implementation
 * Copyright (C) 2025 Satoxcoin Core Developers
 *
 * This is a modified version of the original Blockbook project by Trezor,
 * customized to support Satoxcoin (SATOX) as the default blockchain explorer.
 * The original Blockbook project is available at: https://github.com/trezor/blockbook
 *
 * License: GNU Affero General Public License v3.0
 */

package satoxcoin

import (
	"strings"

	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

const (
	// MainnetMagic is mainnet network constant
	MainnetMagic wire.BitcoinNet = 0x63656556 // S A T T
	// TestnetMagic is testnet network constant
	TestnetMagic wire.BitcoinNet = 0x63656556 // S A T T
	// RegtestMagic is regtest network constant
	RegtestMagic wire.BitcoinNet = 0x444f5752 // D R O W
)

var (
	// MainNetParams are parser parameters for mainnet
	MainNetParams chaincfg.Params
	// TestNetParams are parser parameters for testnet
	TestNetParams chaincfg.Params
	// RegtestParams are parser parameters for regtest
	RegtestParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic

	// Address encoding magics - Satoxcoin uses 'S' prefix for mainnet
	MainNetParams.PubKeyHashAddrID = []byte{63}  // base58 prefix: S (PUBKEY_ADDRESS: 63)
	MainNetParams.ScriptHashAddrID = []byte{122} // base58 prefix: s (SCRIPT_ADDRESS: 122)

	// Extended key prefixes for BIP32 HD wallets
	MainNetParams.HDPrivateKeyID = [4]byte{0x04, 0x88, 0xAD, 0xE4} // xprv
	MainNetParams.HDPublicKeyID = [4]byte{0x04, 0x88, 0xB2, 0x1E}  // xpub

	// BIP44 coin type for Satoxcoin (SLIP 9007)
	MainNetParams.HDCoinType = 9007

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic

	// Address encoding magics - Satoxcoin uses 'S' prefix for testnet too
	TestNetParams.PubKeyHashAddrID = []byte{63}  // base58 prefix: S (PUBKEY_ADDRESS: 63)
	TestNetParams.ScriptHashAddrID = []byte{124} // base58 prefix: s (SCRIPT_ADDRESS: 124)

	// Extended key prefixes for BIP32 HD wallets (testnet)
	TestNetParams.HDPrivateKeyID = [4]byte{0x04, 0x35, 0x83, 0x94} // tprv
	TestNetParams.HDPublicKeyID = [4]byte{0x04, 0x35, 0x87, 0xCF}  // tpub

	// BIP44 coin type for Satoxcoin (SLIP 9007)
	TestNetParams.HDCoinType = 9007

	RegtestParams = chaincfg.RegressionNetParams
	RegtestParams.Net = RegtestMagic

	// Address encoding magics for regtest
	RegtestParams.PubKeyHashAddrID = []byte{66}  // base58 prefix: R (PUBKEY_ADDRESS: 42)
	RegtestParams.ScriptHashAddrID = []byte{124} // base58 prefix: s (SCRIPT_ADDRESS: 124)

	// Extended key prefixes for BIP32 HD wallets (regtest)
	RegtestParams.HDPrivateKeyID = [4]byte{0x04, 0x35, 0x83, 0x94} // tprv
	RegtestParams.HDPublicKeyID = [4]byte{0x04, 0x35, 0x87, 0xCF}  // tpub

	// BIP44 coin type for Satoxcoin (SLIP 9007)
	RegtestParams.HDCoinType = 9007
}

// SatoxcoinParser handle
type SatoxcoinParser struct {
	*btc.BitcoinLikeParser
	baseparser *bchain.BaseParser
}

// NewSatoxcoinParser returns new SatoxcoinParser instance
func NewSatoxcoinParser(params *chaincfg.Params, c *btc.Configuration) *SatoxcoinParser {
	p := &SatoxcoinParser{
		BitcoinLikeParser: btc.NewBitcoinLikeParser(params, c),
		baseparser:        &bchain.BaseParser{},
	}
	// Satoxcoin is asset-based and may have different witness format
	// Disable SegWit support to avoid parsing issues with asset transactions
	p.VSizeSupport = false
	return p
}

// GetChainParams contains network parameters for the main Satoxcoin network,
// the regression test Satoxcoin network, the test Satoxcoin network and
// the simulation test Satoxcoin network, in this order
func GetChainParams(chain string) *chaincfg.Params {
	// Register networks only once
	if !chaincfg.IsRegistered(&MainNetParams) {
		if err := chaincfg.Register(&MainNetParams); err != nil {
			// Ignore duplicate network errors
			if !strings.Contains(err.Error(), "duplicate") {
				panic(err)
			}
		}
	}
	if !chaincfg.IsRegistered(&TestNetParams) {
		if err := chaincfg.Register(&TestNetParams); err != nil {
			// Ignore duplicate network errors
			if !strings.Contains(err.Error(), "duplicate") {
				panic(err)
			}
		}
	}
	if !chaincfg.IsRegistered(&RegtestParams) {
		if err := chaincfg.Register(&RegtestParams); err != nil {
			// Ignore duplicate network errors
			if !strings.Contains(err.Error(), "duplicate") {
				panic(err)
			}
		}
	}
	switch chain {
	case "test":
		return &TestNetParams
	case "regtest":
		return &RegtestParams
	default:
		return &MainNetParams
	}
}

// PackTx packs transaction to byte array using protobuf
func (p *SatoxcoinParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	return p.baseparser.PackTx(tx, height, blockTime)
}

// UnpackTx unpacks transaction from protobuf byte array
func (p *SatoxcoinParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	return p.baseparser.UnpackTx(buf)
}

// ParseTx parses transaction from byte array
func (p *SatoxcoinParser) ParseTx(b []byte) (*bchain.Tx, error) {
	// Use the parent BitcoinLikeParser but with custom handling for Satoxcoin asset transactions
	return p.BitcoinLikeParser.ParseTx(b)
}
