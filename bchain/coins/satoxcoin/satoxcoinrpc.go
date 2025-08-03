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
	"encoding/json"

	"github.com/golang/glog"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

// SatoxcoinRPC is an interface to JSON-RPC satoxcoind service
type SatoxcoinRPC struct {
	*btc.BitcoinRPC
}

// NewSatoxcoinRPC returns new SatoxcoinRPC instance
func NewSatoxcoinRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	b, err := btc.NewBitcoinRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	s := &SatoxcoinRPC{
		b.(*btc.BitcoinRPC),
	}
	s.RPCMarshaler = btc.JSONMarshalerV1{}

	return s, nil
}

// Initialize initializes SatoxcoinRPC instance.
func (b *SatoxcoinRPC) Initialize() error {
	ci, err := b.GetChainInfo()
	if err != nil {
		return err
	}
	chainName := ci.Chain

	params := GetChainParams(chainName)

	// always create parser
	b.Parser = NewSatoxcoinParser(params, b.ChainConfig)

	// parameters for getInfo request
	if params.Net == MainnetMagic {
		b.Testnet = false
		b.Network = "livenet"
	} else {
		b.Testnet = true
		b.Network = "testnet"
	}

	glog.Info("rpc: block chain ", params.Name)

	return nil
}
