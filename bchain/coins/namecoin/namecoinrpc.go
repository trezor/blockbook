package namecoin

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"encoding/json"

	"github.com/golang/glog"
)

// NamecoinRPC is an interface to JSON-RPC namecoin service.
type NamecoinRPC struct {
	*btc.BitcoinRPC
}

// NewNamecoinRPC returns new NamecoinRPC instance.
func NewNamecoinRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	b, err := btc.NewBitcoinRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	s := &NamecoinRPC{
		b.(*btc.BitcoinRPC),
	}
	s.RPCMarshaler = btc.JSONMarshalerV1{}
	s.ChainConfig.SupportsEstimateFee = false

	return s, nil
}

// Initialize initializes NamecoinRPC instance.
func (b *NamecoinRPC) Initialize() error {
	chainName, err := b.GetChainInfoAndInitializeMempool(b)
	if err != nil {
		return err
	}

	glog.Info("Chain name ", chainName)
	params := GetChainParams(chainName)

	// always create parser
	b.Parser = NewNamecoinParser(params, b.ChainConfig)

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

// GetBlock returns block with given hash.
func (b *NamecoinRPC) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	var err error
	if hash == "" {
		hash, err = b.GetBlockHash(height)
		if err != nil {
			return nil, err
		}
	}
	if !b.ParseBlocks {
		return b.GetBlockFull(hash)
	}
	return b.GetBlockWithoutHeader(hash, height)
}
