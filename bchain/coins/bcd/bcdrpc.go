package bcd

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"encoding/json"
	"github.com/golang/glog"
	"github.com/martinboehm/btcd/wire"
)

const bitcoinDiamondForkHeight = 495866

// BGoldRPC is an interface to JSON-RPC bitcoind service.
type BitcoinDiamondRPC struct {
	*btc.BitcoinRPC
}

// NewBGoldRPC returns new BGoldRPC instance.
func NewBitcoinDiamondRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	b, err := btc.NewBitcoinRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	s := &BitcoinDiamondRPC{
		b.(*btc.BitcoinRPC),
	}

	return s, nil
}

// Initialize initializes BGoldRPC instance.
func (b *BitcoinDiamondRPC) Initialize() error {
	ci, err := b.GetChainInfo()
	if err != nil {
		return err
	}
	chainName := ci.Chain

	params := btc.GetChainParams(chainName)

	// always create parser
	b.Parser = NewBitcoinDiamondParser(params, b.ChainConfig)

	// parameters for getInfo request
	if params.Net == wire.MainNet {
		b.Testnet = false
		b.Network = "livenet"
	} else {
		b.Testnet = true
		b.Network = "testnet"
	}

	glog.Info("rpc: block chain ", params.Name)
	return nil
}

func (b *BitcoinDiamondRPC) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	var err error
	if hash == "" && height > 0 {
		hash, err = b.GetBlockHash(height)
		if err != nil {
			return nil, err
		}
	}

	if height < bitcoinDiamondForkHeight {
		return b.GetBlockWithoutHeader(hash, height)
	}

	return b.GetBlockFull(hash)

}