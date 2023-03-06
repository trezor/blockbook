package avalanche

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	jsontypes "github.com/ava-labs/avalanchego/utils/json"
	"github.com/ava-labs/coreth/core/types"
	"github.com/ava-labs/coreth/ethclient"
	"github.com/ava-labs/coreth/interfaces"
	"github.com/ava-labs/coreth/rpc"
	"github.com/ethereum/go-ethereum/common"
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
	info *rpc.Client
}

// NewAvalancheRPC returns new AvalancheRPC instance.
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

// Initialize avalanche rpc interface
func (b *AvalancheRPC) Initialize() error {
	b.OpenRPC = func(url string) (bchain.EVMRPCClient, bchain.EVMClient, error) {
		r, err := rpc.Dial(url)
		if err != nil {
			return nil, nil, err
		}
		rc := &AvalancheRPCClient{Client: r}
		c := &AvalancheClient{Client: ethclient.NewClient(r)}
		return rc, c, nil
	}

	rpcClient, client, err := b.OpenRPC(b.ChainConfig.RPCURL)
	if err != nil {
		return err
	}

	rpcUrl, err := url.Parse(b.ChainConfig.RPCURL)
	if err != nil {
		return err
	}

	scheme := "http"
	if rpcUrl.Scheme == "wss" || rpcUrl.Scheme == "https" {
		scheme = "https"
	}

	infoClient, err := rpc.DialHTTP(fmt.Sprintf("%s://%s/ext/info", scheme, rpcUrl.Host))
	if err != nil {
		return err
	}

	// set chain specific
	b.Client = client
	b.RPC = rpcClient
	b.info = infoClient
	b.MainNetChainID = MainNet
	b.NewBlock = &AvalancheNewBlock{channel: make(chan *types.Header)}
	b.NewTx = &AvalancheNewTx{channel: make(chan common.Hash)}

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

// GetChainInfo returns information about the connected backend
func (b *AvalancheRPC) GetChainInfo() (*bchain.ChainInfo, error) {
	ci, err := b.EthereumRPC.GetChainInfo()
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()

	var v struct {
		Version            string            `json:"version"`
		DatabaseVersion    string            `json:"databaseVersion"`
		RPCProtocolVersion jsontypes.Uint32  `json:"rpcProtocolVersion"`
		GitCommit          string            `json:"gitCommit"`
		VMVersions         map[string]string `json:"vmVersions"`
	}

	if err := b.info.CallContext(ctx, &v, "info.getNodeVersion"); err != nil {
		return nil, err
	}

	if avm, ok := v.VMVersions["avm"]; ok {
		ci.Version = avm
	}

	return ci, nil
}

// EthereumTypeEstimateGas returns estimation of gas consumption for given transaction parameters
func (b *AvalancheRPC) EthereumTypeEstimateGas(params map[string]interface{}) (uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	msg := interfaces.CallMsg{}
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
