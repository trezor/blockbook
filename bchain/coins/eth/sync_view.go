package eth

import (
	"context"

	"github.com/trezor/blockbook/bchain"
)

// ethSyncView is the BlockChain view handed to the SyncWorker for the
// chain-tip code path (sequential connectBlocks). It embeds *EthereumRPC so
// every BlockChain method is delegated unchanged, and overrides only the four
// methods used by db/sync.go's tip-sync flow to tag the request context with
// WithSyncRoute. The tag causes DualRPCClient.CallContext and EthereumClient
// methods to dispatch to the WebSocket-backed clients, which keeps follow-up
// fetches sticky to the backend that delivered newHeads (avoiding the
// load-balancer drift where one node returns 404 for a block another node
// just announced).
//
// Bulk and parallel sync paths in db/sync.go keep using the original
// (HTTP-routed) chain reference, so high-volume catch-up traffic still fans
// out across the LB pool.
type ethSyncView struct {
	*EthereumRPC
}

// SyncBlockChain returns the WS-routed view used by the SyncWorker's
// connectBlocks path. Implements bchain.SyncableBlockChain.
func (b *EthereumRPC) SyncBlockChain() bchain.BlockChain {
	return &ethSyncView{EthereumRPC: b}
}

// GetBlock fetches the full block (header + transactions + logs + internal
// trace) over the WS-pinned connection.
func (s *ethSyncView) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	return s.EthereumRPC.getBlockWithCtx(WithSyncRoute(context.Background()), hash, height)
}

// GetBlockHash fetches the canonical hash at height over the WS-pinned connection.
func (s *ethSyncView) GetBlockHash(height uint32) (string, error) {
	return s.EthereumRPC.getBlockHashWithCtx(WithSyncRoute(context.Background()), height)
}

// GetBestBlockHash returns the cached best-header's hash, falling back to a
// WS-routed HeaderByNumber(nil) on cache miss.
func (s *ethSyncView) GetBestBlockHash() (string, error) {
	return s.EthereumRPC.getBestBlockHashWithCtx(WithSyncRoute(context.Background()))
}

// GetBestBlockHeight returns the cached best-header's height, falling back to
// a WS-routed HeaderByNumber(nil) on cache miss.
func (s *ethSyncView) GetBestBlockHeight() (uint32, error) {
	return s.EthereumRPC.getBestBlockHeightWithCtx(WithSyncRoute(context.Background()))
}
