package dash

import (
	"encoding/json"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"

	"github.com/golang/glog"
)

// DashRPC is an interface to JSON-RPC bitcoind service.
type DashRPC struct {
	*btc.BitcoinRPC
}

// NewDashRPC returns new DashRPC instance.
func NewDashRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	b, err := btc.NewBitcoinRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	s := &DashRPC{
		b.(*btc.BitcoinRPC),
	}
	s.RPCMarshaler = btc.JSONMarshalerV1{}

	return s, nil
}

// Initialize initializes DashRPC instance.
func (b *DashRPC) Initialize() error {
	chainName, err := b.GetChainInfoAndInitializeMempool(b)
	if err != nil {
		return err
	}

	params := GetChainParams(chainName)

	// always create parser
	b.Parser = NewDashParser(params, b.ChainConfig)

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

// EstimateSmartFee returns fee estimation.
func (b *DashRPC) EstimateSmartFee(blocks int, conservative bool) (float64, error) {
	return b.EstimateFee(blocks)
}
