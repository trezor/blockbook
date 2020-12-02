package ghost

import (
	"encoding/json"
	"github.com/golang/glog"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

// GhostRPC is an interface to JSON-RPC bitcoind service.
type GhostRPC struct {
	*btc.BitcoinRPC
}

// NewGhostRPC returns new GhostRPC instance.
func NewGhostRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	b, err := btc.NewBitcoinRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	s := &GhostRPC{
		b.(*btc.BitcoinRPC),
	}
	s.RPCMarshaler = btc.JSONMarshalerV2{}
	s.ChainConfig.Parse = false
	s.ChainConfig.SupportsEstimateFee = false

	return s, nil
}

// Initialize initializes GhostRPC instance.
func (b *GhostRPC) Initialize() error {
	ci, err := b.GetChainInfo()
	if err != nil {
		return err
	}
	chainName := ci.Chain
	glog.Info("Chain name ", chainName)
	params := GetChainParams(chainName)

	// always create parser
	b.Parser = NewGhostParser(params, b.ChainConfig)

	// parameters for getInfo request
	if params.Net == MainnetMagic {
		b.Testnet = false
		b.Network = "MainChain"
	} else {
		b.Testnet = true
		b.Network = "TestChain"
	}

	glog.Info("rpc: block chain ", params.Name)

	return nil
}

// GetBlock returns block with given hash.
func (g *GhostRPC) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	var err error
	if hash == "" && height > 0 {
		hash, err = g.GetBlockHash(height)
		if err != nil {
			return nil, err
		}
	}
	block, err := g.GetBlockFull(hash)
	return block, err
}