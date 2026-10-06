package litecoincash

import (
	"encoding/json"

	"github.com/golang/glog"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

// LitecoinCashRPC is an interface to Litecoin Cash Core JSON-RPC.
type LitecoinCashRPC struct {
	*btc.BitcoinRPC
}

// NewLitecoinCashRPC returns new LitecoinCashRPC instance.
func NewLitecoinCashRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	b, err := btc.NewBitcoinRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	s := &LitecoinCashRPC{
		BitcoinRPC: b.(*btc.BitcoinRPC),
	}
	s.RPCMarshaler = btc.JSONMarshalerV2{}
	s.ChainConfig.SupportsEstimateFee = false

	return s, nil
}

// Initialize initializes LitecoinCashRPC instance.
func (b *LitecoinCashRPC) Initialize() error {
	ci, err := b.GetChainInfo()
	if err != nil {
		return err
	}
	chainName := ci.Chain

	glog.Info("Chain name ", chainName)
	params := GetChainParams(chainName)

	// Always create the Litecoin Cash parser.
	b.Parser = NewLitecoinCashParser(params, b.ChainConfig)

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
