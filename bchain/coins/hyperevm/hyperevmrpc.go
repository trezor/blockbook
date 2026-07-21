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

// GetBlockHash returns the hash of the block at the given height.
//
// For every height except genesis this defers to the base implementation, which
// derives the hash via go-ethereum's HeaderByNumber().Hash() — i.e. it RLP-hashes
// the header fields rather than trusting the node's "hash" field. That works for
// blocks >= 1, but for the HyperEVM genesis go-ethereum recomputes 0x0466…8f8bd5,
// which does NOT match reth-hl's canonical genesis hash 0xd8fcc13b…895f0 (the
// genesis header carries zero-valued Prague/Cancun fields that reth-hl folded into
// the canonical hash under different rules). Sync then asks the backend for a block
// by that mis-derived hash, gets null, and wedges at height 0 forever.
//
// Return the hash the backend actually reports for the genesis header instead, so
// the stored genesis and every subsequent tip/reorg comparison stay consistent.
func (b *HyperevmRPC) GetBlockHash(height uint32) (string, error) {
	if height == 0 {
		raw, err := b.GetBlockRawByHashOrHeight("", 0, false)
		if err != nil {
			return "", errors.Annotate(err, "genesis")
		}
		var h struct {
			Hash string `json:"hash"`
		}
		if err := json.Unmarshal(raw, &h); err != nil {
			return "", errors.Annotate(err, "genesis")
		}
		if h.Hash == "" {
			return "", bchain.ErrBlockNotFound
		}
		return h.Hash, nil
	}
	return b.EthereumRPC.GetBlockHash(height)
}

// GetBlock returns the block of the given hash and height. Genesis is fetched by
// number (empty hash routes getBlockRaw to eth_getBlockByNumber) because
// GetBlockHash's caller passes go-ethereum's mis-derived genesis hash; fetching by
// number sidesteps the eth_getBlockByHash lookup that would return null. All other
// heights keep the hash-based path so reorg detection during sync stays intact.
func (b *HyperevmRPC) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	if height == 0 {
		hash = ""
	}
	return b.EthereumRPC.GetBlock(hash, height)
}

func (b *HyperevmRPC) ResolveENS(name string) (*bchain.ENSResolution, error) {
	return b.EthereumRPC.ResolveENS(name)
}
