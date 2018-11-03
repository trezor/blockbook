package digibyte

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"encoding/json"

	"github.com/golang/glog"
)

// DigiByteRPC is an interface to JSON-RPC bitcoind service.
type DigiByteRPC struct {
	*btc.BitcoinRPC
}

// NewDigiByteRPC returns new DigiByteRPC instance.
func NewDigiByteRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	b, err := btc.NewBitcoinRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	s := &DigiByteRPC{
		b.(*btc.BitcoinRPC),
	}
	s.RPCMarshaler = btc.JSONMarshalerV2{}
	s.ChainConfig.SupportsEstimateFee = false

	return s, nil
}

// Initialize initializes DigiByteRPC instance.
func (b *DigiByteRPC) Initialize() error {
	chainName, err := b.GetChainInfoAndInitializeMempool(b)
	if err != nil {
		return err
	}

	glog.Info("Chain name ", chainName)
	params := GetChainParams(chainName)

	// always create parser
	b.Parser = NewDigiByteParser(params, b.ChainConfig)

	b.Testnet = false
	b.Network = "livenet"

	glog.Info("rpc: block chain ", params.Name)

	return nil
}
