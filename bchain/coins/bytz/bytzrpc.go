package bytz

import (
	"encoding/json"

	"github.com/golang/glog"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

// BytzRPC is an interface to JSON-RPC bitcoind service.
type BytzRPC struct {
	*btc.BitcoinRPC
}

// NewBytzRPC returns new BytzRPC instance.
func NewBytzRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	b, err := btc.NewBitcoinRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	s := &BytzRPC{
		b.(*btc.BitcoinRPC),
	}
	s.RPCMarshaler = btc.JSONMarshalerV1{}
	s.ChainConfig.SupportsEstimateFee = true
	s.ChainConfig.SupportsEstimateSmartFee = false

	return s, nil
}

// Initialize initializes PivXRPC instance.
func (b *BytzRPC) Initialize() error {
	ci, err := b.GetChainInfo()
	if err != nil {
		return err
	}

  chainName := ci.Chain

	glog.Info("Chain name ", chainName)
	params := GetChainParams(chainName)

	// always create parser
	b.Parser = NewBytzParser(params, b.ChainConfig)

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
