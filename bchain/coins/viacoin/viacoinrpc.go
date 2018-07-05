package viacoin

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"encoding/json"

	"github.com/golang/glog"
)

// ViacoinRPC is an interface to JSON-RPC bitcoind service.
type ViacoinRPC struct {
	*btc.BitcoinRPC
}

// NewViacoinRPC returns new ViacoinRPC instance.
func NewViacoinRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
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

// EstimateFee returns fee estimation.
func (b *ViacoinRPC) EstimateFee(blocks int) (float64, error) {
	return b.EstimateSmartFee(blocks, true)
}
