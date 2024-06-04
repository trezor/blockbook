package bsc

import (
	"context"
	"encoding/json"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
)

const (
	// MainNet is production network
	MainNet eth.Network = 56

	// bsc token type names
	BEP20TokenType   bchain.TokenTypeName = "BEP20"
	BEP721TokenType  bchain.TokenTypeName = "BEP721"
	BEP1155TokenType bchain.TokenTypeName = "BEP1155"
)

// BNBSmartChainRPC is an interface to JSON-RPC bsc service.
type BNBSmartChainRPC struct {
	*eth.EthereumRPC
}

// NewBNBSmartChainRPC returns new BNBSmartChainRPC instance.
func NewBNBSmartChainRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	c, err := eth.NewEthereumRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	// overwrite EthereumTokenTypeMap with bsc specific token type names
	bchain.EthereumTokenTypeMap = []bchain.TokenTypeName{BEP20TokenType, BEP721TokenType, BEP1155TokenType}

	s := &BNBSmartChainRPC{
		EthereumRPC: c.(*eth.EthereumRPC),
	}

	return s, nil
}

// Initialize bnb smart chain rpc interface
func (b *BNBSmartChainRPC) Initialize() error {
	b.OpenRPC = eth.OpenRPC

	rc, ec, err := b.OpenRPC(b.ChainConfig.RPCURL)
	if err != nil {
		return err
	}

	// set chain specific
	b.Client = ec
	b.RPC = rc
	b.MainNetChainID = MainNet
	b.NewBlock = eth.NewEthereumNewBlock()
	b.NewTx = eth.NewEthereumNewTx()

	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()

	id, err := b.Client.NetworkID(ctx)
	if err != nil {
		return err
	}

	// parameters for getInfo request
	switch eth.Network(id.Uint64()) {
	case MainNet:
		b.Testnet = false
		b.Network = "livenet"
	default:
		return errors.Errorf("Unknown network id %v", id)
	}

	glog.Info("rpc: block chain ", b.Network)

	return nil
}
