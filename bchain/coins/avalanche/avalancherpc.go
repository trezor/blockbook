package avalanche

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	jsontypes "github.com/ava-labs/avalanchego/utils/json"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
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
		c := &AvalancheClient{Client: ethclient.NewClient(r), AvalancheRPCClient: rc}
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
	b.NewBlock = &AvalancheNewBlock{channel: make(chan *Header)}
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

	b.InitAlternativeProviders()

	glog.Info("rpc: block chain ", b.Network)

	return nil
}

// GetChainInfo returns information about the connected backend
func (b *AvalancheRPC) GetChainInfo() (*bchain.ChainInfo, error) {
	ci, err := b.EthereumRPC.GetChainInfo()
	if err != nil {
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

	if err := b.info.CallContext(ctx, &v, "info.getNodeVersion"); err == nil {
		if avm, ok := v.VMVersions["avm"]; ok {
			ci.Version = avm
		}
	}

	return ci, nil
}
