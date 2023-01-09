package block

import (
	"encoding/json"

	"github.com/golang/glog"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

// BlocknetRPC is an interface to JSON-RPC blocknetd service.
type BlocknetRPC struct {
	*btc.BitcoinRPC
}

// NewBlocknetRPC returns new BlocknetRPC instance.
func NewBlocknetRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	b, err := btc.NewBitcoinRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	s := &BlocknetRPC{
		b.(*btc.BitcoinRPC),
	}
	s.RPCMarshaler = btc.JSONMarshalerV1{}
	// s.ChainConfig.SupportsEstimateFee = true
	// s.ChainConfig.SupportsEstimateSmartFee = false

	return s, nil
}

// Initialize initializes BlocknetRPC instance.
func (b *BlocknetRPC) Initialize() error {
	ci, err := b.GetChainInfo()
	if err != nil {
		return err
	}
	chainName := ci.Chain

	params := GetChainParams(chainName)

	// always create parser
	b.Parser = NewBlocknetParser(params, b.ChainConfig)

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
