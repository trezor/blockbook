package avalanche

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"time"

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
	// nodeVersionTTL is how long an info.getNodeVersion answer is reused, see cachedNodeVersion.
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
	// avm version snapshot, see cachedNodeVersion.
	nodeVersionMu      sync.Mutex
	nodeVersion        string
	nodeVersionProbing bool
	// nodeVersionAttempt is the last probe attempt, successful or not; zero means never
	// attempted. Suppression keys on the attempt so a node that never answers with an avm
	// version is still probed only once per TTL.
	nodeVersionAttempt time.Time
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

	if v := b.cachedNodeVersion(); v != "" {
		ci.Version = v
	}

	return ci, nil
}

// cachedNodeVersion returns the avm version, re-querying the info endpoint at most once per
// nodeVersionTTL: GetChainInfo serves / and /api/ on every request, and this value changes
// only when the node is restarted onto a new build. Empty means never probed successfully.
func (b *AvalancheRPC) cachedNodeVersion() string {
	return b.nodeVersionCached(time.Now(), b.probeNodeVersion)
}

// nodeVersionCached holds the TTL rules; now and probe are injected so they can be tested
// without sleeping or a live node.
func (b *AvalancheRPC) nodeVersionCached(now time.Time, probe func() string) string {
	b.nodeVersionMu.Lock()
	// One caller at a time probes and the rest return the current value straight away, so
	// concurrent requests never queue behind an RPC that may sit until b.Timeout. A failed
	// probe is not retried for a TTL either - the version is a label, never a reason to
	// fail GetChainInfo, and an info endpoint that is down must not cost an RPC per request.
	if b.nodeVersionProbing || (!b.nodeVersionAttempt.IsZero() && now.Sub(b.nodeVersionAttempt) < nodeVersionTTL) {
		v := b.nodeVersion
		b.nodeVersionMu.Unlock()
		return v
	}
	b.nodeVersionProbing = true
	b.nodeVersionAttempt = now
	b.nodeVersionMu.Unlock()

	probed := probe()

	b.nodeVersionMu.Lock()
	defer b.nodeVersionMu.Unlock()
	b.nodeVersionProbing = false
	if probed != "" {
		b.nodeVersion = probed
	}
	return b.nodeVersion
}

// probeNodeVersion returns the node's avm version, or "" if the info endpoint does not
// answer or reports no avm version.
func (b *AvalancheRPC) probeNodeVersion() string {
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
		glog.V(1).Info("avalanche: info.getNodeVersion failed: ", err)
		return ""
	}
	return v.VMVersions["avm"]
}
