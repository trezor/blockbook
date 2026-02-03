package base

import (
	"context"
	"encoding/json"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
)

const (
	// MainNet is production network
	MainNet eth.Network = 8453
)

// BaseRPC is an interface to JSON-RPC base service.
type BaseRPC struct {
	*eth.EthereumRPC
}

// baseParserWrapper wraps EthereumParser to filter NFT transfers
type baseParserWrapper struct {
	*eth.EthereumParser
}

// EthereumTypeGetTokenTransfersFromTx returns contract transfers from bchain.Tx
// Overridden to skip NFT transfers for Base
// TEMPORARY FIX: Skip NFT processing for Base due to performance/consistency issues
// This is a workaround to handle NFT-related DB consistency problems.
func (p *baseParserWrapper) EthereumTypeGetTokenTransfersFromTx(tx *bchain.Tx) (bchain.TokenTransfers, error) {
	// Call parent implementation to get all transfers
	transfers, err := p.EthereumParser.EthereumTypeGetTokenTransfersFromTx(tx)
	if err != nil {
		return nil, err
	}

	// Filter out NFT transfers (ERC721 and ERC1155)
	if len(transfers) == 0 {
		return transfers, nil
	}

	filtered := make(bchain.TokenTransfers, 0, len(transfers))
	skippedCount := 0

	for _, transfer := range transfers {
		// Skip ERC721 (NonFungibleToken) and ERC1155 (MultiToken)
		if transfer.Standard == bchain.NonFungibleToken || transfer.Standard == bchain.MultiToken {
			skippedCount++
			continue
		}
		filtered = append(filtered, transfer)
	}

	if skippedCount > 0 {
		glog.V(1).Infof("Skipped %d NFT transfers (ERC721/ERC1155)", skippedCount)
	}

	return filtered, nil
}

// NewBaseRPC returns new BaseRPC instance.
func NewBaseRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	c, err := eth.NewEthereumRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	s := &BaseRPC{
		EthereumRPC: c.(*eth.EthereumRPC),
	}

	return s, nil
}

// Initialize base rpc interface
func (b *BaseRPC) Initialize() error {
	b.OpenRPC = eth.OpenRPC

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

	b.InitAlternativeProviders()

	glog.Info("rpc: block chain ", b.Network)

	return nil
}

// GetChainParser returns the Base parser which filters out NFT transfers
func (b *BaseRPC) GetChainParser() bchain.BlockChainParser {
	return &baseParserWrapper{
		EthereumParser: b.Parser,
	}
}
