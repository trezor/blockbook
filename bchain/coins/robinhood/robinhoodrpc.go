package robinhood

import (
	"context"
	"encoding/json"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
)

const (
	// MainNet is the chain ID of Robinhood Chain mainnet
	MainNet eth.Network = 4663
	// TestNet is the chain ID of Robinhood Chain testnet (settles to Sepolia)
	TestNet eth.Network = 46630
)

// RobinhoodRPC is an interface to JSON-RPC robinhood service.
type RobinhoodRPC struct {
	*eth.EthereumRPC
}

// NewRobinhoodRPC returns new RobinhoodRPC instance.
func NewRobinhoodRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	c, err := eth.NewEthereumRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	s := &RobinhoodRPC{
		EthereumRPC: c.(*eth.EthereumRPC),
	}

	return s, nil
}

// Initialize robinhood rpc interface
func (b *RobinhoodRPC) Initialize() error {
	b.OpenRPC = eth.OpenRPC

	rc, ec, err := b.OpenRPC(b.ChainConfig.RPCURL, b.ChainConfig.RPCURLWS)
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
	case MainNet:
		b.MainNetChainID = MainNet
		b.Testnet = false
		b.Network = "livenet"
	case TestNet:
		b.MainNetChainID = MainNet
		b.Testnet = true
		b.Network = "testnet"
	default:
		return errors.Errorf("Unknown network id %v", id)
	}

	if err = b.InitAlternativeProviders(); err != nil {
		return err
	}

	glog.Info("rpc: block chain ", b.Network)

	return nil
}

func (b *RobinhoodRPC) ResolveENS(name string) (*bchain.ENSResolution, error) {
	return b.EthereumRPC.ResolveENS(name)
}
