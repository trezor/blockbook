package arbitrum

import (
	"context"
	"encoding/json"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
)

const (
	ArbitrumOneMainNet  eth.Network = 42161
	ArbitrumNovaMainNet eth.Network = 42170
)

// ArbitrumRPC is an interface to JSON-RPC arbitrum service.
type ArbitrumRPC struct {
	*eth.EthereumRPC
}

// NewArbitrumRPC returns new ArbitrumRPC instance.
func NewArbitrumRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	c, err := eth.NewEthereumRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	s := &ArbitrumRPC{
		EthereumRPC: c.(*eth.EthereumRPC),
	}

	return s, nil
}

// Initialize arbitrum rpc interface
func (b *ArbitrumRPC) Initialize() error {
	b.OpenRPC = eth.OpenRPC

	rc, ec, err := b.OpenRPC(b.ChainConfig.RPCURL)
	if err != nil {
		return err
	}

	// set chain specific
	b.Client = ec
	b.RPC = rc
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
	case ArbitrumOneMainNet:
		b.MainNetChainID = ArbitrumOneMainNet
		b.Testnet = false
		b.Network = "livenet"
	case ArbitrumNovaMainNet:
		b.MainNetChainID = ArbitrumNovaMainNet
		b.Testnet = false
		b.Network = "livenet"
	default:
		return errors.Errorf("Unknown network id %v", id)
	}

	b.InitAlternativeProviders()

	glog.Info("rpc: block chain ", b.Network)

	return nil
}
