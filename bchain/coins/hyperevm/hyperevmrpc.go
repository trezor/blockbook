package hyperevm

import (
	"context"
	"encoding/json"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
)

const (
	// MainNet is the chain ID of HyperEVM (Hyperliquid) mainnet
	MainNet eth.Network = 999
)

// HyperevmRPC is an interface to JSON-RPC HyperEVM service.
type HyperevmRPC struct {
	*eth.EthereumRPC
}

// NewHyperevmRPC returns new HyperevmRPC instance.
func NewHyperevmRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	c, err := eth.NewEthereumRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	s := &HyperevmRPC{
		EthereumRPC: c.(*eth.EthereumRPC),
	}

	return s, nil
}

// Initialize hyperevm rpc interface
func (b *HyperevmRPC) Initialize() error {
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
	default:
		return errors.Errorf("Unknown network id %v", id)
	}

	if err = b.InitAlternativeProviders(); err != nil {
		return err
	}

	glog.Info("rpc: block chain ", b.Network)

	return nil
}

// GetBlockHash returns the hash reported by the backend, not go-ethereum's RLP
// recomputation of the header used by the base implementation. The two disagree at the
// HyperEVM genesis, which wedged sync at height 0. Blocks are stored under the reported
// hash, so read that field at every height to keep the comparisons consistent.
func (b *HyperevmRPC) GetBlockHash(height uint32) (string, error) {
	raw, err := b.GetBlockRawByHashOrHeight("", height, false)
	if err != nil {
		// Not annotated: callers match bchain.ErrBlockNotFound with errors.Is and the
		// pinned juju/errors has no Unwrap.
		return "", err
	}
	var h struct {
		Hash string `json:"hash"`
	}
	if err := json.Unmarshal(raw, &h); err != nil {
		return "", errors.Annotatef(err, "height %v", height)
	}
	if h.Hash == "" {
		return "", bchain.ErrBlockNotFound
	}
	return h.Hash, nil
}

func (b *HyperevmRPC) ResolveENS(name string) (*bchain.ENSResolution, error) {
	return b.EthereumRPC.ResolveENS(name)
}
