package monetaryunit

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"encoding/json"

	"github.com/golang/glog"
)

// MonetaryUnitRPC is an interface to JSON-RPC bitcoind service.
type MonetaryUnitRPC struct {
	*btc.BitcoinRPC
}

// NewMonetaryUnitRPC returns new MonetaryUnitRPC instance.
func NewMonetaryUnitRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	b, err := btc.NewBitcoinRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	s := &MonetaryUnitRPC{
		b.(*btc.BitcoinRPC),
	}
	s.RPCMarshaler = btc.JSONMarshalerV1{}
	s.ChainConfig.SupportsEstimateFee = true
	s.ChainConfig.SupportsEstimateSmartFee = false

	return s, nil
}

// Initialize initializes MonetaryUnitRPC instance.
func (b *MonetaryUnitRPC) Initialize() error {
	chainName, err := b.GetChainInfoAndInitializeMempool(b)
	if err != nil {
		return err
	}

	glog.Info("Chain name ", chainName)
	params := GetChainParams(chainName)

	// always create parser
	b.Parser = NewMonetaryUnitParser(params, b.ChainConfig)

	b.Testnet = false
	b.Network = "livenet"

	glog.Info("rpc: block chain ", params.Name)

	return nil
}
