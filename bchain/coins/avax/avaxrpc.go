package avax

import (
	"context"
	"encoding/json"

	"github.com/ava-labs/coreth/core/types"
	"github.com/ava-labs/coreth/ethclient"
	avax "github.com/ava-labs/coreth/interfaces"
	"github.com/ava-labs/coreth/rpc"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
)

const (
	// MainNet is production network
	MainNet eth.Network = 43114
)

// AvalancheRPC is an interface to JSON-RPC avalanche service.
type AvalancheRPC struct {
	*eth.EthereumRPC
}

// NewAvalancheRPC returns new EthRPC instance.
func NewAvalancheRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	c, err := eth.NewEthereumRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	s := &AvalancheRPC{
		EthereumRPC: c.(*eth.EthereumRPC),
	}

	return s, nil
}

func openRPC(url string) (*AvalancheRPCClient, *AvalancheClient, error) {
	r, err := rpc.Dial(url)
	if err != nil {
		return nil, nil, err
	}

	rc := &AvalancheRPCClient{
		Client: r,
	}

	ec := &AvalancheClient{
		Client: ethclient.NewClient(r),
	}

	return rc, ec, nil
}

// Initialize initializes avalanche rpc interface
func (b *AvalancheRPC) Initialize() error {
	rc, ec, err := openRPC(b.ChainConfig.RPCURL)
	if err != nil {
		return err
	}

	b.EthereumRPC.Client = ec
	b.EthereumRPC.RPC = rc
	b.MainNetChainID = MainNet

	// new blocks notifications handling
	// the subscription is done in Initialize
	b.ChanNewBlock = make(chan *types.Header)
	go func() {
		for {
			h, ok := <-b.ChanNewBlock.(chan *types.Header)
			if !ok {
				break
			}
			b.UpdateBestHeader(&AvalancheHeader{Header: h})
			// notify blockbook
			b.PushHandler(bchain.NotificationNewBlock)
		}
	}()

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
func (b *AvalancheRPC) EthereumTypeEstimateGas(params map[string]interface{}) (uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	msg := avax.CallMsg{}
	if s, ok := eth.GetStringFromMap("from", params); ok && len(s) > 0 {
		msg.From = ethcommon.HexToAddress(s)
	}
	if s, ok := eth.GetStringFromMap("to", params); ok && len(s) > 0 {
		a := ethcommon.HexToAddress(s)
		msg.To = &a
	}
	if s, ok := eth.GetStringFromMap("data", params); ok && len(s) > 0 {
		msg.Data = ethcommon.FromHex(s)
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
