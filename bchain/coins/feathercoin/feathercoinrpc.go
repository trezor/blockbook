package feathercoin

import (
	"encoding/json"

	"github.com/golang/glog"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

// FeathercoinRPC is an interface to JSON-RPC bitcoind service.
type FeathercoinRPC struct {
	*btc.BitcoinRPC
}

// NewFeathercoinRPC returns new FeathercoinRPC instance.
func NewFeathercoinRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	b, err := btc.NewBitcoinRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	s := &FeathercoinRPC{
		b.(*btc.BitcoinRPC),
	}
	s.RPCMarshaler = btc.JSONMarshalerV2{}
	s.ChainConfig.SupportsEstimateFee = false

	return s, nil
}

// Initialize initializes FeathercoinRPC instance.
func (b *FeathercoinRPC) Initialize() error {
	ci, err := b.GetChainInfo()
	if err != nil {
		return err
	}
	chainName := ci.Chain

	glog.Info("Chain name ", chainName)
	params := GetChainParams(chainName)

	// always create parser
	b.Parser = NewFeathercoinParser(params, b.ChainConfig)

	// parameters for getInfo request
	b.Testnet = false
	b.Network = "livenet"

	glog.Info("rpc: block chain ", params.Name)

	return nil
}

