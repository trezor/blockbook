package optimism

import (
	"context"
	"encoding/json"

	optimism "github.com/ethereum-optimism/optimism/l2geth"
	"github.com/ethereum-optimism/optimism/l2geth/common"
	"github.com/ethereum-optimism/optimism/l2geth/common/hexutil"
	"github.com/ethereum-optimism/optimism/l2geth/core/types"
	"github.com/ethereum-optimism/optimism/l2geth/ethclient"
	"github.com/ethereum-optimism/optimism/l2geth/rpc"
	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
)

const (
	// MainNet is production network
	MainNet eth.Network = 10
)

// OptimismRPC is an interface to JSON-RPC avalanche service.
type OptimismRPC struct {
	*eth.EthereumRPC
}

// NewOptimismRPC returns new EthRPC instance.
func NewOptimismRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	c, err := eth.NewEthereumRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	s := &OptimismRPC{
		EthereumRPC: c.(*eth.EthereumRPC),
	}

	return s, nil
}

// Initialize initializes avalanche rpc interface
func (b *OptimismRPC) Initialize() error {
	b.OpenRPC = func(url string) (bchain.EVMRPCClient, bchain.EVMClient, error) {
		r, err := rpc.Dial(url)
		if err != nil {
			return nil, nil, err
		}
		return &OptimismRPCClient{Client: r}, &OptimismClient{Client: ethclient.NewClient(r)}, nil
	}

	rc, ec, err := b.OpenRPC(b.ChainConfig.RPCURL)
	if err != nil {
		return err
	}

	b.EthereumRPC.Client = ec
	b.EthereumRPC.RPC = rc
	b.MainNetChainID = MainNet
	b.NewBlock = &OptimismNewBlock{channel: make(chan *types.Header)}
	b.NewTx = &OptimismNewTx{channel: make(chan common.Hash)}

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

// EthereumTypeEstimateGas returns estimation of gas consumption for given transaction parameters
func (b *OptimismRPC) EthereumTypeEstimateGas(params map[string]interface{}) (uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	msg := optimism.CallMsg{}
	if s, ok := eth.GetStringFromMap("from", params); ok && len(s) > 0 {
		msg.From = common.HexToAddress(s)
	}
	if s, ok := eth.GetStringFromMap("to", params); ok && len(s) > 0 {
		a := common.HexToAddress(s)
		msg.To = &a
	}
	if s, ok := eth.GetStringFromMap("data", params); ok && len(s) > 0 {
		msg.Data = common.FromHex(s)
	}
	if s, ok := eth.GetStringFromMap("value", params); ok && len(s) > 0 {
		msg.Value, _ = hexutil.DecodeBig(s)
	}
	if s, ok := eth.GetStringFromMap("gas", params); ok && len(s) > 0 {
		msg.Gas, _ = hexutil.DecodeUint64(s)
	}
	if s, ok := eth.GetStringFromMap("gasPrice", params); ok && len(s) > 0 {
		msg.GasPrice, _ = hexutil.DecodeBig(s)
	}
	return b.Client.EstimateGas(ctx, msg)
}
