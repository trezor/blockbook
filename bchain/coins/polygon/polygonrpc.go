package polygon

import (
	"context"
	"encoding/json"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
)

const (
	// MainNet is production network
	MainNet eth.Network = 137
)

// PolygonRPC is an interface to JSON-RPC polygon service.
type PolygonRPC struct {
	*eth.EthereumRPC
}

// NewPolygonRPC returns new PolygonRPC instance.
func NewPolygonRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	c, err := eth.NewEthereumRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	s := &PolygonRPC{
		EthereumRPC: c.(*eth.EthereumRPC),
	}

	return s, nil
}

// Initialize polygon rpc interface
func (b *PolygonRPC) Initialize() error {
	b.OpenRPC = func(url string) (bchain.EVMRPCClient, bchain.EVMClient, error) {
		r, err := rpc.Dial(url)
		if err != nil {
			return nil, nil, err
		}
		rc := &eth.EthereumRPCClient{Client: r}
		ec := &eth.EthereumClient{Client: ethclient.NewClient(r)}
		return rc, ec, nil
	}

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
