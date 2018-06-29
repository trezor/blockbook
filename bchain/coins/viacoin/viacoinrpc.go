package viacoin

import (
	"blockbook/bchain/coins/btc"
	"encoding/json"
	"blockbook/bchain"
	"github.com/golang/glog"
)

// ViacoinRPC is an interface to JSON-RPC bitcoind service

type ViacoinRPC struct {
	*btc.BitcoinRPC
}

// NewViacoinRPC returns new ViacoinRPC instance
func NewViacoinRPC(config json.RawMessage, pushHandler func(notificationType bchain.NotificationType)) (bchain.BlockChain, error) {
	b, err := btc.NewBitcoinRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	s := &ViacoinRPC{
		b.(*btc.BitcoinRPC),
	}
	s.RPCMarshaler = btc.JSONMarshalerV2{}

	return s, nil
}

// Initialize initializes ViacoinRPC instance.
func (b *ViacoinRPC) Initialize() error {
	chainName, err := b.GetChainInfoAndInitializeMempool(b)
	if err != nil {
		return err
	}

	glog.Info("Chain name ", chainName)
	params := GetChainParams(chainName)

	// always create parser
	b.Parser = NewViacoinParser(params, b.ChainConfig)

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