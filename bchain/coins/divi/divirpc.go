package divi

import (
	"encoding/json"

	"github.com/golang/glog"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

// DivicoinRPC is an interface to JSON-RPC bitcoind service.
type DivicoinRPC struct {
	*btc.BitcoinRPC
}

// NewDiviRPC returns new DivicoinRPC instance.
func NewDiviRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	b, err := btc.NewBitcoinRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	s := &DivicoinRPC{
		b.(*btc.BitcoinRPC),
	}
	s.RPCMarshaler = btc.JSONMarshalerV1{}
	s.ChainConfig.SupportsEstimateFee = true
	s.ChainConfig.SupportsEstimateSmartFee = false

	return s, nil
}

// Initialize initializes DivicoinRPC instance.
func (b *DivicoinRPC) Initialize() error {
	ci, err := b.GetChainInfo()
	if err != nil {
		return err
	}
	chainName := ci.Chain

	glog.Info("Chain name ", chainName)
	params := GetChainParams(chainName)

	// always create parser
	b.Parser = NewDiviParser(params, b.ChainConfig)

	/* parameters for getInfo request
	-- can be added later
	if params.Net == MainnetMagic {*/
	b.Testnet = false
	b.Network = "livenet"
	/*} else {
		b.Testnet = true
		b.Network = "testnet"
	}*/

	glog.Info("rpc: block chain ", params.Name)

	return nil
}
