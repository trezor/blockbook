package groestlcoin

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"encoding/json"

	"github.com/golang/glog"
)

// GroestlcoinRPC is an interface to JSON-RPC bitcoind service.
type GroestlcoinRPC struct {
	*btc.BitcoinRPC
}

// NewGroestlcoinRPC returns new GroestlcoinRPC instance.
func NewGroestlcoinRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	b, err := btc.NewBitcoinRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	s := &GroestlcoinRPC{
		b.(*btc.BitcoinRPC),
	}
	s.RPCMarshaler = btc.JSONMarshalerV2{}

	return s, nil
}

// Initialize initializes GroestlcoinRPC instance.
func (b *GroestlcoinRPC) Initialize() error {
	chainName, err := b.GetChainInfoAndInitializeMempool(b)
	if err != nil {
		return err
	}

	glog.Info("Chain name ", chainName)
	params := GetChainParams(chainName)

	// always create parser
	b.Parser = NewGroestlcoinParser(params, b.ChainConfig)

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
func (b *GroestlcoinRPC) EstimateFee(blocks int) (float64, error) {
	return b.EstimateSmartFee(blocks, true)
}
