package avalanche

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
	"github.com/trezor/blockbook/common"
)

const (
	// MainNet is production network
	MainNet eth.Network = 43114
	// nodeVersionTTL is how long an info.getNodeVersion answer is reused, see probeNodeVersion.
	nodeVersionTTL = 60 * time.Second
)

func dialRPC(rawURL string) (*rpc.Client, error) {
	if rawURL == "" {
		return nil, errors.New("empty rpc url")
	}
	return rpc.DialOptions(context.Background(), rawURL, eth.RPCDialOptions(rawURL)...)
}

// OpenRPC opens RPC connections for Avalanche to separate HTTP and WS endpoints.
var OpenRPC = func(httpURL, wsURL string) (bchain.EVMRPCClient, bchain.EVMClient, error) {
	callURL, subURL, err := eth.NormalizeRPCURLs(httpURL, wsURL)
	if err != nil {
		return nil, nil, err
	}
	callClient, err := dialRPC(callURL)
	if err != nil {
		return nil, nil, err
	}
	callRPC := &AvalancheRPCClient{Client: callClient}
	subRPC := callRPC
	if subURL != callURL {
		subClient, err := dialRPC(subURL)
		if err != nil {
			callClient.Close()
			return nil, nil, err
		}
		subRPC = &AvalancheRPCClient{Client: subClient}
	}
	rc := &AvalancheDualRPCClient{CallClient: callRPC, SubClient: subRPC}
	c := &AvalancheClient{Client: ethclient.NewClient(callClient), AvalancheRPCClient: callRPC}
	return rc, c, nil
}

// AvalancheRPC is an interface to JSON-RPC avalanche service.
type AvalancheRPC struct {
	*eth.EthereumRPC
	info *rpc.Client
	// avm version snapshot, see probeNodeVersion.
	nodeVersion *common.TTLValue[string]
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
	s.nodeVersion = common.NewTTLValue(nodeVersionTTL, s.probeNodeVersion)

	return s, nil
}

// Initialize avalanche rpc interface
func (b *AvalancheRPC) Initialize() error {
	b.OpenRPC = OpenRPC

	rpcClient, client, err := b.OpenRPC(b.ChainConfig.RPCURL, b.ChainConfig.RPCURLWS)
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

	infoURL := fmt.Sprintf("%s://%s/ext/info", scheme, rpcUrl.Host)
	infoClient, err := rpc.DialOptions(context.Background(), infoURL, eth.RPCDialOptions(infoURL)...)
	if err != nil {
		return err
	}

	// set chain specific
	b.Client = client
	b.RPC = rpcClient
	b.info = infoClient
	b.MainNetChainID = MainNet
	b.NewBlock = &AvalancheNewBlock{channel: make(chan *Header)}
	b.NewTx = &AvalancheNewTx{channel: make(chan ethcommon.Hash)}

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

	if err = b.InitAlternativeProviders(); err != nil {
		return err
	}

	glog.Info("rpc: block chain ", b.Network)

	return nil
}

// GetChainInfo returns information about the connected backend
func (b *AvalancheRPC) GetChainInfo() (*bchain.ChainInfo, error) {
	ci, err := b.EthereumRPC.GetChainInfo()
	if err != nil {
		return nil, err
	}

	// the avm version is a label, never a reason to fail GetChainInfo - nil means never probed
	if v, _, _ := b.nodeVersion.Get(time.Now()); v != nil {
		ci.Version = *v
	}

	return ci, nil
}

// probeNodeVersion fetches the node's avm version from the info endpoint; GetChainInfo
// prefers it over the parent's web3_clientVersion.
func (b *AvalancheRPC) probeNodeVersion() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()

	var v struct {
		VMVersions map[string]string `json:"vmVersions"`
	}
	if err := b.info.CallContext(ctx, &v, "info.getNodeVersion"); err != nil {
		glog.V(1).Info("avalanche: info.getNodeVersion failed: ", err)
		return "", err
	}
	if v.VMVersions["avm"] == "" {
		return "", errors.New("info endpoint reported no avm version")
	}
	return v.VMVersions["avm"], nil
}
