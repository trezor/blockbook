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

	// bsc token standard names
	BEP20TokenStandard   bchain.TokenStandardName = "BEP20"
	BEP721TokenStandard  bchain.TokenStandardName = "BEP721"
	BEP1155TokenStandard bchain.TokenStandardName = "BEP1155"
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

	// overwrite EthereumTokenStandardMap with bsc specific token standard names
	bchain.EthereumTokenStandardMap = []bchain.TokenStandardName{BEP20TokenStandard, BEP721TokenStandard, BEP1155TokenStandard}

	s := &BNBSmartChainRPC{
		EthereumRPC: c.(*eth.EthereumRPC),
	}
	s.Parser.EnsSuffix = ".bnb"

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

	b.InitAlternativeProviders()

	glog.Info("rpc: block chain ", b.Network)

	return nil
}
